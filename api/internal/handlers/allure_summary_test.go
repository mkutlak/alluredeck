package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestGetReportSummary(t *testing.T) {
	tests := []struct {
		name      string
		projectID string // "" targets the seeded project
		reportID  string
		want      int
		wantJSON  map[string]any
	}{
		// Build 3 has 15 failures, of which the handler asks the store for the
		// top 10, and a previous build 1 to compute the trend against.
		{name: "numeric report id", reportID: "3", want: http.StatusOK, wantJSON: map[string]any{
			"data.build.build_number":          3,
			"data.build.is_latest":             true,
			"data.build.ci_provider":           "GitHub Actions",
			"data.statistics.passed":           85,
			"data.statistics.total":            100,
			"data.statistics.passed_pct":       85.0,
			"data.timing.duration_ms":          45000,
			"data.quality.flaky_count":         2,
			"data.quality.new_failed_count":    3,
			"data.top_failures#":               10,
			"data.top_failures.0.test_name":    "Login timeout",
			"data.top_failures.0.new_failed":   true,
			"data.trend.previous_build_number": 1,
			"data.trend.passed_delta":          -5,
			"data.trend.failed_delta":          5,
			"data.trend.duration_delta_ms":     5000,
		}},
		{name: "latest", reportID: "latest", want: http.StatusOK, wantJSON: map[string]any{"data.build.build_number": 2}},
		{name: "build not found", reportID: "99", want: http.StatusNotFound},
		{name: "invalid project id", projectID: "../evil", reportID: "1", want: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mocks := testutil.New()
			proj, err := mocks.Projects.CreateProject(context.Background(), "summary-proj")
			if err != nil {
				t.Fatal(err)
			}
			mocks.Builds.GetBuildByNumberFn = func(_ context.Context, pid int64, n int) (store.Build, error) {
				if pid != proj.ID || n != 3 {
					return store.Build{}, store.ErrBuildNotFound
				}
				return store.Build{
					ID: 100, BuildNumber: 3, IsLatest: true, CIProvider: new("GitHub Actions"),
					StatPassed: new(85), StatFailed: new(10), StatBroken: new(3), StatSkipped: new(2), StatTotal: new(100),
					DurationMs: new(int64(45000)), FlakyCount: new(2), NewFailedCount: new(3), NewPassedCount: new(1),
				}, nil
			}
			mocks.Builds.GetLatestBuildFn = func(context.Context, int64) (store.Build, error) {
				return store.Build{ID: 2, BuildNumber: 2, IsLatest: true, StatPassed: new(50), StatTotal: new(50)}, nil
			}
			mocks.Builds.GetPreviousBuildFn = func(_ context.Context, _ int64, n int) (store.Build, error) {
				if n != 3 {
					return store.Build{}, store.ErrBuildNotFound
				}
				return store.Build{
					BuildNumber: 1, StatPassed: new(90), StatFailed: new(5), StatBroken: new(2), StatSkipped: new(3), StatTotal: new(100),
					DurationMs: new(int64(40000)),
				}, nil
			}
			failures := []store.TestResult{{TestName: "Login timeout", Status: "failed", NewFailed: true}}
			for i := range 14 {
				failures = append(failures, store.TestResult{TestName: fmt.Sprintf("Fail%d", i), Status: "failed"})
			}
			mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _, _ int64, limit int) ([]store.TestResult, error) {
				return failures[:min(limit, len(failures))], nil
			}
			h := newTestReportHandlerWithMocks(t, t.TempDir(), mocks)
			id := tc.projectID
			if id == "" {
				id = strconv.FormatInt(proj.ID, 10)
			}
			code, body := serveJSON(t, h.GetReportSummary, http.MethodGet, "/api/v1/projects/"+id+"/reports/"+tc.reportID+"/summary", "",
				"project_id", id, "report_id", tc.reportID)
			if code != tc.want {
				t.Fatalf("status = %d, want %d: %v", code, tc.want, body)
			}
			wantJSON(t, body, tc.wantJSON)
			if code == http.StatusOK {
				wantJSON(t, body, map[string]any{"data.build.project_id": proj.ID})
			}
		})
	}
}
