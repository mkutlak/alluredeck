package handlers

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// compareDiffEntry is the JSON shape for one test in the comparison response.
type compareDiffEntry struct {
	TestName      string `json:"test_name"`
	FullName      string `json:"full_name"`
	HistoryID     string `json:"history_id"`
	StatusA       string `json:"status_a"`
	StatusB       string `json:"status_b"`
	DurationA     int64  `json:"duration_a"`
	DurationB     int64  `json:"duration_b"`
	DurationDelta int64  `json:"duration_delta"`
	Category      string `json:"category"`
}

// compareSummary holds counts per diff category.
type compareSummary struct {
	Regressed int `json:"regressed"`
	Fixed     int `json:"fixed"`
	Added     int `json:"added"`
	Removed   int `json:"removed"`
	Total     int `json:"total"`
}

// CompareHandler handles HTTP requests for build comparison.
type CompareHandler struct {
	testResultStore store.TestResultStorer
	projectsDir     string
}

// NewCompareHandler creates and returns a new CompareHandler.
func NewCompareHandler(trs store.TestResultStorer, projectsDir string) *CompareHandler {
	return &CompareHandler{
		testResultStore: trs,
		projectsDir:     projectsDir,
	}
}

// CompareBuilds godoc
// @Summary      Compare two builds
// @Description  Returns a diff of test statuses between two builds within a project.
// @Tags         compare
// @Produce      json
// @Param        project_id  path   string  true  "Project ID"
// @Param        a           query  int     true  "Build order A (baseline)"
// @Param        b           query  int     true  "Build order B (target)"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Router       /projects/{project_id}/compare [get]
func (h *CompareHandler) CompareBuilds(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, ok := extractProjectID(w, r, h.projectsDir)
	if !ok {
		return
	}

	q := r.URL.Query()
	buildOrderA, errA := strconv.Atoi(q.Get("a"))
	buildOrderB, errB := strconv.Atoi(q.Get("b"))
	if errA != nil || errB != nil || buildOrderA <= 0 || buildOrderB <= 0 {
		writeError(w, http.StatusBadRequest, "query parameters 'a' and 'b' are required and must be positive integers")
		return
	}
	if buildOrderA == buildOrderB {
		writeError(w, http.StatusBadRequest, "build_a and build_b must be different")
		return
	}

	// No store: return empty comparison (same pattern as analytics).
	if h.testResultStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"build_a": buildOrderA,
				"build_b": buildOrderB,
				"summary": compareSummary{},
				"tests":   []compareDiffEntry{},
			},
			"metadata": map[string]string{"message": "Build comparison successfully obtained"},
		})
		return
	}

	buildIDA, err := h.testResultStore.GetBuildID(ctx, projectID, buildOrderA)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("build #%d not found for project %s", buildOrderA, projectID))
		return
	}
	buildIDB, err := h.testResultStore.GetBuildID(ctx, projectID, buildOrderB)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("build #%d not found for project %s", buildOrderB, projectID))
		return
	}

	diffs, err := h.testResultStore.CompareBuildsByHistoryID(ctx, projectID, buildIDA, buildIDB)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to compute build comparison")
		return
	}

	entries, summary := mapDiffEntries(diffs)

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"build_a": buildOrderA,
			"build_b": buildOrderB,
			"summary": summary,
			"tests":   entries,
		},
		"metadata": map[string]string{"message": "Build comparison successfully obtained"},
	})
}

// mapDiffEntries converts store.DiffEntry slice to JSON-ready structs and computes summary.
func mapDiffEntries(diffs []store.DiffEntry) ([]compareDiffEntry, compareSummary) {
	entries := make([]compareDiffEntry, 0, len(diffs))
	var summary compareSummary

	for _, d := range diffs {
		entries = append(entries, compareDiffEntry{
			TestName:      d.TestName,
			FullName:      d.FullName,
			HistoryID:     d.HistoryID,
			StatusA:       d.StatusA,
			StatusB:       d.StatusB,
			DurationA:     d.DurationA,
			DurationB:     d.DurationB,
			DurationDelta: d.DurationB - d.DurationA,
			Category:      string(d.Category),
		})
		switch d.Category {
		case store.DiffRegressed:
			summary.Regressed++
		case store.DiffFixed:
			summary.Fixed++
		case store.DiffAdded:
			summary.Added++
		case store.DiffRemoved:
			summary.Removed++
		}
	}
	summary.Total = len(diffs)
	return entries, summary
}
