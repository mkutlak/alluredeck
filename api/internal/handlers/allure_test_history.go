package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

const defaultTestHistoryLimit = 20
const maxTestHistoryLimit = 100

// TestHistoryHandler handles HTTP requests for per-test history.
type TestHistoryHandler struct {
	testResultStore store.TestResultStorer
	buildStore      store.BuildStorer
	branchStore     store.BranchStorer
	projectsDir     string
}

// NewTestHistoryHandler creates a new TestHistoryHandler.
func NewTestHistoryHandler(ts store.TestResultStorer, bs store.BuildStorer, brs store.BranchStorer, projectsDir string) *TestHistoryHandler {
	return &TestHistoryHandler{
		testResultStore: ts,
		buildStore:      bs,
		branchStore:     brs,
		projectsDir:     projectsDir,
	}
}

// testHistoryEntryJSON is the JSON shape for one history entry.
type testHistoryEntryJSON struct {
	BuildNumber int       `json:"build_number"`
	BuildID     int64     `json:"build_id"`
	Status      string    `json:"status"`
	DurationMs  int64     `json:"duration_ms"`
	CreatedAt   time.Time `json:"created_at"`
	CICommitSHA *string   `json:"ci_commit_sha,omitempty"`
}

// GetTestHistory godoc
// @Summary      Get test history
// @Description  Returns the run history for a specific test (identified by history_id) within a project.
// @Tags         tests
// @Produce      json
// @Param        project_id  path   string  true   "Project ID"
// @Param        history_id  query  string  true   "Test history ID"
// @Param        branch      query  string  false  "Branch name filter"
// @Param        limit       query  int     false  "Max entries"  default(50)
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /projects/{project_id}/test-history [get]
func (h *TestHistoryHandler) GetTestHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, ok := extractProjectID(w, r, h.projectsDir)
	if !ok {
		return
	}
	historyID := r.URL.Query().Get("history_id")
	if historyID == "" {
		writeError(w, http.StatusBadRequest, "history_id query parameter is required")
		return
	}

	// Parse optional limit (default 20, max 100).
	limit := defaultTestHistoryLimit
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			if n > maxTestHistoryLimit {
				n = maxTestHistoryLimit
			}
			limit = n
		}
	}

	// Resolve optional branch filter.
	var branchID *int64
	var resolvedBranchName string
	if branchName := r.URL.Query().Get("branch"); branchName != "" && h.branchStore != nil {
		branch, err := h.branchStore.GetByName(ctx, projectID, branchName)
		if err != nil {
			if errors.Is(err, store.ErrBranchNotFound) {
				writeError(w, http.StatusNotFound, "branch not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to resolve branch")
			return
		}
		branchID = &branch.ID
		resolvedBranchName = branch.Name
	}

	entries, err := h.testResultStore.GetTestHistory(ctx, projectID, historyID, branchID, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch test history")
		return
	}

	out := make([]testHistoryEntryJSON, 0, len(entries))
	for _, e := range entries {
		out = append(out, testHistoryEntryJSON{
			BuildNumber: e.BuildNumber,
			BuildID:     e.BuildID,
			Status:      e.Status,
			DurationMs:  e.DurationMs,
			CreatedAt:   e.CreatedAt,
			CICommitSHA: e.CICommitSHA,
		})
	}

	data := map[string]any{
		"history_id": historyID,
		"history":    out,
	}
	if resolvedBranchName != "" {
		data["branch_name"] = resolvedBranchName
	}

	writeSuccess(w, http.StatusOK, data, "Test history successfully obtained")
}
