package handlers

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestGetProjectTimeline(t *testing.T) {
	created := time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name        string
		query       string
		want        int
		wantPerPage int // 0 skips the check
		wantBranch  int64
		wantJSON    map[string]any
	}{
		// The newest build of 15 is returned: "showing 1 of 15".
		{name: "latest build by default", want: http.StatusOK, wantJSON: map[string]any{
			"data.builds#": 1, "data.builds.0.build_number": 42, "data.builds.0.test_cases#": 2, "data.builds.0.summary.total": 2,
			"data.builds_returned": 1, "data.total_builds_in_range": 15,
		}},
		{name: "branch filter", query: "branch=main", want: http.StatusOK, wantBranch: 10},
		{name: "limit capped at 10", query: "limit=50", want: http.StatusOK, wantPerPage: 10},
		// "to" is inclusive: the range ends at the start of the next day.
		{name: "date range", query: "from=2026-03-20&to=2026-03-25&limit=3", want: http.StatusOK, wantJSON: map[string]any{
			"data.builds#": 2, "data.total_builds_in_range": 5,
		}},
		{name: "invalid date", query: "from=not-a-date", want: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mocks := testutil.New()
			mocks.Branches.GetByNameFn = func(_ context.Context, _ int64, name string) (*store.Branch, error) {
				if name == "main" {
					return &store.Branch{ID: 10, Name: name}, nil
				}
				return nil, store.ErrBranchNotFound
			}
			var gotPerPage int
			var gotBranch *int64
			mocks.Builds.ListBuildsPaginatedBranchFn = func(_ context.Context, _ int64, _, perPage int, branchID *int64) ([]store.Build, int, error) {
				gotPerPage, gotBranch = perPage, branchID
				return []store.Build{{ID: 100, BuildNumber: 42, CreatedAt: created}}, 15, nil
			}
			mocks.Builds.ListBuildsInRangeFn = func(_ context.Context, _ int64, _ *int64, from, to time.Time, _ int) ([]store.Build, int, error) {
				if !from.Equal(time.Date(2026, 3, 20, 0, 0, 0, 0, time.UTC)) || !to.Equal(time.Date(2026, 3, 26, 0, 0, 0, 0, time.UTC)) {
					t.Errorf("range = %v..%v, want 2026-03-20..2026-03-26", from, to)
				}
				return []store.Build{{ID: 100, BuildNumber: 42, CreatedAt: created}, {ID: 99, BuildNumber: 41, CreatedAt: created.Add(-24 * time.Hour)}}, 5, nil
			}
			mocks.TestResults.ListTimelineMultiFn = func(_ context.Context, _ int64, buildIDs []int64, _ int) ([]store.MultiTimelineRow, error) {
				var rows []store.MultiTimelineRow
				for _, bid := range buildIDs {
					switch bid {
					case 100:
						rows = append(rows,
							store.MultiTimelineRow{BuildID: 100, BuildNumber: 42, TestName: "Login", Status: "passed", StartMs: 170000, StopMs: 170005},
							store.MultiTimelineRow{BuildID: 100, BuildNumber: 42, TestName: "Logout", Status: "failed", StartMs: 170010, StopMs: 170020})
					case 99:
						rows = append(rows, store.MultiTimelineRow{BuildID: 99, BuildNumber: 41, TestName: "Signup", Status: "passed", StartMs: 160000, StopMs: 160010})
					}
				}
				return rows, nil
			}
			h := newTestProjectTimelineHandler(t, mocks)
			code, body := serveJSON(t, h.GetProjectTimeline, http.MethodGet, "/api/v1/projects/1/timeline?"+tc.query, "", "project_id", "1")
			if code != tc.want {
				t.Fatalf("status = %d, want %d: %v", code, tc.want, body)
			}
			wantJSON(t, body, tc.wantJSON)
			if tc.wantPerPage != 0 && gotPerPage != tc.wantPerPage {
				t.Errorf("per_page = %d, want %d", gotPerPage, tc.wantPerPage)
			}
			if tc.wantBranch != 0 && (gotBranch == nil || *gotBranch != tc.wantBranch) {
				t.Errorf("branch id = %v, want %d", gotBranch, tc.wantBranch)
			}
		})
	}
}
