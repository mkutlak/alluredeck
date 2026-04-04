package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	filterParentID := r.URL.Query().Get("parent_id")

	var (
		dbProjects []store.Project
		total      int
		err        error
	)

	switch {
	case filterParentID != "":
		// Return children of the given parent project.
		children, childErr := h.projectStore.ListChildren(ctx, filterParentID)
		if childErr != nil {
			h.logger.Error("listing children failed", zap.String("parent_id", filterParentID), zap.Error(childErr))
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
	childrenOf := make(map[string][]string)
	for _, p := range dbProjects {
		if p.ParentID != nil {
			childrenOf[*p.ParentID] = append(childrenOf[*p.ParentID], p.ID)
		}
	}

	entries := make([]ProjectEntry, 0, len(dbProjects))
	for _, p := range dbProjects {
		e := ProjectEntry{
			ProjectID: p.ID,
			CreatedAt: p.CreatedAt.UTC().Format(time.RFC3339),
			ParentID:  p.ParentID,
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
		ParentID string `json:"parent_id,omitempty"`
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
		h.logger.Error("creating project failed", zap.String("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error creating project")
		return
	}

	// Register in database (INSERT OR IGNORE so an already-synced project is not an error).
	parentID := strings.TrimSpace(reqBody.ParentID)
	var dbErr error
	if parentID != "" {
		dbErr = h.projectStore.CreateProjectWithParent(r.Context(), projectID, parentID)
	} else {
		dbErr = h.projectStore.CreateProject(r.Context(), projectID)
	}
	if dbErr != nil {
		if !errors.Is(dbErr, store.ErrProjectExists) {
			// Log but don't fail — filesystem project was already created successfully.
			h.logger.Error("db project registration failed", zap.String("project_id", projectID), zap.Error(dbErr))
		}
	}

	entry := ProjectEntry{ProjectID: projectID}
	if parentID != "" {
		entry.ParentID = &parentID
	}
	writeSuccess(w, http.StatusCreated, entry, "Project successfully created")
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
	projectID, ok := extractProjectID(w, r, h.cfg.ProjectsPath)
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
		h.logger.Error("deleting project failed", zap.String("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error deleting project")
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

	projectID, ok := extractProjectID(w, r, h.cfg.ProjectsPath)
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

	newID := strings.TrimSpace(reqBody.NewID)
	if err := validateProjectID(h.cfg.ProjectsPath, newID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if newID == projectID {
		writeError(w, http.StatusBadRequest, "new_id must differ from current project ID")
		return
	}

	// Check old exists.
	if _, err := h.projectStore.GetProject(ctx, projectID); err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Check new doesn't exist.
	if exists, _ := h.projectStore.ProjectExists(ctx, newID); exists {
		writeError(w, http.StatusConflict, fmt.Sprintf("project %q already exists", newID))
		return
	}

	// Rename storage first (reversible).
	if err := h.store.RenameProject(ctx, projectID, newID); err != nil {
		h.logger.Error("storage rename failed", zap.String("old", projectID), zap.String("new", newID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to rename project storage")
		return
	}

	// Rename in DB (cascades via ON UPDATE CASCADE).
	if err := h.projectStore.RenameProject(ctx, projectID, newID); err != nil {
		h.logger.Error("db rename failed, attempting storage rollback", zap.Error(err))
		if rbErr := h.store.RenameProject(ctx, newID, projectID); rbErr != nil {
			h.logger.Error("storage rollback failed", zap.Error(rbErr))
		}
		if errors.Is(err, store.ErrProjectExists) {
			writeError(w, http.StatusConflict, fmt.Sprintf("project %q already exists", newID))
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to rename project")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     map[string]any{"old_id": projectID, "new_id": newID},
		"metadata": map[string]string{"message": "Project renamed successfully. Note: external references to the old project ID must be updated manually."},
	})
}
