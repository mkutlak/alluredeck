package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

const (
	searchMinLen     = 2
	searchMaxLen     = 100
	searchDefaultLim = 10
	searchMaxLim     = 50
)

// SearchHandler handles HTTP requests for search operations.
type SearchHandler struct {
	searchStore store.SearchStorer
}

// NewSearchHandler creates and returns a new SearchHandler.
func NewSearchHandler(ss store.SearchStorer) *SearchHandler {
	return &SearchHandler{
		searchStore: ss,
	}
}

// Search handles GET /api/v1/search?q=<term>&limit=<n>.
// Returns matching projects, test names, pipeline runs and branches.
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeError(w, http.StatusBadRequest, "query parameter 'q' is required")
		return
	}
	if len(q) < searchMinLen {
		writeError(w, http.StatusBadRequest, "query must be at least 2 characters")
		return
	}
	if len(q) > searchMaxLen {
		writeError(w, http.StatusBadRequest, "query must not exceed 100 characters")
		return
	}

	limit := searchDefaultLim
	if lStr := r.URL.Query().Get("limit"); lStr != "" {
		if l, err := strconv.Atoi(lStr); err == nil && l > 0 {
			limit = l
		}
	}
	if limit > searchMaxLim {
		limit = searchMaxLim
	}

	projects, err := h.searchStore.SearchProjects(r.Context(), q, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	tests, err := h.searchStore.SearchTests(r.Context(), q, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	runs, err := h.searchStore.SearchPipelineRuns(r.Context(), q, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	branches, err := h.searchStore.SearchBranches(r.Context(), q, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "search failed")
		return
	}

	type projectEntry struct {
		ProjectID int64  `json:"project_id"`
		Slug      string `json:"slug"`
		CreatedAt string `json:"created_at"`
	}
	type testEntry struct {
		ProjectID int64  `json:"project_id"`
		Slug      string `json:"slug"`
		TestName  string `json:"test_name"`
		FullName  string `json:"full_name"`
		Status    string `json:"status"`
	}
	type runEntry struct {
		GroupProjectID int64  `json:"group_project_id"`
		GroupSlug      string `json:"group_slug"`
		PipelineID     string `json:"pipeline_id"`
		Branch         string `json:"branch"`
		Timestamp      string `json:"timestamp"`
	}
	type branchEntry struct {
		Name string `json:"name"`
	}

	pEntries := make([]projectEntry, 0, len(projects))
	for _, p := range projects {
		pEntries = append(pEntries, projectEntry{
			ProjectID: p.ID,
			Slug:      p.Slug,
			CreatedAt: p.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	tEntries := make([]testEntry, 0, len(tests))
	for _, t := range tests {
		tEntries = append(tEntries, testEntry{
			ProjectID: t.ProjectID,
			Slug:      t.Slug,
			TestName:  t.TestName,
			FullName:  t.FullName,
			Status:    t.Status,
		})
	}

	rEntries := make([]runEntry, 0, len(runs))
	for _, m := range runs {
		rEntries = append(rEntries, runEntry{
			GroupProjectID: m.GroupProjectID,
			GroupSlug:      m.GroupSlug,
			PipelineID:     m.PipelineID,
			Branch:         m.Branch,
			Timestamp:      m.Timestamp.UTC().Format("2006-01-02T15:04:05Z"),
		})
	}

	bEntries := make([]branchEntry, 0, len(branches))
	for _, m := range branches {
		bEntries = append(bEntries, branchEntry{Name: m.Name})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"projects": pEntries,
			"tests":    tEntries,
			"runs":     rEntries,
			"branches": bEntries,
		},
		"metadata": map[string]string{"message": "Search results"},
	})
}
