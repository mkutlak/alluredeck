package handlers

import (
	"net/http"
	"strconv"

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
	projectsDir string
}

// NewSearchHandler creates and returns a new SearchHandler.
func NewSearchHandler(ss store.SearchStorer, projectsDir string) *SearchHandler {
	return &SearchHandler{
		searchStore: ss,
		projectsDir: projectsDir,
	}
}

// Search handles GET /api/v1/search?q=<term>&limit=<n>.
// Returns matching projects and test names from latest builds.
func (h *SearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
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

	type projectEntry struct {
		ProjectID string `json:"project_id"`
		CreatedAt string `json:"created_at"`
	}
	type testEntry struct {
		ProjectID string `json:"project_id"`
		TestName  string `json:"test_name"`
		FullName  string `json:"full_name"`
		Status    string `json:"status"`
	}

	pEntries := make([]projectEntry, 0, len(projects))
	for _, p := range projects {
		pEntries = append(pEntries, projectEntry{
			ProjectID: p.ID,
			CreatedAt: p.CreatedAt.Format("2006-01-02T15:04:05Z"),
		})
	}

	tEntries := make([]testEntry, 0, len(tests))
	for _, t := range tests {
		tEntries = append(tEntries, testEntry{
			ProjectID: t.ProjectID,
			TestName:  t.TestName,
			FullName:  t.FullName,
			Status:    t.Status,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"projects": pEntries,
			"tests":    tEntries,
		},
		"metadata": map[string]string{"message": "Search results"},
	})
}
