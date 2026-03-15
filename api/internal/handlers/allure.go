package handlers

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// Sentinel errors for HTTP request validation.
var (
	ErrProjectRequired       = errors.New("project_id is required")
	ErrProjectTooLong        = errors.New("project_id must not exceed 100 characters")
	ErrProjectInvalidChars   = errors.New("project_id contains invalid characters")
	ErrProjectReserved       = errors.New("project_id is reserved")
	ErrProjectInvalid        = errors.New("invalid project_id")
	ErrResultsRequired       = errors.New("'results' array is required in the body")
	ErrResultsEmpty          = errors.New("'results' array is empty")
	ErrFileNameRequired      = errors.New("'file_name' attribute is required for all results")
	ErrDuplicateFileNames    = errors.New("duplicated file names in 'results'")
	ErrContentBase64Required = errors.New("'content_base64' attribute is required")
	ErrNoFilesProvided       = errors.New("no files provided in 'files[]' field")

	// tar.gz archive validation errors.
	ErrArchiveEmpty         = errors.New("archive contains no files")
	ErrArchiveTooManyFiles  = errors.New("archive exceeds maximum file count")
	ErrArchiveDecompBomb    = errors.New("decompressed archive exceeds size limit")
	ErrArchiveNestedPath    = errors.New("archive entry contains nested path")
	ErrArchiveInvalidEntry  = errors.New("archive entry is not a regular file")
	ErrArchiveDuplicateFile = errors.New("archive contains duplicate file names")

	// report_id validation errors.
	ErrReportIDRequired = errors.New("report_id is required")
	ErrReportIDInvalid  = errors.New("report_id must be 'latest' or a positive integer")

	// ticket_url validation errors.
	ErrTicketURLInvalidScheme = errors.New("ticket_url must use http or https scheme")

	// errUnsupportedContentType is returned by parseResultsBody when the
	// Content-Type header is not application/json, multipart/form-data, or application/gzip.
	errUnsupportedContentType = errors.New("unsupported Content-Type")
)

// Package-level limits for tar.gz extraction (vars for testability).
//
//nolint:gochecknoglobals // overridden in tests to avoid creating huge archives
var (
	maxDecompressedBytes int64 = 1 << 30 // 1 GB
	maxArchiveFileCount        = 5000
)

// AllureHandler handles HTTP requests for Allure report management.
type AllureHandler struct {
	cfg             *config.Config
	runner          *runner.Allure
	jobManager      runner.JobQueuer
	projectStore    store.ProjectStorer
	buildStore      store.BuildStorer
	knownIssueStore store.KnownIssueStorer
	testResultStore store.TestResultStorer
	searchStore     store.SearchStorer
	store           storage.Store
	branchStore     store.BranchStorer
	logger          *zap.Logger
}

// NewAllureHandler creates and returns a new AllureHandler.
func NewAllureHandler(cfg *config.Config, r *runner.Allure, jobManager runner.JobQueuer, projectStore store.ProjectStorer, buildStore store.BuildStorer, knownIssueStore store.KnownIssueStorer, testResultStore store.TestResultStorer, searchStore store.SearchStorer, st storage.Store, logger *zap.Logger) *AllureHandler {
	return &AllureHandler{
		cfg:             cfg,
		runner:          r,
		jobManager:      jobManager,
		projectStore:    projectStore,
		buildStore:      buildStore,
		knownIssueStore: knownIssueStore,
		testResultStore: testResultStore,
		searchStore:     searchStore,
		store:           st,
		logger:          logger,
	}
}

// SetBranchStore configures an optional branch store for branch-aware filtering.
func (h *AllureHandler) SetBranchStore(bs store.BranchStorer) {
	h.branchStore = bs
}

// ProjectEntry holds a single project in the paginated project listing.
type ProjectEntry struct {
	ProjectID string   `json:"project_id"`
	CreatedAt string   `json:"created_at"`
	Tags      []string `json:"tags"`
}

// reservedProjectNames lists names that clash with API route segments
//
//nolint:gochecknoglobals // read-only constant-like lookup table for reserved project names
var reservedProjectNames = map[string]bool{
	"login":   true,
	"logout":  true,
	"version": true,
	"config":  true,
	"swagger": true,
	".":       true,
	"..":      true,
}

// validateProjectID rejects project IDs that could cause path traversal or
// shadow API routes. Returns an error message suitable for the API response.
func validateProjectID(projectsDir, projectID string) error {
	if projectID == "" {
		return ErrProjectRequired
	}
	if len(projectID) > 100 {
		return ErrProjectTooLong
	}
	if strings.ContainsAny(projectID, "/\\") || strings.Contains(projectID, "..") {
		return ErrProjectInvalidChars
	}
	if reservedProjectNames[projectID] {
		return fmt.Errorf("project_id %q: %w", projectID, ErrProjectReserved)
	}
	// Belt-and-suspenders: verify the resolved path stays under projectsDir
	resolved := filepath.Join(projectsDir, projectID)
	rel, err := filepath.Rel(projectsDir, resolved)
	if err != nil || strings.HasPrefix(rel, "..") {
		return ErrProjectInvalid
	}
	return nil
}

// safeProjectID resolves to "default" when empty, then validates
func safeProjectID(projectsDir, raw string) (string, error) {
	if raw == "" {
		raw = "default"
	}
	if err := validateProjectID(projectsDir, raw); err != nil {
		return "", err
	}
	return raw, nil
}

// validateReportID rejects report IDs that could cause path traversal.
// Accepts "latest" or non-empty all-digit strings (positive integers).
func validateReportID(reportID string) error {
	if reportID == "" {
		return ErrReportIDRequired
	}
	if reportID == "latest" {
		return nil
	}
	for _, c := range reportID {
		if c < '0' || c > '9' {
			return ErrReportIDInvalid
		}
	}
	return nil
}

// validateTicketURL rejects URLs with non-http(s) schemes (e.g. javascript:, data:).
// An empty URL is allowed (optional field). Returns an error for dangerous schemes.
func validateTicketURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ErrTicketURLInvalidScheme
	}
	switch parsed.Scheme {
	case "http", "https":
		return nil
	default:
		return ErrTicketURLInvalidScheme
	}
}

// extractProjectID extracts, unescapes, and validates the "project_id" path
// parameter. On failure it writes a 400 response and returns ("", false).
func (h *AllureHandler) extractProjectID(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw := r.PathValue("project_id")
	unescaped, err := url.PathUnescape(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid project_id encoding")
		return "", false
	}
	projectID, err := safeProjectID(h.cfg.ProjectsPath, unescaped)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	return projectID, true
}

// extractReportID extracts and validates the "report_id" path parameter.
// On failure it writes a 400 response and returns ("", false).
func extractReportID(w http.ResponseWriter, r *http.Request) (string, bool) {
	reportID := r.PathValue("report_id")
	if err := validateReportID(reportID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	return reportID, true
}

// GetProjects godoc
// @Summary      List projects
// @Description  Returns a paginated list of all existing projects.
// @Tags         projects
// @Produce      json
// @Param        page      query  int  false  "Page number"      default(1)
// @Param        per_page  query  int  false  "Items per page"   default(20)
// @Success      200  {object}  map[string]any
// @Failure      500  {object}  map[string]any
// @Router       /projects [get]
func (h *AllureHandler) GetProjects(w http.ResponseWriter, r *http.Request) {
	pg := parsePagination(r)
	tag := r.URL.Query().Get("tag")

	dbProjects, total, err := h.projectStore.ListProjectsPaginated(r.Context(), pg.Page, pg.PerPage, tag)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Error listing projects: %v", err))
		return
	}

	entries := make([]ProjectEntry, 0, len(dbProjects))
	for _, p := range dbProjects {
		tags := p.Tags
		if tags == nil {
			tags = []string{}
		}
		entries = append(entries, ProjectEntry{
			ProjectID: p.ID,
			CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
			Tags:      tags,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":       entries,
		"metadata":   map[string]string{"message": "Projects successfully obtained"},
		"pagination": newPaginationMeta(pg.Page, pg.PerPage, total),
	})
}

// CreateProject godoc
// @Summary      Create a project
// @Description  Creates a new project directory and registers it in the database.
// @Tags         projects
// @Accept       json
// @Produce      json
// @Param        body  body      object  true  "Project ID"
// @Success      201   {object}  map[string]any
// @Failure      400   {object}  map[string]any
// @Failure      409   {object}  map[string]any
// @Router       /projects [post]
func (h *AllureHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var reqBody struct {
		ID string `json:"id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	projectID := strings.TrimSpace(reqBody.ID)
	if err := validateProjectID(h.cfg.ProjectsPath, projectID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	err := h.runner.CreateProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, runner.ErrProjectExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Error creating project: %v", err))
		return
	}

	// Register in database (INSERT OR IGNORE so an already-synced project is not an error).
	if dbErr := h.projectStore.CreateProject(r.Context(), projectID); dbErr != nil {
		if !errors.Is(dbErr, store.ErrProjectExists) {
			// Log but don't fail — filesystem project was already created successfully.
			h.logger.Error("db project registration failed", zap.String("project_id", projectID), zap.Error(dbErr))
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"data":     ProjectEntry{ProjectID: projectID},
		"metadata": map[string]string{"message": "Project successfully created"},
	})
}

// GenerateReport godoc
// @Summary      Generate a report
// @Description  Triggers Allure report generation for the specified project.
// @Tags         reports
// @Produce      json
// @Param        project_id      path   string  true   "Project ID"
// @Param        execution_name  query  string  false  "Execution name (CI provider, e.g. 'GitHub Actions')"
// @Param        execution_from  query  string  false  "Execution from (CI build URL)"
// @Param        execution_type  query  string  false  "Execution type"
// @Param        store_results   query  string  false  "Store results (true/1)"
// @Param        ci_branch       query  string  false  "CI branch name"
// @Param        ci_commit_sha   query  string  false  "CI commit SHA"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      500  {object}  map[string]any
// @Router       /projects/{project_id}/reports [post]
func (h *AllureHandler) GenerateReport(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.extractProjectID(w, r)
	if !ok {
		return
	}

	execName := r.URL.Query().Get("execution_name")
	execFrom := r.URL.Query().Get("execution_from")
	execType := r.URL.Query().Get("execution_type")
	ciBranch := r.URL.Query().Get("ci_branch")
	ciCommitSHA := r.URL.Query().Get("ci_commit_sha")
	storeResultsStr := r.URL.Query().Get("store_results")
	var storeResults bool
	if storeResultsStr == "" {
		// When store_results is not specified, default to the server's KEEP_HISTORY setting.
		storeResults = h.cfg.KeepHistory
	} else {
		storeResults = storeResultsStr == "1" || strings.EqualFold(storeResultsStr, "true")
	}

	params := runner.JobParams{
		ExecName:     execName,
		ExecFrom:     execFrom,
		ExecType:     execType,
		StoreResults: storeResults,
		CIBranch:     ciBranch,
		CICommitSHA:  ciCommitSHA,
	}
	job := h.jobManager.Submit(projectID, params)
	writeJSON(w, http.StatusAccepted, map[string]any{
		"data":     map[string]string{"job_id": job.ID},
		"metadata": map[string]string{"message": "Report generation queued"},
	})
}

// GetJobStatus godoc
// @Summary      Get async report generation job status
// @Description  Returns the current status of an async report generation job.
// @Tags         reports
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Param        job_id      path  string  true  "Job ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /projects/{project_id}/jobs/{job_id} [get]
func (h *AllureHandler) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.extractProjectID(w, r)
	if !ok {
		return
	}

	jobID := r.PathValue("job_id")
	job := h.jobManager.Get(jobID)
	if job == nil || job.ProjectID != projectID {
		writeError(w, http.StatusNotFound, "job not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     job,
		"metadata": map[string]string{"message": "Job status retrieved"},
	})
}

// CleanHistory godoc
// @Summary      Clean report history
// @Description  Removes all historical report snapshots for a project.
// @Tags         reports
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      500  {object}  map[string]any
// @Router       /projects/{project_id}/reports/history [delete]
func (h *AllureHandler) CleanHistory(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.extractProjectID(w, r)
	if !ok {
		return
	}

	if err := h.runner.CleanHistory(r.Context(), projectID); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Error cleaning history: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     map[string]string{"output": ""},
		"metadata": map[string]string{"message": "History successfully cleaned"},
	})
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
func (h *AllureHandler) CleanResults(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.extractProjectID(w, r)
	if !ok {
		return
	}

	if err := h.runner.CleanResults(r.Context(), projectID); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Error cleaning results: %v", err))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     map[string]string{"output": ""},
		"metadata": map[string]string{"message": "Results successfully cleaned"},
	})
}

// SendResults godoc
// @Summary      Upload test results
// @Description  Uploads allure result files to a project. Supports JSON (deprecated), multipart/form-data, and application/gzip (tar.gz archive). The JSON mode (base64-encoded) has +33% size overhead and is deprecated in favor of tar.gz or multipart uploads.
// @Tags         results
// @Accept       json,mpfd,application/gzip
// @Produce      json
// @Param        project_id              path   string  true   "Project ID"
// @Param        force_project_creation  query  string  false  "Auto-create project if missing"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Failure      413  {object}  map[string]any
// @Router       /projects/{project_id}/results [post]
func (h *AllureHandler) SendResults(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.extractProjectID(w, r)
	if !ok {
		return
	}

	// Ensure project exists (auto-create if requested)
	exists, err := h.store.ProjectExists(r.Context(), projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to check project: %v", err))
		return
	}
	if !exists {
		if r.URL.Query().Get("force_project_creation") == "true" {
			if err := h.runner.CreateProject(r.Context(), projectID); err != nil && !errors.Is(err, runner.ErrProjectExists) {
				writeError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to create project: %v", err))
				return
			}
			// Register in database so downstream jobs (River) can reference the project.
			if dbErr := h.projectStore.CreateProject(r.Context(), projectID); dbErr != nil {
				if !errors.Is(dbErr, store.ErrProjectExists) {
					h.logger.Error("db project registration failed", zap.String("project_id", projectID), zap.Error(dbErr))
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

	if h.cfg.APIResponseLessVerbose {
		writeJSON(w, http.StatusOK, map[string]any{
			"metadata": map[string]string{"message": fmt.Sprintf("Results successfully sent for project_id '%s'", projectID)},
		})
		return
	}

	currentFileNames, _ := h.store.ListResultFiles(r.Context(), projectID)

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"current_files":         currentFileNames,
			"current_files_count":   len(currentFileNames),
			"failed_files":          failedFiles,
			"failed_files_count":    len(failedFiles),
			"processed_files":       processedFiles,
			"processed_files_count": len(processedFiles),
			"sent_files_count":      len(processedFiles) + len(failedFiles),
		},
		"metadata": map[string]string{"message": fmt.Sprintf("Results successfully sent for project_id '%s'", projectID)},
	})
}

// parseResultsBody routes the request to the appropriate parser based on Content-Type.
// Returns errUnsupportedContentType when the Content-Type is not recognized.
func (h *AllureHandler) parseResultsBody(r *http.Request, projectID string) (processed []string, failed []map[string]string, err error) {
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
func (h *AllureHandler) sendJSONResults(r *http.Request, projectID string) (processed []string, failed []map[string]string, _ error) {
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
func (h *AllureHandler) sendMultipartResults(r *http.Request, projectID string) (processed []string, failed []map[string]string, _ error) {
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

// countingReader wraps an io.Reader and tracks cumulative bytes read.
// When the limit is exceeded it returns an explicit error instead of silently
// truncating (which io.LimitReader would do).
type countingReader struct {
	r        io.Reader
	n        int64
	limit    int64
	exceeded bool
}

func (cr *countingReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	cr.n += int64(n)
	if cr.n > cr.limit {
		cr.exceeded = true
		return n, fmt.Errorf("decompressed size exceeds %d bytes: %w", cr.limit, ErrArchiveDecompBomb)
	}
	return n, err
}

// sendTarGzResults extracts a tar.gz archive to a temp directory, validates
// all entries, then writes them to storage. Atomic: rejects all on any error.
func (h *AllureHandler) sendTarGzResults(r *http.Request, projectID string) (processed []string, failed []map[string]string, _ error) {
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

// secureFilename strips path components so only the base filename remains
func secureFilename(name string) string {
	return filepath.Base(filepath.Clean(name))
}

// ReportHistoryEntry holds metadata for a single generated report.
type ReportHistoryEntry struct {
	ReportID       string           `json:"report_id"`
	IsLatest       bool             `json:"is_latest"`
	GeneratedAt    *string          `json:"generated_at"`
	DurationMs     *int64           `json:"duration_ms"`
	Statistic      *AllureStatistic `json:"statistic"`
	FlakyCount     *int             `json:"flaky_count,omitempty"`
	RetriedCount   *int             `json:"retried_count,omitempty"`
	NewFailedCount *int             `json:"new_failed_count,omitempty"`
	NewPassedCount *int             `json:"new_passed_count,omitempty"`
	CIProvider     *string          `json:"ci_provider,omitempty"`
	CIBuildURL     *string          `json:"ci_build_url,omitempty"`
	CIBranch       *string          `json:"ci_branch,omitempty"`
	CICommitSHA    *string          `json:"ci_commit_sha,omitempty"`
}

// AllureStatistic mirrors the statistic block in Allure's widgets/summary.json.
type AllureStatistic struct {
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Broken  int `json:"broken"`
	Skipped int `json:"skipped"`
	Unknown int `json:"unknown"`
	Total   int `json:"total"`
}

// allureSummaryFile is the shape of widgets/summary.json we care about.
type allureSummaryFile struct {
	Statistic *AllureStatistic `json:"statistic"`
	Time      *struct {
		Stop     int64 `json:"stop"`
		Duration int64 `json:"duration"`
	} `json:"time"`
}

// GetReportHistory returns metadata for all generated reports of a project.
// Numbered reports are served from the database (cached stats, no filesystem scan).
// The "latest" entry is still read from the filesystem since it is always regenerated.
// GetReportHistory godoc
// @Summary      Get report history
// @Description  Returns paginated metadata for all generated reports of a project.
// @Tags         reports
// @Produce      json
// @Param        project_id  path   string  true   "Project ID"
// @Param        page        query  int     false  "Page number"     default(1)
// @Param        per_page    query  int     false  "Items per page"  default(20)
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      500  {object}  map[string]any
// @Router       /projects/{project_id}/reports [get]
func (h *AllureHandler) GetReportHistory(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.extractProjectID(w, r)
	if !ok {
		return
	}

	pg := parsePagination(r)

	// Resolve optional branch filter.
	var branchID *int64
	if branchName := r.URL.Query().Get("branch"); branchName != "" && h.branchStore != nil {
		if br, err := h.branchStore.GetByName(r.Context(), projectID, branchName); err == nil {
			branchID = &br.ID
		}
	}

	// Fetch numbered builds from DB (sorted descending by build_order).
	builds, total, err := h.buildStore.ListBuildsPaginatedBranch(r.Context(), projectID, pg.Page, pg.PerPage, branchID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Error reading report history: %v", err))
		return
	}

	// Initialize as empty slice so JSON encodes [] instead of null when there are no reports.
	reports := make([]ReportHistoryEntry, 0)

	// Check for "latest" dir via store — always regenerated, not tracked in DB.
	// The "latest" entry is always prepended (not counted against pagination).
	if exists, _ := h.store.LatestReportExists(r.Context(), projectID); exists {
		entry := h.buildReportEntry(r.Context(), projectID, "latest")
		entry.IsLatest = true
		reports = append(reports, entry)
	}

	// Convert DB builds to response entries.
	for i := range builds {
		reports = append(reports, buildEntryFromDB(&builds[i]))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"project_id": projectID,
			"reports":    reports,
		},
		"metadata":   map[string]string{"message": "Report history successfully obtained"},
		"pagination": newPaginationMeta(pg.Page, pg.PerPage, total),
	})
}

// buildEntryFromDB converts a store.Build to a ReportHistoryEntry without filesystem I/O.
func buildEntryFromDB(b *store.Build) ReportHistoryEntry {
	reportID := strconv.Itoa(b.BuildOrder)
	entry := ReportHistoryEntry{
		ReportID: reportID,
		IsLatest: b.IsLatest,
	}
	t := b.CreatedAt.UTC().Format(time.RFC3339)
	entry.GeneratedAt = &t
	entry.DurationMs = b.DurationMs
	entry.FlakyCount = b.FlakyCount
	entry.RetriedCount = b.RetriedCount
	entry.NewFailedCount = b.NewFailedCount
	entry.NewPassedCount = b.NewPassedCount
	entry.CIProvider = b.CIProvider
	entry.CIBuildURL = b.CIBuildURL
	entry.CIBranch = b.CIBranch
	entry.CICommitSHA = b.CICommitSHA

	if b.StatTotal != nil && *b.StatTotal > 0 {
		entry.Statistic = &AllureStatistic{
			Passed:  derefInt(b.StatPassed),
			Failed:  derefInt(b.StatFailed),
			Broken:  derefInt(b.StatBroken),
			Skipped: derefInt(b.StatSkipped),
			Unknown: derefInt(b.StatUnknown),
			Total:   *b.StatTotal,
		}
	}
	return entry
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// readJSONViaStore reads a project-relative path via the store and unmarshals JSON into v.
// Returns true only when both the read and unmarshal succeed.
func (h *AllureHandler) readJSONViaStore(ctx context.Context, projectID, relPath string, v any) bool {
	data, err := h.store.ReadFile(ctx, projectID, relPath)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, v) == nil
}

// EnvironmentEntry represents one row in the Allure environment widget.
type EnvironmentEntry struct {
	Name   string   `json:"name"`
	Values []string `json:"values"`
}

// GetReportEnvironment godoc
// @Summary      Get report environment info
// @Description  Returns the environment properties from the environment widget JSON for a given report.
// @Tags         reports
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Param        report_id   path  string  true  "Report ID or 'latest'"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      500  {object}  map[string]any
// @Router       /projects/{project_id}/reports/{report_id}/environment [get]
func (h *AllureHandler) GetReportEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, ok := h.extractProjectID(w, r)
	if !ok {
		return
	}

	reportID, ok := extractReportID(w, r)
	if !ok {
		return
	}

	relPath := "reports/" + reportID + "/widgets/environment.json"
	entries := make([]EnvironmentEntry, 0)
	if !h.readJSONViaStore(ctx, projectID, relPath, &entries) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data":     entries,
			"metadata": map[string]string{"message": "No environment data available"},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     entries,
		"metadata": map[string]string{"message": "Environment info successfully obtained"},
	})
}

// applySummaryFile reads widgets/summary.json (Allure 2) and fills entry in-place.
// Returns true when the file was read and applied successfully.
func (h *AllureHandler) applySummaryFile(ctx context.Context, projectID string, entry *ReportHistoryEntry, widgetsRelPath string) bool {
	var s allureSummaryFile
	if !h.readJSONViaStore(ctx, projectID, widgetsRelPath+"/summary.json", &s) {
		return false
	}
	entry.Statistic = s.Statistic
	if s.Time != nil {
		if s.Time.Stop != 0 {
			t := time.Unix(0, s.Time.Stop*int64(time.Millisecond)).UTC().Format(time.RFC3339)
			entry.GeneratedAt = &t
		}
		if s.Time.Duration != 0 {
			entry.DurationMs = &s.Time.Duration
		}
	}
	return true
}

// testResultTiming holds the start/stop epoch milliseconds from an Allure test result file.
type testResultTiming struct {
	Start int64 `json:"start"`
	Stop  int64 `json:"stop"`
}

// applyTimingFromTestResults scans data/test-results/*.json to derive timing for Allure 3 reports.
// It sets entry.DurationMs = sum of individual test durations and entry.GeneratedAt = RFC3339 of max(stop).
// When no valid timing is found the entry fields are left unchanged.
func (h *AllureHandler) applyTimingFromTestResults(ctx context.Context, projectID string, entry *ReportHistoryEntry, dataRelPath string) {
	entries, err := h.store.ReadDir(ctx, projectID, dataRelPath)
	if err != nil {
		return
	}
	var totalDuration, maxStop int64
	for _, e := range entries {
		if e.IsDir || !strings.HasSuffix(e.Name, ".json") {
			continue
		}
		data, err := h.store.ReadFile(ctx, projectID, dataRelPath+"/"+e.Name)
		if err != nil {
			continue
		}
		var tr testResultTiming
		if json.Unmarshal(data, &tr) != nil || tr.Start == 0 || tr.Stop == 0 {
			continue
		}
		if tr.Stop > tr.Start {
			totalDuration += tr.Stop - tr.Start
		}
		if tr.Stop > maxStop {
			maxStop = tr.Stop
		}
	}
	if totalDuration > 0 {
		entry.DurationMs = &totalDuration
	}
	if maxStop > 0 {
		t := time.Unix(0, maxStop*int64(time.Millisecond)).UTC().Format(time.RFC3339)
		entry.GeneratedAt = &t
	}
}

// buildReportEntry reads report metadata from widget files via the store.
// Tries widgets/summary.json first (Allure 2), then widgets/statistic.json (Allure 3).
func (h *AllureHandler) buildReportEntry(ctx context.Context, projectID, name string) ReportHistoryEntry {
	entry := ReportHistoryEntry{ReportID: name}
	widgetsRelPath := "reports/" + name + "/widgets"

	// Allure 2: widgets/summary.json contains statistic + time nested under keys.
	if h.applySummaryFile(ctx, projectID, &entry, widgetsRelPath) {
		return entry
	}

	// Allure 3: widgets/statistic.json has statistic fields at root level.
	var stat AllureStatistic
	if h.readJSONViaStore(ctx, projectID, widgetsRelPath+"/statistic.json", &stat) && stat.Total > 0 {
		entry.Statistic = &stat
	}
	// Allure 3 has no timing in statistic.json; derive from test result files.
	h.applyTimingFromTestResults(ctx, projectID, &entry, "reports/"+name+"/data/test-results")
	return entry
}

// CategoryMatchedStatistic holds the defect count breakdown for one category.
type CategoryMatchedStatistic struct {
	Failed  int `json:"failed"`
	Broken  int `json:"broken"`
	Known   int `json:"known"`
	Unknown int `json:"unknown"`
	Total   int `json:"total"`
}

// CategoryEntry represents one row in the Allure categories widget.
type CategoryEntry struct {
	Name             string                    `json:"name"`
	MatchedStatistic *CategoryMatchedStatistic `json:"matchedStatistic"`
}

// GetReportCategories godoc
// @Summary      Get report defect categories
// @Description  Returns the failure categories from the categories widget JSON for a given report.
// @Tags         reports
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Param        report_id   path  string  true  "Report ID or 'latest'"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      500  {object}  map[string]any
// @Router       /projects/{project_id}/reports/{report_id}/categories [get]
func (h *AllureHandler) GetReportCategories(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, ok := h.extractProjectID(w, r)
	if !ok {
		return
	}

	reportID, ok := extractReportID(w, r)
	if !ok {
		return
	}

	relPath := "reports/" + reportID + "/widgets/categories.json"
	entries := make([]CategoryEntry, 0)
	if !h.readJSONViaStore(ctx, projectID, relPath, &entries) {
		writeJSON(w, http.StatusOK, map[string]any{
			"data":     entries,
			"metadata": map[string]string{"message": "No categories data available"},
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     entries,
		"metadata": map[string]string{"message": "Categories successfully obtained"},
	})
}

// DeleteProject godoc
// @Summary      Delete a project
// @Description  Removes an entire project and all its reports.
// @Tags         projects
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Failure      500  {object}  map[string]any
// @Router       /projects/{project_id} [delete]
func (h *AllureHandler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.extractProjectID(w, r)
	if !ok {
		return
	}

	if err := h.runner.DeleteProject(r.Context(), projectID); err != nil {
		if errors.Is(err, storage.ErrProjectNotFound) {
			// Filesystem missing — attempt DB cleanup for half-synced state
			// (project exists in DB but was never on disk, or disk was removed externally).
			if dbErr := h.projectStore.DeleteProject(r.Context(), projectID); dbErr == nil {
				// Stale DB record removed; surface as success so the UI clears it.
				writeJSON(w, http.StatusOK, map[string]any{
					"data":     map[string]string{"project_id": projectID},
					"metadata": map[string]string{"message": "Project successfully deleted"},
				})
				return
			}
			writeError(w, http.StatusNotFound, fmt.Sprintf("project_id %q not found", projectID))
			return
		}
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("Error deleting project: %v", err))
		return
	}

	// Remove from database. Non-fatal: project may not be in DB.
	if dbErr := h.projectStore.DeleteProject(r.Context(), projectID); dbErr != nil {
		if !errors.Is(dbErr, store.ErrProjectNotFound) {
			h.logger.Error("db project cleanup failed", zap.String("project_id", projectID), zap.Error(dbErr))
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     map[string]string{"project_id": projectID},
		"metadata": map[string]string{"message": "Project successfully deleted"},
	})
}

// DeleteReport godoc
// @Summary      Delete a report
// @Description  Removes a single numbered report directory for a project.
// @Tags         reports
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Param        report_id   path  string  true  "Report ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Failure      500  {object}  map[string]any
// @Router       /projects/{project_id}/reports/{report_id} [delete]
func (h *AllureHandler) DeleteReport(w http.ResponseWriter, r *http.Request) {
	projectID, ok := h.extractProjectID(w, r)
	if !ok {
		return
	}

	reportID, ok := extractReportID(w, r)
	if !ok {
		return
	}

	if err := h.runner.DeleteReport(r.Context(), projectID, reportID); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, storage.ErrReportNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, storage.ErrReportIDEmpty) || errors.Is(err, storage.ErrReportIDInvalid) {
			status = http.StatusBadRequest
		}
		writeError(w, status, fmt.Sprintf("Error deleting report: %v", err))
		return
	}

	// Remove build record from database. Non-fatal if not found.
	if buildOrder, err := strconv.Atoi(reportID); err == nil {
		_ = h.buildStore.DeleteBuild(r.Context(), projectID, buildOrder)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     map[string]string{"report_id": reportID, "project_id": projectID},
		"metadata": map[string]string{"message": fmt.Sprintf("Report %q successfully deleted", reportID)},
	})
}
