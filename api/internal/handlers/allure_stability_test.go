package handlers

import (
	"context"
	"net/http"
	"strconv"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestGetReportStability serves stability data from the database: the summary
// counters come from the build row, the lists from its classified results.
func TestGetReportStability(t *testing.T) {
	tests := []struct {
		name     string
		reportID string
		want     int
		wantJSON map[string]any
	}{
		{name: "numeric report id", reportID: "5", want: http.StatusOK, wantJSON: map[string]any{
			"data.flaky_tests#": 1, "data.new_failed#": 1, "data.new_passed#": 1,
			"data.summary.flaky_count": 2, "data.summary.total": 100,
		}},
		// Lists stay [] rather than null when the build has no flagged results.
		{name: "latest", reportID: "latest", want: http.StatusOK, wantJSON: map[string]any{
			"data.flaky_tests#": 0, "data.new_failed#": 0, "data.new_passed#": 0, "data.summary.total": 42,
		}},
		{name: "build not found", reportID: "99", want: http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mocks := testutil.New()
			proj, err := mocks.Projects.CreateProject(context.Background(), "stability-proj")
			if err != nil {
				t.Fatal(err)
			}
			mocks.Builds.GetBuildByNumberFn = func(_ context.Context, pid int64, n int) (store.Build, error) {
				if pid != proj.ID || n != 5 {
					return store.Build{}, store.ErrBuildNotFound
				}
				return store.Build{ID: 200, BuildNumber: 5, FlakyCount: new(2), StatTotal: new(100)}, nil
			}
			mocks.Builds.GetLatestBuildFn = func(context.Context, int64) (store.Build, error) {
				return store.Build{ID: 50, BuildNumber: 7, StatTotal: new(42)}, nil
			}
			mocks.TestResults.ListStabilityByBuildFn = func(_ context.Context, _ int64, buildID int64) ([]store.TestResult, error) {
				if buildID != 200 {
					return []store.TestResult{}, nil
				}
				return []store.TestResult{
					{TestName: "test-flaky", Status: "passed", Flaky: true, Retries: 2},
					{TestName: "test-new-fail", Status: "failed", NewFailed: true},
					{TestName: "test-new-pass", Status: "passed", NewPassed: true},
				}, nil
			}
			h := newTestReportHandlerWithMocks(t, t.TempDir(), mocks)
			id := strconv.FormatInt(proj.ID, 10)
			code, body := serveJSON(t, h.GetReportStability, http.MethodGet, "/api/v1/projects/"+id+"/reports/"+tc.reportID+"/stability", "",
				"project_id", id, "report_id", tc.reportID)
			if code != tc.want {
				t.Fatalf("status = %d, want %d: %v", code, tc.want, body)
			}
			wantJSON(t, body, tc.wantJSON)
		})
	}
}
