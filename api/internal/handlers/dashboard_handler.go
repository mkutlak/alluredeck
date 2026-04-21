package handlers

import (
	"net/http"

	"go.uber.org/zap"

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
	BuildNumber    int                 `json:"build_number"`
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
	BuildNumber int     `json:"build_number"`
	PassRate    float64 `json:"pass_rate"`
}

type aggregateStats struct {
	Passed   int     `json:"passed"`
	Failed   int     `json:"failed"`
	Broken   int     `json:"broken"`
	Skipped  int     `json:"skipped"`
	Total    int     `json:"total"`
	PassRate float64 `json:"pass_rate"`
}

type dashboardProjectResp struct {
	ProjectID   int64                  `json:"project_id"`
	Slug        string                 `json:"slug"`
	DisplayName string                 `json:"display_name"`
	ParentID    *int64                 `json:"parent_id,omitempty"`
	ReportType  string                 `json:"report_type"`
	CreatedAt   string                 `json:"created_at"`
	LatestBuild *latestBuildResp       `json:"latest_build"`
	Sparkline   []sparklinePointResp   `json:"sparkline"`
	IsGroup     bool                   `json:"is_group"`
	Children    []dashboardProjectResp `json:"children,omitempty"`
	Aggregate   *aggregateStats        `json:"aggregate,omitempty"`
}

type dashboardSummaryResp struct {
	TotalProjects int `json:"total_projects"`
	Healthy       int `json:"healthy"`
	Degraded      int `json:"degraded"`
	Failing       int `json:"failing"`
}

// DashboardHandler handles HTTP requests for the cross-project dashboard.
type DashboardHandler struct {
	buildStore store.BuildStorer
	logger     *zap.Logger
}

// NewDashboardHandler creates and returns a new DashboardHandler.
func NewDashboardHandler(bs store.BuildStorer, logger *zap.Logger) *DashboardHandler {
	return &DashboardHandler{
		buildStore: bs,
		logger:     logger,
	}
}

// GetDashboard godoc
// @Summary      Get cross-project dashboard data
// @Description  Returns all projects with their latest build status and pass rate sparklines.
// @Tags         dashboard
// @Produce      json
// @Success      200  {object}  map[string]any
// @Failure      500  {object}  map[string]any
// @Router       /dashboard [get]
func (h *DashboardHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	projects, err := h.buildStore.GetDashboardData(ctx, 10)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	// Build lookup: parent_id -> child responses.
	childrenOf := map[int64][]dashboardProjectResp{}
	for _, dp := range projects {
		if dp.ParentID != nil {
			childrenOf[*dp.ParentID] = append(childrenOf[*dp.ParentID], buildProjectResp(dp))
		}
	}

	var projectResps []dashboardProjectResp
	summary := dashboardSummaryResp{}

	for _, dp := range projects {
		if dp.ParentID != nil {
			continue // children appear nested, not at top level
		}
		pr := buildProjectResp(dp)
		if children, ok := childrenOf[dp.ProjectID]; ok {
			pr.IsGroup = true
			pr.Children = children
			pr.Aggregate = computeAggregate(children)
			// Health classification for a group uses aggregate pass rate.
			if pr.Aggregate.Total == 0 {
				summary.Failing++
			} else {
				switch {
				case pr.Aggregate.PassRate >= healthyThreshold:
					summary.Healthy++
				case pr.Aggregate.PassRate >= degradedThreshold:
					summary.Degraded++
				default:
					summary.Failing++
				}
			}
		} else {
			// Standalone project: classify by its own latest build.
			if pr.LatestBuild != nil {
				switch {
				case pr.LatestBuild.PassRate >= healthyThreshold:
					summary.Healthy++
				case pr.LatestBuild.PassRate >= degradedThreshold:
					summary.Degraded++
				default:
					summary.Failing++
				}
			} else {
				// No builds: counts as failing.
				summary.Failing++
			}
		}
		projectResps = append(projectResps, pr)
	}
	if projectResps == nil {
		projectResps = []dashboardProjectResp{}
	}
	summary.TotalProjects = len(projectResps)

	writeJSON(w, http.StatusOK, map[string]any{
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
		BuildNumber: b.BuildNumber,
		CreatedAt:   b.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
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

// buildProjectResp converts a store.DashboardProject into a dashboardProjectResp.
func buildProjectResp(dp store.DashboardProject) dashboardProjectResp {
	reportType := dp.ReportType
	if reportType == "" {
		reportType = "allure"
	}
	displayName := dp.DisplayName
	if displayName == "" {
		displayName = dp.Slug
	}
	pr := dashboardProjectResp{
		ProjectID:   dp.ProjectID,
		Slug:        dp.Slug,
		ParentID:    dp.ParentID,
		DisplayName: displayName,
		ReportType:  reportType,
		CreatedAt:   dp.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		Sparkline:   buildSparkline(dp.Sparkline),
	}
	if dp.Latest != nil {
		pr.LatestBuild = buildLatestResp(dp.Latest)
	}
	return pr
}

// computeAggregate sums the latest-build stats of all children and derives pass rate.
func computeAggregate(children []dashboardProjectResp) *aggregateStats {
	agg := &aggregateStats{}
	for i := range children {
		if children[i].LatestBuild == nil {
			continue
		}
		s := children[i].LatestBuild.Statistics
		agg.Passed += s.Passed
		agg.Failed += s.Failed
		agg.Broken += s.Broken
		agg.Skipped += s.Skipped
		agg.Total += s.Total
	}
	agg.PassRate = pct(agg.Passed, agg.Total)
	return agg
}

// buildSparkline converts store.SparklinePoint slice into response type,
// always returning a non-nil slice.
func buildSparkline(points []store.SparklinePoint) []sparklinePointResp {
	result := make([]sparklinePointResp, 0, len(points))
	for _, sp := range points {
		result = append(result, sparklinePointResp{
			BuildNumber: sp.BuildNumber,
			PassRate:    sp.PassRate,
		})
	}
	return result
}
