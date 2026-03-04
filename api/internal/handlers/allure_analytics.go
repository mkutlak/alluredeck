package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
)

const (
	analyticsDefaultBuilds = 20
	analyticsMaxBuilds     = 100
	analyticsDefaultLimit  = 20
	analyticsMaxLimit      = 100
)

// analyticsTestEntry is the response shape for one low-performing test.
type analyticsTestEntry struct {
	TestName   string    `json:"test_name"`
	FullName   string    `json:"full_name"`
	HistoryID  string    `json:"history_id"`
	Metric     float64   `json:"metric"`
	BuildCount int       `json:"build_count"`
	Trend      []float64 `json:"trend"`
}

// GetLowPerformingTests godoc
// @Summary      Get low-performing tests
// @Description  Returns tests ranked by average duration or failure rate across recent builds.
// @Tags         analytics
// @Produce      json
// @Param        project_id  path   string  true   "Project ID"
// @Param        sort        query  string  false  "Sort by: duration or failure_rate"  default(duration)
// @Param        builds      query  int     false  "Number of recent builds to consider"  default(20)
// @Param        limit       query  int     false  "Maximum results to return"  default(20)
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Router       /projects/{project_id}/analytics/low-performing [get]
func (h *AllureHandler) GetLowPerformingTests(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ctx := r.Context()

	raw := r.PathValue("project_id")
	unescaped, err := url.PathUnescape(raw)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]string{"message": "invalid project_id encoding"},
		})
		return
	}
	projectID, err := safeProjectID(h.cfg.ProjectsPath, unescaped)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]string{"message": err.Error()},
		})
		return
	}

	q := r.URL.Query()
	sortBy := q.Get("sort")
	if sortBy == "" {
		sortBy = "duration"
	}
	if sortBy != "duration" && sortBy != "failure_rate" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]string{"message": "sort must be 'duration' or 'failure_rate'"},
		})
		return
	}

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

	if h.testResultStore == nil {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"tests":  []analyticsTestEntry{},
				"sort":   sortBy,
				"builds": buildsParam,
				"total":  0,
			},
			"metadata": map[string]string{"message": "Low-performing tests successfully obtained"},
		})
		return
	}

	var entries []analyticsTestEntry
	if sortBy == "duration" {
		results, err := h.testResultStore.ListSlowest(ctx, projectID, buildsParam, limitParam)
		if err == nil {
			for _, t := range results {
				trend := t.Trend
				if trend == nil {
					trend = []float64{}
				}
				entries = append(entries, analyticsTestEntry{
					TestName:   t.TestName,
					FullName:   t.FullName,
					HistoryID:  t.HistoryID,
					Metric:     t.Metric,
					BuildCount: t.BuildCount,
					Trend:      trend,
				})
			}
		}
	} else {
		results, err := h.testResultStore.ListLeastReliable(ctx, projectID, buildsParam, limitParam)
		if err == nil {
			for _, t := range results {
				trend := t.Trend
				if trend == nil {
					trend = []float64{}
				}
				entries = append(entries, analyticsTestEntry{
					TestName:   t.TestName,
					FullName:   t.FullName,
					HistoryID:  t.HistoryID,
					Metric:     t.Metric,
					BuildCount: t.BuildCount,
					Trend:      trend,
				})
			}
		}
	}

	if entries == nil {
		entries = []analyticsTestEntry{}
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]any{
			"tests":  entries,
			"sort":   sortBy,
			"builds": buildsParam,
			"total":  len(entries),
		},
		"metadata": map[string]string{"message": "Low-performing tests successfully obtained"},
	})
}
