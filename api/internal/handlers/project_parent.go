package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// ProjectParentHandler handles HTTP requests for managing parent-child project relationships.
type ProjectParentHandler struct {
	projectStore store.ProjectStorer
	logger       *zap.Logger
}

// NewProjectParentHandler creates a new ProjectParentHandler.
func NewProjectParentHandler(ps store.ProjectStorer, logger *zap.Logger) *ProjectParentHandler {
	return &ProjectParentHandler{projectStore: ps, logger: logger}
}

type setParentRequest struct {
	ParentID int64 `json:"parent_id"`
}

// SetParent godoc
// @Summary      Set project parent
// @Description  Assigns a parent project to the given project. The parent must exist, must not itself be a child, and the target must have no children.
// @Tags         projects
// @Accept       json
// @Produce      json
// @Param        project_id  path  string           true  "Project ID"
// @Param        body        body  setParentRequest  true  "Parent payload"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Failure      409  {object}  map[string]any
// @Router       /projects/{project_id}/parent [put]
func (h *ProjectParentHandler) SetParent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, ok := resolveProjectIntID(w, r, h.projectStore)
	if !ok {
		return
	}

	var req setParentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.ParentID == 0 {
		writeError(w, http.StatusBadRequest, "parent_id is required")
		return
	}

	if projectID == req.ParentID {
		writeError(w, http.StatusBadRequest, "project cannot be its own parent")
		return
	}

	// Parent project must exist.
	parent, err := h.projectStore.GetProject(ctx, req.ParentID)
	if err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusNotFound, "parent project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch parent project")
		return
	}

	// Parent must not itself be a child.
	if parent.ParentID != nil {
		writeError(w, http.StatusBadRequest, "parent project is already a child of another project")
		return
	}

	// Target project must not already have children.
	hasChildren, err := h.projectStore.HasChildren(ctx, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check project children")
		return
	}
	if hasChildren {
		writeError(w, http.StatusConflict, "project already has children and cannot become a child itself")
		return
	}

	if err := h.projectStore.SetParent(ctx, projectID, req.ParentID); err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to set parent")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{
		"project_id": projectID,
		"parent_id":  req.ParentID,
	}, "Parent set successfully")
}

// ClearParent godoc
// @Summary      Remove project parent
// @Description  Removes the parent assignment from the given project.
// @Tags         projects
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /projects/{project_id}/parent [delete]
func (h *ProjectParentHandler) ClearParent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, ok := resolveProjectIntID(w, r, h.projectStore)
	if !ok {
		return
	}

	project, err := h.projectStore.GetProject(ctx, projectID)
	if err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch project")
		return
	}

	if project.ParentID == nil {
		writeError(w, http.StatusBadRequest, "project does not have a parent")
		return
	}

	if err := h.projectStore.ClearParent(ctx, projectID); err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to clear parent")
		return
	}

	writeSuccess(w, http.StatusOK, map[string]any{"project_id": projectID}, "Parent removed successfully")
}

// ListChildren godoc
// @Summary      List child projects
// @Description  Returns all projects that have the given project as their parent.
// @Tags         projects
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Success      200  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Failure      500  {object}  map[string]any
// @Router       /projects/{project_id}/children [get]
func (h *ProjectParentHandler) ListChildren(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, ok := resolveProjectIntID(w, r, h.projectStore)
	if !ok {
		return
	}

	// Verify the project exists.
	if _, err := h.projectStore.GetProject(ctx, projectID); err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to fetch project")
		return
	}

	children, err := h.projectStore.ListChildren(ctx, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list children")
		return
	}

	if children == nil {
		children = []store.Project{}
	}

	writeSuccess(w, http.StatusOK, children, "Children successfully obtained")
}
