package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// knownIssueResponse is the JSON representation of a KnownIssue.
type knownIssueResponse struct {
	ID          int64  `json:"id"`
	ProjectID   string `json:"project_id"`
	TestName    string `json:"test_name"`
	Pattern     string `json:"pattern"`
	TicketURL   string `json:"ticket_url"`
	Description string `json:"description"`
	IsActive    bool   `json:"is_active"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func kiToResponse(ki *store.KnownIssue) knownIssueResponse {
	return knownIssueResponse{
		ID:          ki.ID,
		ProjectID:   ki.ProjectID,
		TestName:    ki.TestName,
		Pattern:     ki.Pattern,
		TicketURL:   ki.TicketURL,
		Description: ki.Description,
		IsActive:    ki.IsActive,
		CreatedAt:   ki.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   ki.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

// ListKnownIssues godoc
// @Summary      List known issues
// @Description  Returns all known issues for a project.
// @Tags         known-issues
// @Produce      json
// @Param        project_id  path   string  true   "Project ID"
// @Param        active_only query  bool    false  "Filter to active only"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Router       /projects/{project_id}/known-issues [get]
func (h *AllureHandler) ListKnownIssues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, ok := h.extractProjectID(w, r)
	if !ok {
		return
	}

	activeOnly := r.URL.Query().Get("active_only") == "true"
	issues, err := h.knownIssueStore.List(ctx, projectID, activeOnly)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error listing known issues")
		return
	}

	resp := make([]knownIssueResponse, 0, len(issues))
	for i := range issues {
		resp = append(resp, kiToResponse(&issues[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":     resp,
		"metadata": map[string]string{"message": "Known issues successfully obtained"},
	})
}

// CreateKnownIssue godoc
// @Summary      Create a known issue
// @Description  Records a new known issue for a project.
// @Tags         known-issues
// @Accept       json
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Success      201  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      409  {object}  map[string]any
// @Router       /projects/{project_id}/known-issues [post]
func (h *AllureHandler) CreateKnownIssue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, ok := h.extractProjectID(w, r)
	if !ok {
		return
	}

	var req struct {
		TestName    string `json:"test_name"`
		Pattern     string `json:"pattern"`
		TicketURL   string `json:"ticket_url"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.TestName == "" {
		writeError(w, http.StatusBadRequest, "test_name is required")
		return
	}
	if err := validateTicketURL(req.TicketURL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	issue, err := h.knownIssueStore.Create(ctx, projectID, req.TestName, req.Pattern, req.TicketURL, req.Description)
	if err != nil {
		if isUniqueConstraintError(err) {
			writeError(w, http.StatusConflict, "known issue with this test_name already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "Error creating known issue")
		return
	}
	if issue == nil {
		writeError(w, http.StatusInternalServerError, "Error creating known issue")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"data":     kiToResponse(issue),
		"metadata": map[string]string{"message": "Known issue created"},
	})
}

// UpdateKnownIssue godoc
// @Summary      Update a known issue
// @Description  Updates ticket_url, description, and/or active status.
// @Tags         known-issues
// @Accept       json
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Param        issue_id    path  int     true  "Known Issue ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /projects/{project_id}/known-issues/{issue_id} [put]
func (h *AllureHandler) UpdateKnownIssue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, ok := h.extractProjectID(w, r)
	if !ok {
		return
	}

	issueID, err := strconv.ParseInt(r.PathValue("issue_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid issue_id")
		return
	}

	var req struct {
		TicketURL   string `json:"ticket_url"`
		Description string `json:"description"`
		IsActive    *bool  `json:"is_active"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := validateTicketURL(req.TicketURL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Default is_active to true if not provided.
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	if err := h.knownIssueStore.Update(ctx, issueID, projectID, req.TicketURL, req.Description, isActive); err != nil {
		if errors.Is(err, store.ErrKnownIssueNotFound) {
			writeError(w, http.StatusNotFound, "known issue not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Error updating known issue")
		return
	}

	issue, err := h.knownIssueStore.Get(ctx, issueID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error fetching updated known issue")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     kiToResponse(issue),
		"metadata": map[string]string{"message": "Known issue updated"},
	})
}

// DeleteKnownIssue godoc
// @Summary      Delete a known issue
// @Description  Permanently removes a known issue record.
// @Tags         known-issues
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Param        issue_id    path  int     true  "Known Issue ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /projects/{project_id}/known-issues/{issue_id} [delete]
func (h *AllureHandler) DeleteKnownIssue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, ok := h.extractProjectID(w, r)
	if !ok {
		return
	}

	issueID, err := strconv.ParseInt(r.PathValue("issue_id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid issue_id")
		return
	}

	if err := h.knownIssueStore.Delete(ctx, issueID, projectID); err != nil {
		if errors.Is(err, store.ErrKnownIssueNotFound) {
			writeError(w, http.StatusNotFound, "known issue not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Error deleting known issue")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     map[string]any{"id": issueID},
		"metadata": map[string]string{"message": "Known issue deleted"},
	})
}

// GetReportKnownFailures godoc
// @Summary      Get known failures for a report
// @Description  Cross-references failed/broken test cases against the known issues list.
// @Tags         known-issues
// @Produce      json
// @Param        project_id  path  string  true  "Project ID"
// @Param        report_id   path  string  true  "Report ID or 'latest'"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Router       /projects/{project_id}/reports/{report_id}/known-failures [get]
func (h *AllureHandler) GetReportKnownFailures(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, ok := h.extractProjectID(w, r)
	if !ok {
		return
	}

	reportID := r.PathValue("report_id")
	if reportID == "" {
		reportID = "latest"
	}
	if err := validateReportID(reportID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// List active known issues for this project.
	knownIssues, err := h.knownIssueStore.List(ctx, projectID, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Error loading known issues")
		return
	}

	type failure struct {
		TestName string `json:"test_name"`
		Status   string `json:"status"`
	}

	knownFailures := make([]failure, 0)
	newFailures := make([]failure, 0)

	// Only scan test-cases when there are known issues to check against.
	if len(knownIssues) > 0 {
		// Build lookup map: testName -> bool
		knownMap := make(map[string]bool, len(knownIssues))
		for i := range knownIssues {
			knownMap[knownIssues[i].TestName] = true
		}

		relBase := "reports/" + reportID + "/data/test-results"
		entries, err := h.store.ReadDir(ctx, projectID, relBase)
		if err == nil {
			for _, entry := range entries {
				if entry.IsDir {
					continue
				}
				data, err := h.store.ReadFile(ctx, projectID, relBase+"/"+entry.Name)
				if err != nil {
					continue
				}
				var tc struct {
					Name   string `json:"name"`
					Status string `json:"status"`
				}
				if json.Unmarshal(data, &tc) != nil {
					continue
				}
				if tc.Status != "failed" && tc.Status != "broken" {
					continue
				}
				f := failure{TestName: tc.Name, Status: tc.Status}
				if knownMap[tc.Name] {
					knownFailures = append(knownFailures, f)
				} else {
					newFailures = append(newFailures, f)
				}
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"known_failures": knownFailures,
			"new_failures":   newFailures,
			"adjusted_stats": map[string]any{
				"known_count": len(knownFailures),
				"new_count":   len(newFailures),
				"total_count": len(knownFailures) + len(newFailures),
			},
		},
		"metadata": map[string]string{"message": "Known failures successfully obtained"},
	})
}

// isUniqueConstraintError returns true when err wraps store.ErrDuplicateEntry.
func isUniqueConstraintError(err error) bool {
	return errors.Is(err, store.ErrDuplicateEntry)
}
