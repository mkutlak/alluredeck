package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// AdminHandler handles admin-only system monitoring endpoints.
type AdminHandler struct {
	jobManager   runner.JobQueuer
	store        storage.Store
	projectStore store.ProjectStorer
	projectsDir  string
	logger       *zap.Logger
}

// NewAdminHandler creates a new AdminHandler.
func NewAdminHandler(jm runner.JobQueuer, store storage.Store, projectsDir string, logger *zap.Logger) *AdminHandler {
	return &AdminHandler{
		jobManager:  jm,
		store:       store,
		projectsDir: projectsDir,
		logger:      logger,
	}
}

// NewAdminHandlerWithProjects creates a new AdminHandler with project store support for group operations.
func NewAdminHandlerWithProjects(jm runner.JobQueuer, store storage.Store, projectStore store.ProjectStorer, projectsDir string, logger *zap.Logger) *AdminHandler {
	return &AdminHandler{
		jobManager:   jm,
		store:        store,
		projectStore: projectStore,
		projectsDir:  projectsDir,
		logger:       logger,
	}
}

// pendingResultsEntry is the JSON shape for one project's pending result files.
type pendingResultsEntry struct {
	ProjectID    string    `json:"project_id"`
	FileCount    int       `json:"file_count"`
	TotalSize    int64     `json:"total_size"`
	LastModified time.Time `json:"last_modified"`
}

// ListJobs godoc
// @Summary      List all jobs
// @Description  Returns all known async report generation jobs (active and recent).
// @Tags         admin
// @Produce      json
// @Success      200  {object}  map[string]any
// @Router       /admin/jobs [get]
func (h *AdminHandler) ListJobs(w http.ResponseWriter, r *http.Request) {
	jobs := h.jobManager.ListJobs()
	writeSuccess(w, http.StatusOK, jobs, fmt.Sprintf("%d job(s) found", len(jobs)))
}

// ListPendingResults godoc
// @Summary      List projects with unprocessed result files
// @Description  Scans all projects and returns those with pending result files.
// @Tags         admin
// @Produce      json
// @Success      200  {object}  map[string]any
// @Router       /admin/results [get]
func (h *AdminHandler) ListPendingResults(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projects, err := h.store.ListProjects(ctx)
	if err != nil {
		h.logger.Error("admin: list projects failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to list projects")
		return
	}

	entries := make([]pendingResultsEntry, 0)
	for _, projectID := range projects {
		files, err := h.store.ReadDir(ctx, projectID, "results")
		if err != nil || len(files) == 0 {
			continue
		}

		var totalSize int64
		var lastMod int64
		for _, f := range files {
			if f.IsDir {
				continue
			}
			totalSize += f.Size
			if f.ModTime > lastMod {
				lastMod = f.ModTime
			}
		}

		// Count only non-directory entries.
		fileCount := 0
		for _, f := range files {
			if !f.IsDir {
				fileCount++
			}
		}
		if fileCount == 0 {
			continue
		}

		entries = append(entries, pendingResultsEntry{
			ProjectID:    projectID,
			FileCount:    fileCount,
			TotalSize:    totalSize,
			LastModified: time.Unix(0, lastMod),
		})
	}

	writeSuccess(w, http.StatusOK, entries, fmt.Sprintf("%d project(s) with pending results", len(entries)))
}

// CancelJob godoc
// @Summary      Cancel a job
// @Description  Cancels a pending or running async report generation job.
// @Tags         admin
// @Produce      json
// @Param        job_id  path  string  true  "Job ID"
// @Success      200  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Failure      409  {object}  map[string]any
// @Router       /admin/jobs/{job_id}/cancel [post]
func (h *AdminHandler) CancelJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("job_id")
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "job_id is required")
		return
	}

	err := h.jobManager.Cancel(jobID)
	if err == nil {
		writeSuccess(w, http.StatusOK, map[string]any{}, "job cancelled")
		return
	}

	// Distinguish not-found vs already-terminal.
	if errors.Is(err, runner.ErrJobNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	// Terminal state conflict.
	writeError(w, http.StatusConflict, err.Error())
}

// CleanProjectResults godoc
// @Summary      Delete pending result files for a project
// @Description  Removes all unprocessed result files from a project's results directory.
// @Tags         admin
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Success      200  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /admin/results/{project_id} [delete]
func (h *AdminHandler) CleanProjectResults(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID, ok := extractProjectID(w, r, h.projectsDir)
	if !ok {
		return
	}

	exists, err := h.store.ProjectExists(ctx, projectID)
	if err != nil {
		h.logger.Error("admin: project exists check failed", zap.String("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if !exists {
		writeError(w, http.StatusNotFound, fmt.Sprintf("project %q not found", projectID))
		return
	}

	if err := h.store.CleanResults(ctx, projectID); err != nil {
		h.logger.Error("admin: clean results failed", zap.String("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to clean results")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{}, "results cleaned")
}

// CleanGroupResults godoc
// @Summary      Clean results for all projects in a group
// @Description  Deletes pending result files for all child projects of the given parent.
// @Tags         admin
// @Produce      json
// @Param        project_id  path  string  true  "Parent project ID"
// @Success      200  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /admin/results/group/{project_id} [delete]
func (h *AdminHandler) CleanGroupResults(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID := r.PathValue("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	if h.projectStore == nil {
		writeError(w, http.StatusInternalServerError, "project store not available")
		return
	}

	childIDs, err := h.projectStore.ListChildIDs(ctx, projectID)
	if err != nil {
		h.logger.Error("admin: list child IDs failed", zap.String("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if len(childIDs) == 0 {
		writeError(w, http.StatusNotFound, "no child projects found")
		return
	}

	cleaned := 0
	// Clean each child project.
	for _, childID := range childIDs {
		if err := h.store.CleanResults(ctx, childID); err != nil {
			h.logger.Warn("admin: clean group results failed for child", zap.String("child_id", childID), zap.Error(err))
			continue
		}
		cleaned++
	}
	// Also clean the parent itself.
	if err := h.store.CleanResults(ctx, projectID); err != nil {
		h.logger.Warn("admin: clean group results failed for parent", zap.String("project_id", projectID), zap.Error(err))
	} else {
		cleaned++
	}

	writeSuccess(w, http.StatusOK, map[string]any{"cleaned": cleaned}, fmt.Sprintf("results cleaned for %d project(s)", cleaned))
}

// DeleteJob godoc
// @Summary      Delete a terminal job
// @Description  Permanently removes a completed, failed, or cancelled job record.
// @Tags         admin
// @Produce      json
// @Param        job_id  path  string  true  "Job ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Failure      409  {object}  map[string]any
// @Router       /admin/jobs/{job_id} [delete]
func (h *AdminHandler) DeleteJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("job_id")
	if jobID == "" {
		writeError(w, http.StatusBadRequest, "job_id is required")
		return
	}

	err := h.jobManager.Delete(jobID)
	if err == nil {
		writeSuccess(w, http.StatusOK, map[string]any{}, "job deleted")
		return
	}

	if errors.Is(err, runner.ErrJobNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}

	if errors.Is(err, runner.ErrJobNotTerminal) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	h.logger.Error("admin: delete job failed", zap.String("job_id", jobID), zap.Error(err))
	writeError(w, http.StatusInternalServerError, "internal error")
}
