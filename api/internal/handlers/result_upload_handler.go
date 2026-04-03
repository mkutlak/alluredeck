package handlers

import (
	"archive/tar"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// ResultUploadHandler handles test result file uploads and cleanup.
type ResultUploadHandler struct {
	store        storage.Store
	projectStore store.ProjectStorer
	jobManager   runner.JobQueuer
	runner       *runner.Allure
	cfg          *config.Config
	logger       *zap.Logger
}

// NewResultUploadHandler creates and returns a new ResultUploadHandler.
func NewResultUploadHandler(st storage.Store, ps store.ProjectStorer, jm runner.JobQueuer, r *runner.Allure, cfg *config.Config, logger *zap.Logger) *ResultUploadHandler {
	return &ResultUploadHandler{store: st, projectStore: ps, jobManager: jm, runner: r, cfg: cfg, logger: logger}
}

// CleanResults godoc
// @Summary      Clean test results
// @Description  Removes all test result files for a project.
// @Tags         results
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      500  {object}  map[string]any
// @Router       /projects/{project_id}/results [delete]
func (h *ResultUploadHandler) CleanResults(w http.ResponseWriter, r *http.Request) {
	projectID, ok := extractProjectID(w, r, h.cfg.ProjectsPath)
	if !ok {
		return
	}

	if err := h.runner.CleanResults(r.Context(), projectID); err != nil {
		h.logger.Error("cleaning results failed", zap.String("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error cleaning results")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"output": ""}, "Results successfully cleaned")
}

// SendResults godoc
// @Summary      Upload test results
// @Description  Uploads allure result files to a project and automatically triggers report generation. Supports JSON (deprecated), multipart/form-data, and application/gzip (tar.gz archive). The JSON mode (base64-encoded) has +33% size overhead and is deprecated in favor of tar.gz or multipart uploads.
// @Tags         results
// @Accept       json,mpfd,application/gzip
// @Produce      json
// @Param        project_id              path   string  true   "Project ID"
// @Param        force_project_creation  query  string  false  "Auto-create project if missing"
// @Param        execution_name          query  string  false  "Execution name (CI provider, e.g. 'GitHub Actions')"
// @Param        execution_from          query  string  false  "Execution from (CI build URL)"
// @Param        execution_type          query  string  false  "Execution type"
// @Param        ci_branch               query  string  false  "CI branch name"
// @Param        ci_commit_sha           query  string  false  "CI commit SHA"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Failure      413  {object}  map[string]any
// @Router       /projects/{project_id}/results [post]
func (h *ResultUploadHandler) SendResults(w http.ResponseWriter, r *http.Request) {
	projectID, ok := extractProjectID(w, r, h.cfg.ProjectsPath)
	if !ok {
		return
	}

	parentID := r.URL.Query().Get("parent_id")
	execName := r.URL.Query().Get("execution_name")
	execFrom := r.URL.Query().Get("execution_from")
	execType := r.URL.Query().Get("execution_type")
	ciBranch := r.URL.Query().Get("ci_branch")
	ciCommitSHA := r.URL.Query().Get("ci_commit_sha")

	// Ensure project exists (auto-create if requested)
	exists, err := h.store.ProjectExists(r.Context(), projectID)
	if err != nil {
		h.logger.Error("checking project existence failed", zap.String("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to check project")
		return
	}
	if !exists {
		if r.URL.Query().Get("force_project_creation") == "true" {
			if err := h.runner.CreateProject(r.Context(), projectID); err != nil && !errors.Is(err, runner.ErrProjectExists) {
				h.logger.Error("auto-creating project failed", zap.String("project_id", projectID), zap.Error(err))
				writeError(w, http.StatusInternalServerError, "failed to create project")
				return
			}
			if parentID != "" {
				// Ensure parent project exists in DB; create it if missing.
				if dbErr := h.projectStore.CreateProject(r.Context(), parentID); dbErr != nil {
					if !errors.Is(dbErr, store.ErrProjectExists) {
						h.logger.Error("db parent project registration failed", zap.String("parent_id", parentID), zap.Error(dbErr))
					}
				}
				// Register child project with parent link in DB.
				if dbErr := h.projectStore.CreateProjectWithParent(r.Context(), projectID, parentID); dbErr != nil {
					if !errors.Is(dbErr, store.ErrProjectExists) {
						h.logger.Error("db project registration failed", zap.String("project_id", projectID), zap.Error(dbErr))
					}
				}
			} else {
				// Register in database so downstream jobs (River) can reference the project.
				if dbErr := h.projectStore.CreateProject(r.Context(), projectID); dbErr != nil {
					if !errors.Is(dbErr, store.ErrProjectExists) {
						h.logger.Error("db project registration failed", zap.String("project_id", projectID), zap.Error(dbErr))
					}
				}
			}
		} else {
			writeError(w, http.StatusNotFound, fmt.Sprintf("project_id '%s' not found", projectID))
			return
		}
	}

	// Limit request body to prevent memory exhaustion (AUDIT 2.2).
	// Configurable via MAX_UPLOAD_SIZE_MB env var or max_upload_size_mb YAML key (default 100 MB).
	maxBodyBytes := int64(h.cfg.MaxUploadSizeMB) << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	processedFiles, failedFiles, err := h.parseResultsBody(r, projectID)
	if errors.Is(err, errUnsupportedContentType) {
		writeError(w, http.StatusBadRequest, "Content-Type must be application/json, multipart/form-data, or application/gzip")
		return
	}

	if err != nil {
		code := http.StatusBadRequest
		msg := err.Error()
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			code = http.StatusRequestEntityTooLarge
			msg = "request body too large"
		}
		writeError(w, code, msg)
		return
	}

	if len(failedFiles) > 0 {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("Problems with files: %v", failedFiles))
		return
	}

	// Auto-trigger report generation after successful upload.
	job := h.jobManager.Submit(projectID, runner.JobParams{
		ExecName:     execName,
		ExecFrom:     execFrom,
		ExecType:     execType,
		StoreResults: true,
		CIBranch:     ciBranch,
		CICommitSHA:  ciCommitSHA,
	})

	if h.cfg.APIResponseLessVerbose {
		writeSuccess(w, http.StatusOK, map[string]any{
			"job_id": job.ID,
		}, fmt.Sprintf("Results successfully sent for project_id '%s'", projectID))
		return
	}

	currentFileNames, _ := h.store.ListResultFiles(r.Context(), projectID)

	writeSuccess(w, http.StatusOK, map[string]any{
		"job_id":                job.ID,
		"current_files":         currentFileNames,
		"current_files_count":   len(currentFileNames),
		"failed_files":          failedFiles,
		"failed_files_count":    len(failedFiles),
		"processed_files":       processedFiles,
		"processed_files_count": len(processedFiles),
		"sent_files_count":      len(processedFiles) + len(failedFiles),
	}, fmt.Sprintf("Results successfully sent for project_id '%s'", projectID))
}

// parseResultsBody routes the request to the appropriate parser based on Content-Type.
// Returns errUnsupportedContentType when the Content-Type is not recognized.
func (h *ResultUploadHandler) parseResultsBody(r *http.Request, projectID string) (processed []string, failed []map[string]string, err error) {
	contentType := r.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(contentType, "application/json"):
		return h.sendJSONResults(r, projectID)
	case strings.HasPrefix(contentType, "multipart/form-data"):
		return h.sendMultipartResults(r, projectID)
	case contentType == "application/gzip",
		contentType == "application/x-gzip",
		contentType == "application/x-tar+gzip":
		return h.sendTarGzResults(r, projectID)
	default:
		return nil, nil, errUnsupportedContentType
	}
}

// sendJSONResults processes JSON body: {"results":[{"file_name":"...","content_base64":"..."}]}
func (h *ResultUploadHandler) sendJSONResults(r *http.Request, projectID string) (processed []string, failed []map[string]string, _ error) {
	var body struct {
		Results []struct {
			FileName      string `json:"file_name"`
			ContentBase64 string `json:"content_base64"`
		} `json:"results"`
	}

	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return nil, nil, fmt.Errorf("invalid JSON body: %w", err)
	}
	if body.Results == nil {
		return nil, nil, ErrResultsRequired
	}
	if len(body.Results) == 0 {
		return nil, nil, ErrResultsEmpty
	}

	// Check for duplicate file names
	seen := make(map[string]bool, len(body.Results))
	for _, res := range body.Results {
		if res.FileName == "" {
			return nil, nil, ErrFileNameRequired
		}
		if seen[res.FileName] {
			return nil, nil, ErrDuplicateFileNames
		}
		seen[res.FileName] = true
	}

	for i := range body.Results {
		safeName := secureFilename(body.Results[i].FileName)
		if body.Results[i].ContentBase64 == "" {
			return nil, nil, fmt.Errorf("'content_base64' required for %q: %w", body.Results[i].FileName, ErrContentBase64Required)
		}

		// Stream base64 decode directly to disk via io.Copy to avoid holding a
		// full decoded []byte in memory alongside the original base64 string.
		decoder := base64.NewDecoder(base64.StdEncoding, strings.NewReader(body.Results[i].ContentBase64))
		// Release the base64 string now that the decoder holds its own reference,
		// allowing the GC to reclaim it while io.Copy runs.
		body.Results[i].ContentBase64 = ""

		if err := h.store.WriteResultFile(r.Context(), projectID, safeName, decoder); err != nil {
			return nil, nil, fmt.Errorf("decode base64 for %q: %w", body.Results[i].FileName, err)
		}

		processed = append(processed, safeName)
	}

	return processed, failed, nil
}

// sendMultipartResults processes multipart/form-data with files[] field
func (h *ResultUploadHandler) sendMultipartResults(r *http.Request, projectID string) (processed []string, failed []map[string]string, _ error) {
	const maxMemory = 32 << 20 // 32 MB
	if err := r.ParseMultipartForm(maxMemory); err != nil {
		return nil, nil, fmt.Errorf("parse multipart form: %w", err)
	}

	files := r.MultipartForm.File["files[]"]
	if len(files) == 0 {
		return nil, nil, ErrNoFilesProvided
	}

	for _, fh := range files {
		safeName := secureFilename(fh.Filename)
		f, err := fh.Open()
		if err != nil {
			failed = append(failed, map[string]string{"file_name": safeName, "message": err.Error()})
			continue
		}

		err = h.store.WriteResultFile(r.Context(), projectID, safeName, f)
		_ = f.Close()
		if err != nil {
			failed = append(failed, map[string]string{"file_name": safeName, "message": err.Error()})
		} else {
			processed = append(processed, safeName)
		}
	}

	return processed, failed, nil
}

// sendTarGzResults extracts a tar.gz archive to a temp directory, validates
// all entries, then writes them to storage. Atomic: rejects all on any error.
func (h *ResultUploadHandler) sendTarGzResults(r *http.Request, projectID string) (processed []string, failed []map[string]string, _ error) {
	gz, err := gzip.NewReader(r.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid gzip stream: %w", err)
	}
	defer func() { _ = gz.Close() }()

	cr := &countingReader{r: gz, limit: maxDecompressedBytes}
	tr := tar.NewReader(cr)

	tmpDir, err := os.MkdirTemp("", "alluredeck-targz-*")
	if err != nil {
		return nil, nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	seen := make(map[string]bool)
	var fileNames []string
	fileCount := 0

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if cr.exceeded {
				return nil, nil, ErrArchiveDecompBomb
			}
			return nil, nil, fmt.Errorf("read tar entry: %w", err)
		}

		// Skip non-regular files (directories, symlinks, etc.).
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		// Sanitize filename.
		safeName := secureFilename(hdr.Name)
		if safeName == "." || safeName == "" {
			return nil, nil, fmt.Errorf("entry %q: %w", hdr.Name, ErrArchiveInvalidEntry)
		}

		// Reject nested paths: if the original name differs from the base name,
		// it contained directory components.
		cleanName := filepath.Clean(hdr.Name)
		if cleanName != safeName {
			return nil, nil, fmt.Errorf("entry %q resolves to nested path: %w", hdr.Name, ErrArchiveNestedPath)
		}

		// Reject duplicates.
		if seen[safeName] {
			return nil, nil, fmt.Errorf("entry %q: %w", safeName, ErrArchiveDuplicateFile)
		}
		seen[safeName] = true

		// Enforce file count limit.
		fileCount++
		if fileCount > maxArchiveFileCount {
			return nil, nil, fmt.Errorf("archive has more than %d files: %w", maxArchiveFileCount, ErrArchiveTooManyFiles)
		}

		// Write to temp dir.
		tmpPath := filepath.Join(tmpDir, safeName)
		f, err := os.Create(tmpPath)
		if err != nil {
			return nil, nil, fmt.Errorf("create temp file %q: %w", safeName, err)
		}
		if _, err := io.Copy(f, tr); err != nil {
			_ = f.Close()
			if cr.exceeded {
				return nil, nil, ErrArchiveDecompBomb
			}
			return nil, nil, fmt.Errorf("extract %q: %w", safeName, err)
		}
		if err := f.Close(); err != nil {
			return nil, nil, fmt.Errorf("close temp file %q: %w", safeName, err)
		}

		fileNames = append(fileNames, safeName)
	}

	if len(fileNames) == 0 {
		return nil, nil, ErrArchiveEmpty
	}

	// Sort for deterministic output.
	sort.Strings(fileNames)

	// Write validated files to storage.
	for _, name := range fileNames {
		tmpPath := filepath.Join(tmpDir, name)
		f, err := os.Open(tmpPath)
		if err != nil {
			return nil, nil, fmt.Errorf("reopen temp file %q: %w", name, err)
		}
		err = h.store.WriteResultFile(r.Context(), projectID, name, f)
		_ = f.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("write result file %q: %w", name, err)
		}
		processed = append(processed, name)
	}

	return processed, nil, nil
}
