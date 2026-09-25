package handlers

import (
	"context"
	"net/http"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestGetDashboard(t *testing.T) {
	latest := func(number, passed int) *store.Build {
		return &store.Build{BuildNumber: number, StatPassed: new(passed), StatTotal: new(100)}
	}
	tests := []struct {
		name     string
		projects []store.DashboardProject
		want     map[string]any
	}{
		{name: "no projects", projects: []store.DashboardProject{}, want: map[string]any{
			"data.projects#": 0, "data.summary.total_projects": 0,
		}},
		// Health buckets: >=90% healthy, >=70% degraded, below that or no
		// builds at all failing.
		{name: "health buckets", projects: []store.DashboardProject{
			{ProjectID: 1, Slug: "healthy", Latest: latest(3, 95), Sparkline: []store.SparklinePoint{
				{BuildNumber: 1, PassRate: 80}, {BuildNumber: 2, PassRate: 85}, {BuildNumber: 3, PassRate: 95},
			}},
			{ProjectID: 2, Slug: "degraded", Latest: latest(1, 80)},
			{ProjectID: 3, Slug: "failing", Latest: latest(1, 50)},
			{ProjectID: 4, Slug: "no-builds"},
		}, want: map[string]any{
			"data.projects#":                            4,
			"data.projects.0.project_id":                1,
			"data.projects.0.latest_build.build_number": 3,
			"data.projects.0.latest_build.pass_rate":    95.0,
			"data.projects.0.sparkline#":                3,
			"data.projects.3.latest_build":              nil,
			"data.projects.3.sparkline#":                0,
			"data.summary.total_projects":               4,
			"data.summary.healthy":                      1,
			"data.summary.degraded":                     1,
			"data.summary.failing":                      2,
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mocks := testutil.New()
			mocks.Builds.GetDashboardDataFn = func(context.Context, int) ([]store.DashboardProject, error) {
				return tc.projects, nil
			}
			h := NewDashboardHandler(mocks.Builds, zap.NewNop())
			code, body := serveJSON(t, h.GetDashboard, http.MethodGet, "/api/v1/dashboard", "")
			if code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %v", code, body)
			}
			wantJSON(t, body, tc.want)
		})
	}
}
