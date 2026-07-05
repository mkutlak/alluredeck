package handlers

import (
	"net/http"
	"strconv"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// FlakyImpactHandler handles HTTP requests for flaky/retry impact analytics.
type FlakyImpactHandler struct {
	analyticsStore store.AnalyticsStorer
	branchStore    store.BranchStorer
	projectStore   store.ProjectStorer
	logger         *zap.Logger
}

// NewFlakyImpactHandler creates and returns a new FlakyImpactHandler.
func NewFlakyImpactHandler(as store.AnalyticsStorer, brs store.BranchStorer, ps store.ProjectStorer, logger *zap.Logger) *FlakyImpactHandler {
	return &FlakyImpactHandler{
		analyticsStore: as,
		branchStore:    brs,
		projectStore:   ps,
		logger:         logger,
	}
}

// GetFlakyImpact godoc
// @Summary      Get flaky-test impact
// @Description  Returns tests ranked by flaky/retry impact (wasted CI time) across recent builds.
// @Tags         analytics
// @Produce      json
// @Param        project_id  path   string  true   "Project ID"
// @Param        builds      query  int     false  "Number of recent builds to consider"  default(20)
// @Param        limit       query  int     false  "Maximum results to return"  default(20)
// @Param        branch      query  string  false  "Branch name to filter by"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Router       /projects/{project_id}/analytics/flaky [get]
func (h *FlakyImpactHandler) GetFlakyImpact(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projectID, ok := resolveProjectIntID(w, r, h.projectStore)
	if !ok {
		return
	}

	q := r.URL.Query()

	buildsParam, _ := strconv.Atoi(q.Get("builds"))
	if buildsParam <= 0 {
		buildsParam = analyticsDefaultBuilds
	}
	if buildsParam > analyticsMaxBuilds {
		buildsParam = analyticsMaxBuilds
	}

	limitParam, _ := strconv.Atoi(q.Get("limit"))
	if limitParam <= 0 {
		limitParam = analyticsDefaultLimit
	}
	if limitParam > analyticsMaxLimit {
		limitParam = analyticsMaxLimit
	}

	if h.analyticsStore == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"data": map[string]any{
				"tests":  []store.FlakyImpact{},
				"builds": buildsParam,
				"total":  0,
			},
			"metadata": map[string]string{"message": "Flaky impact successfully obtained"},
		})
		return
	}

	var branchID *int64
	if branchName := q.Get("branch"); branchName != "" {
		if h.branchStore != nil {
			br, err := h.branchStore.GetByName(ctx, projectID, branchName)
			if err == nil && br != nil {
				branchID = &br.ID
			}
		}
	}

	results, err := h.analyticsStore.ListFlakyImpact(ctx, projectID, branchID, buildsParam, limitParam)
	if err != nil {
		h.logger.Error("analytics: list flaky impact failed", zap.Int64("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "failed to retrieve analytics data")
		return
	}
	if results == nil {
		results = []store.FlakyImpact{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"tests":  results,
			"builds": buildsParam,
			"total":  len(results),
		},
		"metadata": map[string]string{"message": "Flaky impact successfully obtained"},
	})
}
