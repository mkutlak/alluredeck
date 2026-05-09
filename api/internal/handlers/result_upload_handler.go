package handlers

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// gzipMagic is the two-byte signature that prefixes every valid gzip stream
// (RFC 1952 §2.3.1). The async upload path peeks for this before streaming
// the body to MinIO so a malformed/non-gzip body fails fast at the edge
// instead of being staged and crashing the worker later.
var gzipMagic = []byte{0x1f, 0x8b}

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
// resolveProjectID tries to parse the path value as a numeric ID.
// If it fails, treats it as a slug and looks up the project.
// Returns (id, slug, storageKey, found).
func (h *ResultUploadHandler) resolveProjectID(ctx context.Context, pathValue string) (int64, string, string, bool) {
	if id, err := strconv.ParseInt(pathValue, 10, 64); err == nil {
		p, err := h.projectStore.GetProject(ctx, id)
		if err == nil {
			return p.ID, p.Slug, p.StorageKey, true
		}
	}
	// Treat as slug — look up by slug.
	p, err := h.projectStore.GetProjectBySlugAny(ctx, pathValue)
	if err == nil {
		return p.ID, p.Slug, p.StorageKey, true
	}
	return 0, pathValue, pathValue, false
}

func (h *ResultUploadHandler) CleanResults(w http.ResponseWriter, r *http.Request) {
	pathValue := r.PathValue("project_id")
	projectID, slug, storageKey, found := h.resolveProjectID(r.Context(), pathValue)
	if !found {
		writeError(w, http.StatusNotFound, fmt.Sprintf("project '%s' not found", pathValue))
		return
	}
	_ = projectID // DB operations would use this

	if err := h.runner.CleanResults(r.Context(), storageKey); err != nil {
		h.logger.Error("cleaning results failed", zap.String("slug", slug), zap.Error(err))
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
// @Param        ci_pipeline_id          query  string  false  "CI pipeline ID"
// @Param        ci_pipeline_url         query  string  false  "CI pipeline URL"
// @Param        async                   query  bool    false  "Stream tar.gz to staging blob and return 202 immediately (only honored for application/gzip uploads)"
// @Success      200  {object}  map[string]any
// @Success      202  {object}  map[string]any  "Accepted (async=true): returns {job_id, batch_id} once the tar.gz is staged"
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Failure      413  {object}  map[string]any
// @Router       /projects/{project_id}/results [post]
func (h *ResultUploadHandler) SendResults(w http.ResponseWriter, r *http.Request) {
	pathValue := r.PathValue("project_id")
	projectID, slug, storageKey, found := h.resolveProjectID(r.Context(), pathValue)

	batchID := runner.GenerateBatchID()

	parentIDStr := r.URL.Query().Get("parent_id")

	execName := r.URL.Query().Get("execution_name")
	execFrom := r.URL.Query().Get("execution_from")
	execType := r.URL.Query().Get("execution_type")
	ciBranch := r.URL.Query().Get("ci_branch")
	ciCommitSHA := r.URL.Query().Get("ci_commit_sha")
	ciPipelineID := r.URL.Query().Get("ci_pipeline_id")
	ciPipelineURL := r.URL.Query().Get("ci_pipeline_url")

	if !found {
		if r.URL.Query().Get("force_project_creation") == "true" {
			slug = pathValue // use path value as slug for new project
			storageKey = slug
			// Create filesystem project via runner.
			if err := h.runner.CreateProject(r.Context(), storageKey); err != nil && !errors.Is(err, runner.ErrProjectExists) {
				h.logger.Error("auto-creating project failed", zap.String("slug", slug), zap.Error(err))
				writeError(w, http.StatusInternalServerError, "failed to create project")
				return
			}
			if parentIDStr != "" {
				var parentID int64
				if id, parseErr := strconv.ParseInt(parentIDStr, 10, 64); parseErr == nil {
					parentID = id
				} else {
					parent, lookupErr := h.projectStore.GetProjectBySlugAny(r.Context(), parentIDStr)
					if lookupErr != nil {
						// Parent slug not found — auto-create it as a top-level project.
						// force_project_creation=true means "recreate whatever I reference",
						// which must include the parent after a full-wipe reset.
						if fsErr := h.runner.CreateProject(r.Context(), parentIDStr); fsErr != nil && !errors.Is(fsErr, runner.ErrProjectExists) {
							h.logger.Error("auto-creating parent project failed", zap.String("slug", parentIDStr), zap.Error(fsErr))
							writeError(w, http.StatusInternalServerError, "failed to create parent project")
							return
						}
						created, dbErr := h.projectStore.CreateProject(r.Context(), parentIDStr)
						if dbErr != nil && !errors.Is(dbErr, store.ErrProjectExists) {
							h.logger.Error("db parent registration failed", zap.String("slug", parentIDStr), zap.Error(dbErr))
							writeError(w, http.StatusInternalServerError, "failed to register parent project")
							return
						}
						if created != nil {
							parentID = created.ID
						} else {
							// Raced with a concurrent creator; look it up again.
							p, err := h.projectStore.GetProjectBySlugAny(r.Context(), parentIDStr)
							if err != nil {
								h.logger.Error("parent project lookup after create failed", zap.String("slug", parentIDStr), zap.Error(err))
								writeError(w, http.StatusInternalServerError, "failed to resolve parent project")
								return
							}
							parentID = p.ID
						}
					} else {
						parentID = parent.ID
					}
				}
				// Register child project with parent link.
				project, dbErr := h.projectStore.CreateProjectWithParent(r.Context(), slug, parentID)
				if dbErr != nil {
					if !errors.Is(dbErr, store.ErrProjectExists) {
						h.logger.Error("db project registration failed", zap.String("slug", slug), zap.Error(dbErr))
					}
				}
				if project != nil {
					projectID = project.ID
					storageKey = project.StorageKey
				}
			} else {
				// Register in database so downstream jobs (River) can reference the project.
				project, dbErr := h.projectStore.CreateProject(r.Context(), slug)
				if dbErr != nil {
					if !errors.Is(dbErr, store.ErrProjectExists) {
						h.logger.Error("db project registration failed", zap.String("slug", slug), zap.Error(dbErr))
					}
				}
				if project != nil {
					projectID = project.ID
					storageKey = project.StorageKey
				}
			}
			// If project already existed, look it up.
			if projectID == 0 {
				if p, err := h.projectStore.GetProjectBySlugAny(r.Context(), slug); err == nil {
					projectID = p.ID
					storageKey = p.StorageKey
				}
			}
		} else {
			writeError(w, http.StatusNotFound, fmt.Sprintf("project '%s' not found", pathValue))
			return
		}
	}

	// Limit request body to prevent memory exhaustion (AUDIT 2.2).
	// Configurable via MAX_UPLOAD_SIZE_MB env var or max_upload_size_mb YAML key (default 100 MB).
	maxBodyBytes := int64(h.cfg.MaxUploadSizeMB) << 20
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	// Async upload: stream the tar.gz body straight to storage at
	// staging/{batchID}.tar.gz, enqueue a worker to extract + generate, and
	// return 202. Only honored for tar.gz Content-Types — JSON/multipart
	// callers fall through to the sync path.
	if r.URL.Query().Get("async") == "true" && isTarGzContentType(r.Header.Get("Content-Type")) {
		h.sendResultsAsync(w, r, projectID, slug, storageKey, batchID, execName, execFrom, execType, ciBranch, ciCommitSHA, ciPipelineID, ciPipelineURL)
		return
	}

	processedFiles, failedFiles, err := h.parseResultsBody(r, storageKey, batchID)
	if errors.Is(err, errUnsupportedContentType) {
		writeError(w, http.StatusBadRequest, "Content-Type must be application/json, multipart/form-data, or application/gzip")
		return
	}

	if err != nil {
		code := http.StatusBadRequest
		msg := err.Error()
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
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
	job := h.jobManager.Submit(r.Context(), projectID, slug, runner.JobParams{
		StorageKey:    storageKey,
		BatchID:       batchID,
		ExecName:      execName,
		ExecFrom:      execFrom,
		ExecType:      execType,
		StoreResults:  true,
		CIBranch:      ciBranch,
		CICommitSHA:   ciCommitSHA,
		CIPipelineID:  ciPipelineID,
		CIPipelineURL: ciPipelineURL,
	})

	if h.cfg.APIResponseLessVerbose {
		writeSuccess(w, http.StatusOK, map[string]any{
			"job_id": job.ID,
		}, fmt.Sprintf("Results successfully sent for project '%s'", slug))
		return
	}

	currentFileNames, _ := h.store.ListResultFiles(r.Context(), storageKey, batchID)

	writeSuccess(w, http.StatusOK, map[string]any{
		"job_id":                job.ID,
		"batch_id":              batchID,
		"current_files":         currentFileNames,
		"current_files_count":   len(currentFileNames),
		"failed_files":          failedFiles,
		"failed_files_count":    len(failedFiles),
		"processed_files":       processedFiles,
		"processed_files_count": len(processedFiles),
		"sent_files_count":      len(processedFiles) + len(failedFiles),
	}, fmt.Sprintf("Results successfully sent for project '%s'", slug))
}

// parseResultsBody routes the request to the appropriate parser based on Content-Type.
// Returns errUnsupportedContentType when the Content-Type is not recognized.
func (h *ResultUploadHandler) parseResultsBody(r *http.Request, storageKey, batchID string) (processed []string, failed []map[string]string, err error) {
	contentType := r.Header.Get("Content-Type")
	switch {
	case strings.HasPrefix(contentType, "application/json"):
		return h.sendJSONResults(r, storageKey, batchID)
	case strings.HasPrefix(contentType, "multipart/form-data"):
		return h.sendMultipartResults(r, storageKey, batchID)
	case contentType == "application/gzip",
		contentType == "application/x-gzip",
		contentType == "application/x-tar+gzip":
		return h.sendTarGzResults(r, storageKey, batchID)
	default:
		return nil, nil, errUnsupportedContentType
	}
}

// sendJSONResults processes JSON body: {"results":[{"file_name":"...","content_base64":"..."}]}
func (h *ResultUploadHandler) sendJSONResults(r *http.Request, storageKey, batchID string) (processed []string, failed []map[string]string, _ error) {
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

		if err := h.store.WriteResultFile(r.Context(), storageKey, batchID, safeName, decoder); err != nil {
			return nil, nil, fmt.Errorf("decode base64 for %q: %w", body.Results[i].FileName, err)
		}

		processed = append(processed, safeName)
	}

	return processed, failed, nil
}

// sendMultipartResults processes multipart/form-data with files[] field
func (h *ResultUploadHandler) sendMultipartResults(r *http.Request, storageKey, batchID string) (processed []string, failed []map[string]string, _ error) {
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

		err = h.store.WriteResultFile(r.Context(), storageKey, batchID, safeName, f)
		_ = f.Close()
		if err != nil {
			failed = append(failed, map[string]string{"file_name": safeName, "message": err.Error()})
		} else {
			processed = append(processed, safeName)
		}
	}

	return processed, failed, nil
}

// sendTarGzResults extracts a tar.gz archive and streams entries directly to
// storage. Validation (decompression bomb, nested path, dup name, file count,
// empty archive) runs in the shared extractor.
func (h *ResultUploadHandler) sendTarGzResults(r *http.Request, storageKey, batchID string) (processed []string, failed []map[string]string, _ error) {
	maxFiles := maxArchiveFileCount
	if h.cfg != nil && h.cfg.MaxArchiveFileCount > 0 {
		maxFiles = h.cfg.MaxArchiveFileCount
	}
	written, err := runner.ExtractTarGzToStorage(r.Context(), h.store, storageKey, batchID, r.Body, runner.TarExtractOptions{
		MaxDecompressedBytes: maxDecompressedBytes,
		MaxFileCount:         maxFiles,
		Concurrency:          uploadWriteConcurrency(h.cfg),
	})
	if err != nil {
		return nil, nil, err
	}
	return written, nil, nil
}

// uploadWriteConcurrency returns the configured concurrency for parallel
// upload writes, falling back to a safe default if unset or non-positive.
func uploadWriteConcurrency(cfg *config.Config) int {
	if cfg != nil && cfg.UploadWriteConcurrency > 0 {
		return cfg.UploadWriteConcurrency
	}
	return 32
}

// isTarGzContentType reports whether the given Content-Type header value
// matches one of the supported tar.gz upload content types.
func isTarGzContentType(ct string) bool {
	return ct == "application/gzip" ||
		ct == "application/x-gzip" ||
		ct == "application/x-tar+gzip"
}

// sendResultsAsync streams the request body to a staging blob and enqueues a
// River job to extract and generate the report. It returns 202 with
// {job_id, batch_id} on success.
//
// Validation that runs at the edge: the gzip magic bytes (1f 8b) are peeked
// before any byte hits storage. The body length cap is already enforced by
// the http.MaxBytesReader applied by SendResults. All deeper validation
// (decompression bomb, nested paths, dup names, file count, empty archive)
// runs in the worker.
func (h *ResultUploadHandler) sendResultsAsync(w http.ResponseWriter, r *http.Request, projectID int64, slug, storageKey, batchID, execName, execFrom, execType, ciBranch, ciCommitSHA, ciPipelineID, ciPipelineURL string) {
	// Peek the first two bytes for the gzip magic without consuming them so
	// the staged blob still contains a valid gzip stream.
	br := bufio.NewReader(r.Body)
	head, err := br.Peek(len(gzipMagic))
	if err != nil {
		// Treat MaxBytes errors as 413; everything else is a 400.
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid gzip stream: "+err.Error())
		return
	}
	if !bytesEqual(head, gzipMagic) {
		writeError(w, http.StatusBadRequest, "invalid gzip stream: missing gzip magic bytes")
		return
	}

	stagingKey := "staging/" + batchID + ".tar.gz"
	if err := h.store.WriteRawBlob(r.Context(), stagingKey, br); err != nil {
		// The blob may be partially uploaded — do NOT enqueue a job. Surface
		// the error and let the caller retry. http.MaxBytesError still
		// surfaces at this layer when the body exceeds the configured cap.
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
			return
		}
		h.logger.Error("async upload: write staging blob failed",
			zap.String("slug", slug), zap.String("staging_key", stagingKey), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to stage upload")
		return
	}

	job := h.jobManager.SubmitStagedTarGz(r.Context(), projectID, slug, runner.StagedTarGzParams{
		StorageKey:    storageKey,
		BatchID:       batchID,
		StagingKey:    stagingKey,
		ExecName:      execName,
		ExecFrom:      execFrom,
		ExecType:      execType,
		StoreResults:  true,
		CIBranch:      ciBranch,
		CICommitSHA:   ciCommitSHA,
		CIPipelineID:  ciPipelineID,
		CIPipelineURL: ciPipelineURL,
	})

	writeSuccess(w, http.StatusAccepted, map[string]any{
		"job_id":   job.ID,
		"batch_id": batchID,
	}, fmt.Sprintf("Results staged for project '%s' (async)", slug))
}

// bytesEqual reports whether a and b are byte-wise equal. Avoids importing
// bytes for a single equality check.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
