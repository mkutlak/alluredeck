package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

const (
	maxTagLength = 50
	maxTagCount  = 20
)

// tagPattern allows alphanumeric characters, hyphens, and underscores.
var tagPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

type updateTagsRequest struct {
	Tags []string `json:"tags"`
}

// UpdateProjectTags godoc
// @Summary      Set project tags
// @Description  Replaces the tags on a project. Admin only.
// @Tags         projects
// @Accept       json
// @Produce      json
// @Param        project_id  path  string           true  "Project ID"
// @Param        body        body  updateTagsRequest true  "Tags payload"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /projects/{project_id}/tags [put]
func (h *AllureHandler) UpdateProjectTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID := r.PathValue("project_id")
	if err := validateProjectID(h.cfg.ProjectsPath, projectID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var req updateTagsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	if req.Tags == nil {
		req.Tags = []string{}
	}

	// Validate tags.
	if len(req.Tags) > maxTagCount {
		writeError(w, http.StatusBadRequest, "too many tags: maximum 20 allowed")
		return
	}

	seen := make(map[string]struct{}, len(req.Tags))
	cleaned := make([]string, 0, len(req.Tags))
	for _, tag := range req.Tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if len(tag) > maxTagLength {
			writeError(w, http.StatusBadRequest, "tag too long: maximum 50 characters")
			return
		}
		if !tagPattern.MatchString(tag) {
			writeError(w, http.StatusBadRequest, "tag contains invalid characters: only alphanumeric, hyphens, and underscores allowed")
			return
		}
		if _, dup := seen[tag]; dup {
			continue // silently deduplicate
		}
		seen[tag] = struct{}{}
		cleaned = append(cleaned, tag)
	}

	if err := h.projectStore.SetTags(ctx, projectID, cleaned); err != nil {
		if errors.Is(err, store.ErrProjectNotFound) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to update tags")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"project_id": projectID,
			"tags":       cleaned,
		},
		"metadata": map[string]string{"message": "Tags successfully updated"},
	})
}

// ListTags godoc
// @Summary      List all distinct tags
// @Description  Returns all distinct tags across all projects, sorted alphabetically.
// @Tags         projects
// @Produce      json
// @Success      200  {object}  map[string]any
// @Failure      500  {object}  map[string]any
// @Router       /tags [get]
func (h *AllureHandler) ListTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tags, err := h.projectStore.ListAllTags(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list tags")
		return
	}

	if tags == nil {
		tags = []string{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     tags,
		"metadata": map[string]string{"message": "Tags successfully obtained"},
	})
}
