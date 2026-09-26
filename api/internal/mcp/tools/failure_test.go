package tools_test

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestListFailingTests(t *testing.T) {
	failing := []store.TestResult{
		{ID: 501, BuildID: 42, HistoryID: "h1", FullName: "pkg.Test1", Status: "failed"},
		{ID: 502, BuildID: 42, HistoryID: "h2", FullName: "pkg.Test2", Status: "broken", Retries: 1, Flaky: true},
		{ID: 503, BuildID: 42, HistoryID: "h3", FullName: "pkg.Test3", Status: "failed"},
	}
	latest := &store.Build{ID: 42, BuildNumber: 7}
	five, two := 5, 2
	withStats := &store.Build{ID: 42, BuildNumber: 7, StatFailed: &five, StatBroken: &two}

	tests := []struct {
		name         string
		args         map[string]any
		latest, byID *store.Build // nil: ErrBuildNotFound
		rows         []store.TestResult
		distinct     int
		listErr      error
		want         tools.ListFailingTestsOutput
		wantBuild    int64 // build id forwarded to ListFailedByBuild
		wantLimit    int
		wantErr      string
	}{
		{
			// test_result_id is the row's own id (it used to be a fabricated
			// zero). The cursor is accepted and ignored: without offset
			// pagination a "next page" would only re-return page one.
			name:   "latest build rows map to items; limit forwarded, cursor ignored",
			args:   map[string]any{"project_id": 1, "limit": 2, "cursor": "not-a-real-cursor!!"},
			latest: latest, rows: failing,
			want: tools.ListFailingTestsOutput{Build: &tools.BuildRef{BuildNumber: 7}, Items: []tools.FailingTestItem{
				{TestResultID: 501, BuildID: 42, HistoryID: "h1", FullName: "pkg.Test1", Status: "failed"},
				{TestResultID: 502, BuildID: 42, HistoryID: "h2", FullName: "pkg.Test2", Status: "broken", Retries: 1, Flaky: true},
			}},
			wantBuild: 42, wantLimit: 2,
		},
		{name: "project without builds is empty, not an error", args: map[string]any{"project_id": 1}},
		{
			name: "explicit green build still hoists its build ref",
			args: map[string]any{"project_id": 1, "build_id": 164},
			byID: &store.Build{ID: 164, BuildNumber: 28, CIBranch: new("main"), CICommitSHA: new("abc123")},
			want: tools.ListFailingTestsOutput{Items: []tools.FailingTestItem{},
				Build: &tools.BuildRef{BuildNumber: 28, Branch: "main", CommitSHA: "abc123"}},
			wantBuild: 164, wantLimit: 50,
		},
		{
			name:   "summary falls back to the distinct count when the build has no stats",
			args:   map[string]any{"project_id": 1, "summary_only": true},
			latest: latest, rows: failing, distinct: 3,
			want: tools.ListFailingTestsOutput{Items: []tools.FailingTestItem{}, Build: &tools.BuildRef{BuildNumber: 7},
				Summary: &tools.FailingSummary{TotalFailed: 3, DistinctFailed: 3, Statuses: map[string]int{"failed": 2, "broken": 1}}},
			wantBuild: 42, wantLimit: 50,
		},
		{
			// total_failed counts rows the way the report does; distinct_failed
			// stays dedup-aware (a test ingested under two history_id schemes).
			name:   "summary prefers the build's stat_failed+stat_broken",
			args:   map[string]any{"project_id": 1, "summary_only": true},
			latest: withStats, rows: failing[:1], distinct: 4,
			want: tools.ListFailingTestsOutput{Items: []tools.FailingTestItem{}, Build: &tools.BuildRef{BuildNumber: 7},
				Summary: &tools.FailingSummary{TotalFailed: 7, DistinctFailed: 4, Statuses: map[string]int{"failed": 1}}},
			wantBuild: 42, wantLimit: 50,
		},
		{name: "non-positive project_id", args: map[string]any{"project_id": 0}, wantErr: "project_id must be positive"},
		{name: "store error is surfaced", args: map[string]any{"project_id": 1}, latest: latest,
			listErr: errors.New("db connection reset"), wantErr: "db connection reset"},
		{name: "build outside the project hints at resolve_url", args: map[string]any{"project_id": 1, "build_id": 28},
			wantErr: "resolve_url"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mocks := testutil.New()
			lookup := func(b *store.Build) (store.Build, error) {
				if b == nil {
					return store.Build{}, store.ErrBuildNotFound
				}
				return *b, nil
			}
			mocks.Builds.GetLatestBuildFn = func(context.Context, int64) (store.Build, error) { return lookup(tc.latest) }
			mocks.Builds.GetBuildByIDFn = func(context.Context, int64, int64) (store.Build, error) { return lookup(tc.byID) }
			var gotBuild int64
			var gotLimit int
			mocks.TestResults.ListFailedByBuildFn = func(_ context.Context, _, buildID int64, limit int) ([]store.TestResult, error) {
				gotBuild, gotLimit = buildID, limit
				return tc.rows[:min(limit, len(tc.rows))], tc.listErr
			}
			mocks.TestResults.CountFailedByBuildFn = func(context.Context, int64, int64) (int, error) { return tc.distinct, nil }
			cs := setupTestServer(t, &bootstrap.Stores{Build: mocks.Builds, TestResult: mocks.TestResults})

			if tc.wantErr != "" {
				if msg := callErr(t, cs, "list_failing_tests", tc.args); !strings.Contains(msg, tc.wantErr) {
					t.Errorf("error = %q, want it to contain %q", msg, tc.wantErr)
				}
				return
			}
			out, _ := call[tools.ListFailingTestsOutput](t, cs, "list_failing_tests", tc.args)
			if !reflect.DeepEqual(out, tc.want) {
				t.Errorf("output = %+v, want %+v", out, tc.want)
			}
			if gotBuild != tc.wantBuild || gotLimit != tc.wantLimit {
				t.Errorf("ListFailedByBuild(build=%d, limit=%d), want (%d, %d)", gotBuild, gotLimit, tc.wantBuild, tc.wantLimit)
			}
		})
	}
}
