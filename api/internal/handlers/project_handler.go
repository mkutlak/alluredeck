package handlers

import (
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

// ProjectHandler handles project CRUD operations.
type ProjectHandler struct {
	projectStore store.ProjectStorer
	runner       *runner.Allure
	store        storage.Store
	cfg          *config.Config
	logger       *zap.Logger
}

// NewProjectHandler creates a ProjectHandler.
func NewProjectHandler(ps store.ProjectStorer, r *runner.Allure, st storage.Store, cfg *config.Config, logger *zap.Logger) *ProjectHandler {
	return &ProjectHandler{projectStore: ps, runner: r, store: st, cfg: cfg, logger: logger}
}

// GetProjects godoc
// @Summary      List projects
// @Description  Returns a paginated list of all existing projects. Supports top_level=true to return only top-level projects and parent_id=<id> to list children of a specific project.
// @Tags         projects
// @Produce      json
// @Param        page       query  int     false  "Page number"                        default(1)
// @Param        per_page   query  int     false  "Items per page"                     default(20)
// @Param        top_level  query  bool    false  "Return only top-level projects"
// @Param        parent_id  query  string  false  "Return children of this project ID"
// @Success      200  {object}  map[string]any
// @Failure      500  {object}  map[string]any
// @Router       /projects [get]
func (h *ProjectHandler) GetProjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pg := parsePagination(r)
	topLevel := r.URL.Query().Get("top_level") == "true"
	filterParentIDStr := r.URL.Query().Get("parent_id")

	var (
		dbProjects []store.Project
		total      int
		err        error
	)

	switch {
	case filterParentIDStr != "":
		filterParentID, parseErr := strconv.ParseInt(filterParentIDStr, 10, 64)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, "parent_id must be a numeric ID")
			return
		}
		// Return children of the given parent project.
		children, childErr := h.projectStore.ListChildren(ctx, filterParentID)
		if childErr != nil {
			h.logger.Error("listing children failed", zap.Int64("parent_id", filterParentID), zap.Error(childErr))
			writeError(w, http.StatusInternalServerError, "error listing children")
			return
		}
		dbProjects = children
		total = len(children)
	case topLevel:
		dbProjects, total, err = h.projectStore.ListProjectsPaginatedTopLevel(ctx, pg.Page, pg.PerPage)
		if err != nil {
			h.logger.Error("listing projects failed", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "error listing projects")
			return
		}
	default:
		dbProjects, total, err = h.projectStore.ListProjectsPaginated(ctx, pg.Page, pg.PerPage)
		if err != nil {
			h.logger.Error("listing projects failed", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "error listing projects")
			return
		}
	}

	// Build parent → children map from the fetched projects.
	childrenOf := make(map[int64][]int64)
	for _, p := range dbProjects {
		if p.ParentID != nil {
			childrenOf[*p.ParentID] = append(childrenOf[*p.ParentID], p.ID)
		}
	}

	entries := make([]ProjectEntry, 0, len(dbProjects))
	for _, p := range dbProjects {
		displayName := p.DisplayName
		if displayName == "" {
			displayName = p.Slug
		}
		e := ProjectEntry{
			ProjectID:   p.ID,
			Slug:        p.Slug,
			StorageKey:  p.StorageKey,
			DisplayName: displayName,
			ReportType:  p.ReportType,
			CreatedAt:   p.CreatedAt.UTC().Format(time.RFC3339),
		}
		if p.ParentID != nil {
			e.ParentID = p.ParentID
		}
		if kids, ok := childrenOf[p.ID]; ok {
			e.Children = kids
		}
		entries = append(entries, e)
	}

	writePagedSuccess(w, entries, "Projects successfully obtained", newPaginationMeta(pg.Page, pg.PerPage, total))
}

// CreateProject godoc
// @Summary      Create a project
// @Description  Creates a new project directory and registers it in the database.
// @Tags         projects
// @Accept       json
// @Produce      json
// @Param        body  body      object  true  "Project ID and optional parent"
// @Success      201   {object}  map[string]any
// @Failure      400   {object}  map[string]any
// @Failure      409   {object}  map[string]any
// @Router       /projects [post]
func (h *ProjectHandler) CreateProject(w http.ResponseWriter, r *http.Request) {
	var reqBody struct {
		ID       string `json:"id"`
		ParentID *int64 `json:"parent_id,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid JSON payload")
		return
	}

	projectSlug := strings.TrimSpace(reqBody.ID)
	if err := validateProjectID(h.cfg.ProjectsPath, projectSlug); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Create filesystem project via runner (storage still uses slug).
	err := h.runner.CreateProject(r.Context(), projectSlug)
	if err != nil {
		if errors.Is(err, runner.ErrProjectExists) {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		h.logger.Error("creating project failed", zap.String("slug", projectSlug), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error creating project")
		return
	}

	// Register in database — returns *Project with numeric ID.
	var project *store.Project
	var dbErr error
	if reqBody.ParentID != nil {
		project, dbErr = h.projectStore.CreateProjectWithParent(r.Context(), projectSlug, *reqBody.ParentID)
	} else {
		project, dbErr = h.projectStore.CreateProject(r.Context(), projectSlug)
	}
	if dbErr != nil {
		if !errors.Is(dbErr, store.ErrProjectExists) {
			h.logger.Error("db project registration failed", zap.String("slug", projectSlug), zap.Error(dbErr))
		}
		// If the project already existed in DB, look it up to get the numeric ID.
		if project == nil {
			project, _ = h.projectStore.GetProjectBySlug(r.Context(), projectSlug)
		}
	}

	entry := ProjectEntry{Slug: projectSlug, DisplayName: projectSlug}
	if project != nil {
		entry.ProjectID = project.ID
		entry.ParentID = reqBody.ParentID
	}
	writeSuccess(w, http.StatusCreated, entry, "Project successfully created")
}

// GetProject godoc
// @Summary      Get a project
// @Description  Returns a single project by numeric ID or slug.
// @Tags         projects
// @Produce      json
// @Param        project_id  path  string  true  "Project ID (numeric) or slug"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /projects/{project_id} [get]
func (h *ProjectHandler) GetProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := resolveProjectIntID(w, r, h.projectStore)
	if !ok {
		return
	}

	project, err := h.projectStore.GetProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "error fetching project")
		return
	}

	displayName := project.DisplayName
	if displayName == "" {
		displayName = project.Slug
	}
	entry := ProjectEntry{
		ProjectID:   project.ID,
		Slug:        project.Slug,
		StorageKey:  project.StorageKey,
		DisplayName: displayName,
		ReportType:  project.ReportType,
		CreatedAt:   project.CreatedAt.UTC().Format(time.RFC3339),
	}
	if project.ParentID != nil {
		entry.ParentID = project.ParentID
	}

	children, childErr := h.projectStore.ListChildren(r.Context(), project.ID)
	if childErr == nil && len(children) > 0 {
		ids := make([]int64, len(children))
		for i, c := range children {
			ids[i] = c.ID
		}
		entry.Children = ids
	}

	writeSuccess(w, http.StatusOK, entry, "Project successfully obtained")
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
func (h *ProjectHandler) DeleteProject(w http.ResponseWriter, r *http.Request) {
	projectID, ok := resolveProjectIntID(w, r, h.projectStore)
	if !ok {
		return
	}

	// Load project to get slug for storage operations.
	project, err := h.projectStore.GetProject(r.Context(), projectID)
	if err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusNotFound, fmt.Sprintf("project_id %d not found", projectID))
			return
		}
		writeError(w, http.StatusInternalServerError, "error fetching project")
		return
	}
	if err := h.runner.DeleteProject(r.Context(), project.StorageKey); err != nil {
		if errors.Is(err, storage.ErrProjectNotFound) {
			// Filesystem missing — attempt DB cleanup for half-synced state.
			if dbErr := h.projectStore.DeleteProject(r.Context(), projectID); dbErr == nil {
				writeJSON(w, http.StatusOK, map[string]any{
					"data":     map[string]any{"project_id": projectID},
					"metadata": map[string]string{"message": "Project successfully deleted"},
				})
				return
			}
			writeError(w, http.StatusNotFound, fmt.Sprintf("project_id %d not found", projectID))
			return
		}
		h.logger.Error("deleting project failed", zap.Int64("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error deleting project")
		return
	}

	// Remove from database. Non-fatal: project may not be in DB.
	if dbErr := h.projectStore.DeleteProject(r.Context(), projectID); dbErr != nil {
		if !errors.Is(dbErr, store.ErrProjectNotFound) {
			h.logger.Error("db project cleanup failed", zap.Int64("project_id", projectID), zap.Error(dbErr))
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     map[string]any{"project_id": projectID},
		"metadata": map[string]string{"message": "Project successfully deleted"},
	})
}

// RenameProject godoc
// @Summary      Rename a project
// @Description  Changes the project ID. All builds, results, and storage are updated. Warning: this operation may cause data loss if interrupted during S3 storage migration.
// @Tags         projects
// @Accept       json
// @Produce      json
// @Param        project_id  path  string  true  "Current project ID"
// @Param        body        body  object  true  "New project ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Failure      409  {object}  map[string]any
// @Router       /projects/{project_id}/rename [put]
func (h *ProjectHandler) RenameProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, ok := resolveProjectIntID(w, r, h.projectStore)
	if !ok {
		return
	}

	var reqBody struct {
		NewID string `json:"new_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	newSlug := strings.TrimSpace(reqBody.NewID)
	if err := validateProjectID(h.cfg.ProjectsPath, newSlug); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Check old exists and get slug.
	project, err := h.projectStore.GetProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	oldSlug := project.Slug

	if newSlug == oldSlug {
		writeError(w, http.StatusBadRequest, "new_id must differ from current project slug")
		return
	}

	// Check new slug doesn't exist.
	if existing, _ := h.projectStore.GetProjectBySlug(ctx, newSlug); existing != nil {
		writeError(w, http.StatusConflict, fmt.Sprintf("project %q already exists", newSlug))
		return
	}

	// For top-level projects, rename storage (storageKey == slug, so it changes on rename).
	// For child projects, storageKey is the numeric ID and does not change on rename — skip storage rename.
	if project.ParentID == nil {
		oldStorageKey := project.StorageKey
		if err := h.store.RenameProject(ctx, oldStorageKey, newSlug); err != nil {
			h.logger.Error("storage rename failed", zap.String("old", oldStorageKey), zap.String("new", newSlug), zap.Error(err))
			writeError(w, http.StatusInternalServerError, "failed to rename project storage")
			return
		}

		// Rename in DB.
		if err := h.projectStore.RenameProject(ctx, projectID, newSlug); err != nil {
			h.logger.Error("db rename failed, attempting storage rollback", zap.Error(err))
			if rbErr := h.store.RenameProject(ctx, newSlug, oldStorageKey); rbErr != nil {
				h.logger.Error("storage rollback failed", zap.Error(rbErr))
			}
			if errors.Is(err, store.ErrProjectExists) {
				writeError(w, http.StatusConflict, fmt.Sprintf("project %q already exists", newSlug))
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to rename project")
			return
		}
	} else {
		// Child project: storageKey is numeric ID, doesn't change. Only rename in DB.
		if err := h.projectStore.RenameProject(ctx, projectID, newSlug); err != nil {
			if errors.Is(err, store.ErrProjectExists) {
				writeError(w, http.StatusConflict, fmt.Sprintf("project %q already exists", newSlug))
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to rename project")
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     map[string]any{"project_id": projectID, "old_slug": oldSlug, "new_slug": newSlug},
		"metadata": map[string]string{"message": "Project renamed successfully. Note: external references to the old project slug must be updated manually."},
	})
}
