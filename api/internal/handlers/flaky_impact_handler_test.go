package handlers

import (
	"context"
	"net/http"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestGetFlakyImpact(t *testing.T) {
	tests := []struct {
		name                  string
		noStore               bool // analytics unavailable: degrade to an empty list
		project               string
		query                 string
		want                  int
		wantBuilds, wantLimit int // forwarded to the store; 0 skips the check
		wantJSON              map[string]any
	}{
		{name: "defaults", project: "1", want: http.StatusOK, wantBuilds: analyticsDefaultBuilds, wantLimit: analyticsDefaultLimit,
			wantJSON: map[string]any{"data.tests#": 1, "data.tests.0.full_name": "suite.TestFoo", "data.total": 1}},
		{name: "clamped to max", project: "1", query: "builds=9999&limit=9999", want: http.StatusOK, wantBuilds: analyticsMaxBuilds, wantLimit: analyticsMaxLimit},
		{name: "non-positive falls back to defaults", project: "1", query: "builds=0&limit=-5", want: http.StatusOK, wantBuilds: analyticsDefaultBuilds, wantLimit: analyticsDefaultLimit},
		{name: "no analytics store", noStore: true, project: "1", want: http.StatusOK, wantJSON: map[string]any{"data.tests#": 0}},
		{name: "unknown project slug", project: "does-not-exist", want: http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotProject int64
			var gotBranch *int64
			var gotBuilds, gotLimit int
			var as store.AnalyticsStorer = &testutil.MockAnalyticsStore{
				ListFlakyImpactFn: func(_ context.Context, projectID int64, branchID *int64, builds, limit int) ([]store.FlakyImpact, error) {
					gotProject, gotBranch, gotBuilds, gotLimit = projectID, branchID, builds, limit
					return []store.FlakyImpact{{FullName: "suite.TestFoo", FlakyCount: 3, RetrySum: 5, WastedMs: 12000, Runs: 10}}, nil
				},
			}
			if tc.noStore {
				as = nil
			}
			mocks := testutil.New()
			h := NewFlakyImpactHandler(as, mocks.Branches, mocks.Projects, zap.NewNop())
			code, body := serveJSON(t, h.GetFlakyImpact, http.MethodGet, "/api/v1/projects/"+tc.project+"/analytics/flaky?"+tc.query, "", "project_id", tc.project)
			if code != tc.want {
				t.Fatalf("status = %d, want %d: %v", code, tc.want, body)
			}
			wantJSON(t, body, tc.wantJSON)
			if tc.wantBuilds != 0 && (gotProject != 1 || gotBranch != nil || gotBuilds != tc.wantBuilds || gotLimit != tc.wantLimit) {
				t.Errorf("store got project %d, branch %v, builds %d, limit %d; want 1, nil, %d, %d", gotProject, gotBranch, gotBuilds, gotLimit, tc.wantBuilds, tc.wantLimit)
			}
		})
	}
}
