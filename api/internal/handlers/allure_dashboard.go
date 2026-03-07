package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// Health thresholds for pass rate classification.
const (
	healthyThreshold  = 90.0
	degradedThreshold = 70.0
)

type dashboardStatistics struct {
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Broken  int `json:"broken"`
	Skipped int `json:"skipped"`
	Unknown int `json:"unknown"`
	Total   int `json:"total"`
}

type latestBuildResp struct {
	BuildOrder     int                 `json:"build_order"`
	CreatedAt      string              `json:"created_at"`
	Statistics     dashboardStatistics `json:"statistics"`
	PassRate       float64             `json:"pass_rate"`
	DurationMs     int64               `json:"duration_ms"`
	FlakyCount     int                 `json:"flaky_count"`
	NewFailedCount int                 `json:"new_failed_count"`
	NewPassedCount int                 `json:"new_passed_count"`
	CIBranch       string              `json:"ci_branch,omitempty"`
}

type sparklinePointResp struct {
	BuildOrder int     `json:"build_order"`
	PassRate   float64 `json:"pass_rate"`
}

type dashboardProjectResp struct {
	ProjectID   string               `json:"project_id"`
	CreatedAt   string               `json:"created_at"`
	Tags        []string             `json:"tags"`
	LatestBuild *latestBuildResp     `json:"latest_build"`
	Sparkline   []sparklinePointResp `json:"sparkline"`
}

type dashboardSummaryResp struct {
	TotalProjects int `json:"total_projects"`
	Healthy       int `json:"healthy"`
	Degraded      int `json:"degraded"`
	Failing       int `json:"failing"`
}

// GetDashboard godoc
// @Summary      Get cross-project dashboard data
// @Description  Returns all projects with their latest build status and pass rate sparklines.
// @Tags         dashboard
// @Produce      json
// @Success      200  {object}  map[string]any
// @Failure      500  {object}  map[string]any
// @Router       /dashboard [get]
func (h *AllureHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ctx := r.Context()

	tag := r.URL.Query().Get("tag")
	projects, err := h.buildStore.GetDashboardData(ctx, 10, tag)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]string{"message": "internal error"},
		})
		return
	}

	projectResps := make([]dashboardProjectResp, 0, len(projects))
	summary := dashboardSummaryResp{TotalProjects: len(projects)}

	for _, dp := range projects {
		tags := dp.Tags
		if tags == nil {
			tags = []string{}
		}
		pr := dashboardProjectResp{
			ProjectID: dp.ProjectID,
			CreatedAt: dp.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
			Tags:      tags,
			Sparkline: buildSparkline(dp.Sparkline),
		}

		if dp.Latest != nil {
			pr.LatestBuild = buildLatestResp(dp.Latest)
			passRate := pr.LatestBuild.PassRate
			switch {
			case passRate >= healthyThreshold:
				summary.Healthy++
			case passRate >= degradedThreshold:
				summary.Degraded++
			default:
				summary.Failing++
			}
		} else {
			// No builds: counts as failing.
			summary.Failing++
		}

		projectResps = append(projectResps, pr)
	}

	_ = json.NewEncoder(w).Encode(map[string]any{
		"data": map[string]any{
			"projects": projectResps,
			"summary":  summary,
		},
		"metadata": map[string]string{"message": "Dashboard data successfully obtained"},
	})
}

// buildLatestResp converts a store.Build into a latestBuildResp.
func buildLatestResp(b *store.Build) *latestBuildResp {
	passed := derefInt(b.StatPassed)
	total := derefInt(b.StatTotal)
	return &latestBuildResp{
		BuildOrder: b.BuildOrder,
		CreatedAt:  b.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Statistics: dashboardStatistics{
			Passed:  passed,
			Failed:  derefInt(b.StatFailed),
			Broken:  derefInt(b.StatBroken),
			Skipped: derefInt(b.StatSkipped),
			Unknown: derefInt(b.StatUnknown),
			Total:   total,
		},
		PassRate:       pct(passed, total),
		DurationMs:     derefInt64(b.DurationMs),
		FlakyCount:     derefInt(b.FlakyCount),
		NewFailedCount: derefInt(b.NewFailedCount),
		NewPassedCount: derefInt(b.NewPassedCount),
		CIBranch:       derefStr(b.CIBranch),
	}
}

// buildSparkline converts store.SparklinePoint slice into response type,
// always returning a non-nil slice.
func buildSparkline(points []store.SparklinePoint) []sparklinePointResp {
	result := make([]sparklinePointResp, 0, len(points))
	for _, sp := range points {
		result = append(result, sparklinePointResp{
			BuildOrder: sp.BuildOrder,
			PassRate:   sp.PassRate,
		})
	}
	return result
}
