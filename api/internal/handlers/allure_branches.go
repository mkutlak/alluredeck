package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// BranchHandler handles HTTP requests for branch management.
type BranchHandler struct {
	branchStore store.BranchStorer
	buildStore  store.BuildStorer
}

// NewBranchHandler creates a new BranchHandler.
func NewBranchHandler(bs store.BranchStorer, buildStore store.BuildStorer) *BranchHandler {
	return &BranchHandler{
		branchStore: bs,
		buildStore:  buildStore,
	}
}

// branchJSON is the JSON representation of a branch.
type branchJSON struct {
	ID        int64     `json:"id"`
	ProjectID string    `json:"project_id"`
	Name      string    `json:"name"`
	IsDefault bool      `json:"is_default"`
	CreatedAt time.Time `json:"created_at"`
}

func branchToJSON(b store.Branch) branchJSON {
	return branchJSON{
		ID:        b.ID,
		ProjectID: b.ProjectID,
		Name:      b.Name,
		IsDefault: b.IsDefault,
		CreatedAt: b.CreatedAt,
	}
}

// ListBranches godoc
// @Summary      List branches
// @Description  Returns all branches for a project.
// @Tags         branches
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Router       /projects/{project_id}/branches [get]
func (h *BranchHandler) ListBranches(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID := r.PathValue("project_id")

	branches, err := h.branchStore.List(ctx, projectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list branches")
		return
	}

	out := make([]branchJSON, 0, len(branches))
	for _, b := range branches {
		out = append(out, branchToJSON(b))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"branches": out,
		},
		"metadata": map[string]string{"message": "Branches successfully obtained"},
	})
}

// SetDefaultBranch godoc
// @Summary      Set default branch
// @Description  Sets the specified branch as the default for the project.
// @Tags         branches
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Param        branch_id   path  int     true  "Branch ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /projects/{project_id}/branches/{branch_id}/default [put]
func (h *BranchHandler) SetDefaultBranch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID := r.PathValue("project_id")

	branchIDStr := r.PathValue("branch_id")
	branchID, err := strconv.ParseInt(branchIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "branch_id must be a positive integer")
		return
	}

	if err := h.branchStore.SetDefault(ctx, projectID, branchID); err != nil {
		if errors.Is(err, store.ErrBranchNotFound) {
			writeError(w, http.StatusNotFound, "branch not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to set default branch")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"metadata": map[string]string{"message": "Default branch successfully updated"},
	})
}

// DeleteBranch godoc
// @Summary      Delete branch
// @Description  Deletes a non-default branch from the project.
// @Tags         branches
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Param        branch_id   path  int     true  "Branch ID"
// @Success      204  "No Content"
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Failure      409  {object}  map[string]any
// @Router       /projects/{project_id}/branches/{branch_id} [delete]
func (h *BranchHandler) DeleteBranch(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID := r.PathValue("project_id")

	branchIDStr := r.PathValue("branch_id")
	branchID, err := strconv.ParseInt(branchIDStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "branch_id must be a positive integer")
		return
	}

	if err := h.branchStore.Delete(ctx, projectID, branchID); err != nil {
		if errors.Is(err, store.ErrCannotDeleteDefaultBranch) {
			writeError(w, http.StatusConflict, "cannot delete default branch")
			return
		}
		if errors.Is(err, store.ErrBranchNotFound) {
			writeError(w, http.StatusNotFound, "branch not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete branch")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
