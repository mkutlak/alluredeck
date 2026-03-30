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

// NOTE: reservedProjectNames, validateProjectID, safeProjectID, and the
// package-level extractProjectID function are defined in project_id.go.
// Types, errors, and validation helpers are defined in types.go, errors.go, and project_id.go.

// extractProjectID delegates to the package-level extractProjectID using the
// handler's configured projects directory.
func (h *AllureHandler) extractProjectID(w http.ResponseWriter, r *http.Request) (string, bool) {
	return extractProjectID(w, r, h.cfg.ProjectsPath)
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
	writeSuccess(w, http.StatusAccepted, map[string]string{"job_id": job.ID}, "Report generation queued")
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

	writeSuccess(w, http.StatusOK, job, "Job status retrieved")
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
		h.logger.Error("cleaning history failed", zap.String("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error cleaning history")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"output": ""}, "History successfully cleaned")
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
		h.logger.Error("cleaning results failed", zap.String("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error cleaning results")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"output": ""}, "Results successfully cleaned")
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

	parentID := r.URL.Query().Get("parent_id")

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

	if h.cfg.APIResponseLessVerbose {
		writeSuccess(w, http.StatusOK, map[string]any{}, fmt.Sprintf("Results successfully sent for project_id '%s'", projectID))
		return
	}

	currentFileNames, _ := h.store.ListResultFiles(r.Context(), projectID)

	writeSuccess(w, http.StatusOK, map[string]any{
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
		h.logger.Error("reading report history failed", zap.String("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error reading report history")
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

// readJSONViaStore reads a project-relative path via the store and unmarshals JSON into v.
// Returns true only when both the read and unmarshal succeed.
func (h *AllureHandler) readJSONViaStore(ctx context.Context, projectID, relPath string, v any) bool {
	data, err := h.store.ReadFile(ctx, projectID, relPath)
	if err != nil {
		return false
	}
	return json.Unmarshal(data, v) == nil
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
		writeSuccess(w, http.StatusOK, entries, "No environment data available")
		return
	}

	writeSuccess(w, http.StatusOK, entries, "Environment info successfully obtained")
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
		writeSuccess(w, http.StatusOK, entries, "No categories data available")
		return
	}

	writeSuccess(w, http.StatusOK, entries, "Categories successfully obtained")
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
		h.logger.Error("deleting report failed", zap.String("project_id", projectID), zap.String("report_id", reportID), zap.Error(err))
		status := http.StatusInternalServerError
		msg := "error deleting report"
		if errors.Is(err, storage.ErrReportNotFound) {
			status = http.StatusNotFound
			msg = "report not found"
		} else if errors.Is(err, storage.ErrReportIDEmpty) || errors.Is(err, storage.ErrReportIDInvalid) {
			status = http.StatusBadRequest
			msg = "invalid report id"
		}
		writeError(w, status, msg)
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
