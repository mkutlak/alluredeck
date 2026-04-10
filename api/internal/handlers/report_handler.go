package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// ReportHandler handles report generation, history, and widget endpoints.
type ReportHandler struct {
	jobManager      runner.JobQueuer
	runner          *runner.Allure
	buildStore      store.BuildStorer
	branchStore     store.BranchStorer
	testResultStore store.TestResultStorer
	knownIssueStore store.KnownIssueStorer
	projectStore    store.ProjectStorer
	store           storage.Store
	cfg             *config.Config
	logger          *zap.Logger
}

// ReportHandlerDeps holds the dependencies for creating a ReportHandler.
type ReportHandlerDeps struct {
	JobManager      runner.JobQueuer
	Runner          *runner.Allure
	BuildStore      store.BuildStorer
	BranchStore     store.BranchStorer
	TestResultStore store.TestResultStorer
	KnownIssueStore store.KnownIssueStorer
	ProjectStore    store.ProjectStorer
	Store           storage.Store
	Config          *config.Config
	Logger          *zap.Logger
}

// NewReportHandler creates a ReportHandler.
func NewReportHandler(deps ReportHandlerDeps) *ReportHandler {
	return &ReportHandler{
		jobManager:      deps.JobManager,
		runner:          deps.Runner,
		buildStore:      deps.BuildStore,
		branchStore:     deps.BranchStore,
		testResultStore: deps.TestResultStore,
		knownIssueStore: deps.KnownIssueStore,
		projectStore:    deps.ProjectStore,
		store:           deps.Store,
		cfg:             deps.Config,
		logger:          deps.Logger,
	}
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
// lookupProjectSlug resolves a project path value (numeric ID or slug) to its
// int64 ID and slug for storage operations.
// Returns (projectID, slug, ok). Writes an error response and returns false on failure.
func (h *ReportHandler) lookupProjectSlug(w http.ResponseWriter, r *http.Request) (int64, string, bool) {
	pathValue := r.PathValue("project_id")
	if pathValue == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return 0, "", false
	}

	ctx := r.Context()

	// Try numeric ID first.
	if id, err := strconv.ParseInt(pathValue, 10, 64); err == nil {
		project, err := h.projectStore.GetProject(ctx, id)
		if err == nil {
			return project.ID, project.Slug, true
		}
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return 0, "", false
		}
		writeError(w, http.StatusInternalServerError, "error fetching project")
		return 0, "", false
	}

	// Validate slug before DB lookup (path traversal, reserved names, etc.).
	if strings.ContainsAny(pathValue, "/\\") || strings.Contains(pathValue, "..") {
		writeError(w, http.StatusBadRequest, ErrProjectInvalidChars.Error())
		return 0, "", false
	}
	if reservedProjectNames[pathValue] {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("project_id %q: %s", pathValue, ErrProjectReserved.Error()))
		return 0, "", false
	}

	// Fall back to slug lookup.
	project, err := h.projectStore.GetProjectBySlug(ctx, pathValue)
	if err == nil {
		return project.ID, project.Slug, true
	}
	if errors.Is(err, store.ErrProjectNotFound) {
		writeError(w, http.StatusNotFound, "project not found")
		return 0, "", false
	}
	writeError(w, http.StatusInternalServerError, "error fetching project")
	return 0, "", false
}

func (h *ReportHandler) GenerateReport(w http.ResponseWriter, r *http.Request) {
	projectID, slug, ok := h.lookupProjectSlug(w, r)
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
	job := h.jobManager.Submit(projectID, slug, params)
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
func (h *ReportHandler) GetJobStatus(w http.ResponseWriter, r *http.Request) {
	projectID, _, ok := h.lookupProjectSlug(w, r)
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
func (h *ReportHandler) CleanHistory(w http.ResponseWriter, r *http.Request) {
	projectID, slug, ok := h.lookupProjectSlug(w, r)
	if !ok {
		return
	}

	if err := h.runner.CleanHistory(r.Context(), projectID, slug); err != nil {
		h.logger.Error("cleaning history failed", zap.String("slug", slug), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error cleaning history")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]string{"output": ""}, "History successfully cleaned")
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
func (h *ReportHandler) GetReportHistory(w http.ResponseWriter, r *http.Request) {
	projectID, slug, ok := h.lookupProjectSlug(w, r)
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

	// Fetch numbered builds from DB (sorted descending by build_number).
	builds, total, err := h.buildStore.ListBuildsPaginatedBranch(r.Context(), projectID, pg.Page, pg.PerPage, branchID)
	if err != nil {
		h.logger.Error("reading report history failed", zap.Int64("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error reading report history")
		return
	}

	// Initialize as empty slice so JSON encodes [] instead of null when there are no reports.
	reports := make([]ReportHistoryEntry, 0)

	// Check for "latest" dir via store — always regenerated, not tracked in DB.
	// The "latest" entry is always prepended (not counted against pagination).
	if exists, _ := h.store.LatestReportExists(r.Context(), slug); exists {
		entry := h.buildReportEntry(r.Context(), slug, "latest")
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
func (h *ReportHandler) readJSONViaStore(ctx context.Context, projectID, relPath string, v any) bool {
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
func (h *ReportHandler) GetReportEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, slug, ok := h.lookupProjectSlug(w, r)
	if !ok {
		return
	}

	reportID, ok := extractReportID(w, r)
	if !ok {
		return
	}

	relPath := "reports/" + reportID + "/widgets/environment.json"
	entries := make([]EnvironmentEntry, 0)
	if !h.readJSONViaStore(ctx, slug, relPath, &entries) {
		writeSuccess(w, http.StatusOK, entries, "No environment data available")
		return
	}

	writeSuccess(w, http.StatusOK, entries, "Environment info successfully obtained")
}

// applySummaryFile reads widgets/summary.json (Allure 2) and fills entry in-place.
// Returns true when the file was read and applied successfully.
func (h *ReportHandler) applySummaryFile(ctx context.Context, projectID string, entry *ReportHistoryEntry, widgetsRelPath string) bool {
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
func (h *ReportHandler) applyTimingFromTestResults(ctx context.Context, projectID string, entry *ReportHistoryEntry, dataRelPath string) {
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
func (h *ReportHandler) buildReportEntry(ctx context.Context, projectID, name string) ReportHistoryEntry {
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

// CleanGroupHistory godoc
// @Summary      Clean history for all projects in a group
// @Description  Deletes all report history for all child projects of the given parent.
// @Tags         reports
// @Produce      json
// @Param        project_id  path  string  true  "Parent project ID"
// @Success      200  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /projects/{project_id}/reports/history/group [delete]
func (h *ReportHandler) CleanGroupHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID, slug, ok := h.lookupProjectSlug(w, r)
	if !ok {
		return
	}

	if h.projectStore == nil {
		writeError(w, http.StatusInternalServerError, "project store not available")
		return
	}

	// ListChildIDs returns slugs for storage operations.
	childSlugs, err := h.projectStore.ListChildIDs(ctx, projectID)
	if err != nil {
		h.logger.Error("clean group history: list child IDs failed", zap.Int64("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if len(childSlugs) == 0 {
		writeError(w, http.StatusNotFound, "no child projects found")
		return
	}

	cleaned := 0
	// Clean each child project (runner uses slug for storage).
	for _, childSlug := range childSlugs {
		if err := h.runner.CleanHistory(ctx, 0, childSlug); err != nil {
			h.logger.Warn("clean group history: failed for child", zap.String("child_slug", childSlug), zap.Error(err))
			continue
		}
		cleaned++
	}
	// Also clean the parent itself.
	if err := h.runner.CleanHistory(ctx, projectID, slug); err != nil {
		h.logger.Warn("clean group history: failed for parent", zap.String("slug", slug), zap.Error(err))
	} else {
		cleaned++
	}

	writeSuccess(w, http.StatusOK, map[string]any{"cleaned": cleaned}, fmt.Sprintf("history cleaned for %d project(s)", cleaned))
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
func (h *ReportHandler) GetReportCategories(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	_, slug, ok := h.lookupProjectSlug(w, r)
	if !ok {
		return
	}

	reportID, ok := extractReportID(w, r)
	if !ok {
		return
	}

	relPath := "reports/" + reportID + "/widgets/categories.json"
	entries := make([]CategoryEntry, 0)
	if !h.readJSONViaStore(ctx, slug, relPath, &entries) {
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
func (h *ReportHandler) DeleteReport(w http.ResponseWriter, r *http.Request) {
	projectID, slug, ok := h.lookupProjectSlug(w, r)
	if !ok {
		return
	}

	reportID, ok := extractReportID(w, r)
	if !ok {
		return
	}

	if err := h.runner.DeleteReport(r.Context(), projectID, slug, reportID); err != nil {
		h.logger.Error("deleting report failed", zap.Int64("project_id", projectID), zap.String("report_id", reportID), zap.Error(err))
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
	if buildNumber, err := strconv.Atoi(reportID); err == nil {
		_ = h.buildStore.DeleteBuild(r.Context(), projectID, buildNumber)
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     map[string]any{"report_id": reportID, "project_id": projectID},
		"metadata": map[string]string{"message": fmt.Sprintf("Report %q successfully deleted", reportID)},
	})
}
