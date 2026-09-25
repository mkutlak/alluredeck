package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// pipelineRow sets the stat counters of a pipeline store row.
func pipelineRow(r store.PipelineRunRow, passed, failed, skipped, total int, durationMs int64) store.PipelineRunRow {
	r.StatPassed, r.StatFailed, r.StatBroken, r.StatSkipped, r.StatTotal, r.DurationMs = &passed, &failed, new(0), &skipped, &total, &durationMs
	return r
}

// Fixture rows shared by the grouping and HTTP tests: a run of two suites on
// commit abc1234 and a one-suite run on def5678, and one commit under two
// different parent groups.
var (
	pipelineTS         = time.Date(2026, 3, 25, 10, 0, 0, 0, time.UTC)
	pipelineTwoCommits = []store.PipelineRunRow{
		pipelineRow(store.PipelineRunRow{CommitSHA: "abc1234", CIBuildURL: "https://ci/1", CreatedAt: pipelineTS, ProjectID: 10, Slug: "child-a", BuildNumber: 5, BuildID: 501}, 40, 2, 0, 42, 15000),
		pipelineRow(store.PipelineRunRow{CommitSHA: "abc1234", CreatedAt: pipelineTS.Add(-time.Second), ProjectID: 11, Slug: "child-b", BuildNumber: 3, BuildID: 301}, 100, 0, 0, 100, 30000),
		pipelineRow(store.PipelineRunRow{CommitSHA: "def5678", CIBuildURL: "https://ci/2", CreatedAt: pipelineTS.Add(-time.Hour), ProjectID: 10, Slug: "child-a", BuildNumber: 4, BuildID: 401}, 42, 0, 0, 42, 14000),
	}
	pipelineTwoGroups = []store.PipelineRunRow{
		pipelineRow(store.PipelineRunRow{CommitSHA: "sameSHA", CreatedAt: pipelineTS, ProjectID: 10, Slug: "child-a", BuildNumber: 5, BuildID: 501, GroupProjectID: 100, GroupSlug: "group-a"}, 10, 0, 0, 10, 1000),
		pipelineRow(store.PipelineRunRow{CommitSHA: "sameSHA", CreatedAt: pipelineTS.Add(-time.Minute), ProjectID: 20, Slug: "child-b", BuildNumber: 3, BuildID: 301, GroupProjectID: 200, GroupSlug: "group-b"}, 20, 0, 0, 20, 2000),
	}
)

func TestGroupPipelineRuns(t *testing.T) {
	// shard is one uploaded build of the sharded ui-users suite in pipeline 196765.
	shard := func(number int, buildID int64, passed, failed, skipped int, dur int64, age time.Duration) store.PipelineRunRow {
		return pipelineRow(store.PipelineRunRow{PipelineID: "196765", CommitSHA: "6fb9dec", CreatedAt: pipelineTS.Add(-age),
			ProjectID: 30, Slug: "ui-users", DisplayName: "UI Users", BuildNumber: number, BuildID: buildID}, passed, failed, skipped, 24, dur)
	}
	suite := func(projectID int64, slug string, number int, passed, failed int) store.PipelineRunRow {
		return pipelineRow(store.PipelineRunRow{PipelineID: "p1", CommitSHA: "sha", CreatedAt: pipelineTS, ProjectID: projectID,
			Slug: slug, BuildNumber: number, BuildID: int64(number)}, passed, failed, 0, 10, 100)
	}
	tests := []struct {
		name string
		rows []store.PipelineRunRow
		want map[string]any
	}{
		{name: "groups rows by commit", rows: pipelineTwoCommits, want: map[string]any{
			"#":            2,
			"0.commit_sha": "abc1234", "0.ci_build_url": "https://ci/1", "0.suites#": 2,
			"0.aggregate.suites_total": 2, "0.aggregate.tests_total": 142,
			"0.suites.0.project_id": 10, "0.suites.0.build_id": 501,
			"1.commit_sha": "def5678", "1.suites#": 1, "1.suites.0.status": "passed",
		}},
		// CI shards a suite across parallel jobs and every shard uploads its own
		// build under the same pipeline ID. They are one logical suite: counters
		// sum, the link points at the newest shard, every shard stays listed
		// oldest-first, and skipped tests are excluded from the pass rate and
		// from tests_passed (65 of 72-3 = 94.2%).
		{name: "merges shard builds into one suite", rows: []store.PipelineRunRow{
			shard(656, 17465, 22, 2, 0, 3000, 0),
			shard(655, 17464, 22, 2, 0, 2000, time.Second),
			shard(654, 17463, 21, 0, 3, 1000, 2*time.Minute),
		}, want: map[string]any{
			"#": 1, "0.suites#": 1,
			"0.suites.0.total": 72, "0.suites.0.failed": 4, "0.suites.0.duration_ms": 6000, "0.suites.0.display_name": "UI Users",
			"0.suites.0.pass_rate": 94.2, "0.suites.0.status": "degraded",
			"0.suites.0.build_number": 656, "0.suites.0.build_id": 17465,
			"0.suites.0.builds#": 3, "0.suites.0.builds.0.build_number": 654, "0.suites.0.builds.1.build_number": 655, "0.suites.0.builds.2.build_number": 656,
			"0.aggregate.suites_total": 1, "0.aggregate.suites_passed": 0, "0.aggregate.tests_total": 72, "0.aggregate.tests_passed": 65,
		}},
		// A merged suite passes only when every one of its shards passed.
		{name: "suite passes only when every shard passes", rows: []store.PipelineRunRow{
			suite(40, "clean", 1, 10, 0), suite(40, "clean", 2, 10, 0), suite(41, "mixed", 1, 10, 0), suite(41, "mixed", 2, 9, 1),
		}, want: map[string]any{
			"#": 1, "0.aggregate.suites_total": 2, "0.aggregate.suites_passed": 1,
			"0.suites.0.slug": "clean", "0.suites.0.status": "passed", "0.suites.1.slug": "mixed", "0.suites.1.status": "degraded",
		}},
		// The same commit under two parent groups stays two runs.
		{name: "same commit in two groups", rows: pipelineTwoGroups, want: map[string]any{
			"#":                  2,
			"0.group_project_id": 100, "0.group_slug": "group-a", "0.suites#": 1, "0.suites.0.build_id": 501,
			"1.group_project_id": 200, "1.group_slug": "group-b", "1.suites#": 1, "1.suites.0.build_id": 301,
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := json.Marshal(groupPipelineRuns(tc.rows))
			if err != nil {
				t.Fatal(err)
			}
			var runs any
			if err := json.Unmarshal(raw, &runs); err != nil {
				t.Fatal(err)
			}
			wantJSON(t, runs, tc.want)
		})
	}
}

// TestPipelineHandler drives the three pipeline endpoints over a parent with
// two children (ui-users has the active known issue "flaky login") and a
// standalone project. call records the store arguments the handler forwarded.
func TestPipelineHandler(t *testing.T) {
	getRuns := (*PipelineHandler).GetPipelineRuns
	getAll := (*PipelineHandler).GetAllPipelineRuns
	getFailures := (*PipelineHandler).GetRunFailures
	failures := []store.RunFailureRow{
		{ProjectID: 2, Slug: "ui-users", BuildID: 17465, BuildNumber: 656, TestName: "flaky login", Status: "failed", Retries: 3,
			StatusMessage: "TimeoutError: locator.click\n  at foo.ts:12"},
		{ProjectID: 2, Slug: "ui-users", BuildID: 17464, BuildNumber: 655, TestName: "add member", Status: "broken", NewFailed: true},
	}
	tests := []struct {
		name        string
		serve       func(*PipelineHandler, http.ResponseWriter, *http.Request)
		project     string // project_id path value; "" for the cross-project endpoint
		runKey      string
		query       string
		runs        []store.PipelineRunRow
		total       int
		fillToLimit bool // the failures store returns as many rows as asked for
		want        int
		wantCall    string // "" skips the check
		wantJSON    map[string]any
	}{
		{name: "runs grouped", serve: getRuns, project: "1", runs: pipelineTwoCommits, total: 2, want: http.StatusOK,
			wantJSON: map[string]any{"data#": 2, "pagination.total": 2}},
		{name: "runs empty", serve: getRuns, project: "1", want: http.StatusOK, wantCall: "project=1 branch= page=1 per_page=10",
			wantJSON: map[string]any{"data#": 0}},
		{name: "runs branch", serve: getRuns, project: "1", query: "branch=develop", want: http.StatusOK, wantCall: "project=1 branch=develop page=1 per_page=10"},
		{name: "runs pagination", serve: getRuns, project: "1", query: "page=2&per_page=10", total: 25, want: http.StatusOK,
			wantJSON: map[string]any{"pagination.page": 2, "pagination.per_page": 10, "pagination.total": 25, "pagination.total_pages": 3}},
		{name: "runs of a standalone project", serve: getRuns, project: "4", want: http.StatusBadRequest},
		{name: "all runs grouped", serve: getAll, runs: pipelineTwoGroups, total: 2, want: http.StatusOK, wantJSON: map[string]any{"data#": 2}},
		{name: "all runs empty", serve: getAll, want: http.StatusOK, wantCall: "groups=[] branch= page=1 per_page=10",
			wantJSON: map[string]any{"data#": 0, "pagination.total": 0}},
		{name: "all runs filters", serve: getAll, query: "branch=develop&group_id=1&group_id=2", want: http.StatusOK, wantCall: "groups=[1 2] branch=develop page=1 per_page=10"},
		{name: "all runs invalid group_id", serve: getAll, query: "group_id=abc", want: http.StatusBadRequest},
		// One extra row is requested so a full page can be reported as
		// truncated; only an error's first line is sent.
		{name: "failures tagged", serve: getFailures, project: "1", runKey: "196765", want: http.StatusOK,
			wantCall: fmt.Sprintf("group=1 run=196765 limit=%d", defaultRunFailuresLimit+1), wantJSON: map[string]any{
				"data#": 2, "metadata.truncated": false,
				"data.0.slug": "ui-users", "data.0.build_number": 656, "data.0.known": true, "data.0.error_message": "TimeoutError: locator.click",
				"data.1.known": false, "data.1.new_failed": true,
			}},
		{name: "failures truncated at limit", serve: getFailures, project: "1", runKey: "196765", query: "limit=3", fillToLimit: true, want: http.StatusOK,
			wantJSON: map[string]any{"data#": 3, "metadata.truncated": true}},
		{name: "failures invalid limit", serve: getFailures, project: "1", runKey: "196765", query: "limit=-1", want: http.StatusBadRequest},
		{name: "failures empty run key", serve: getFailures, project: "1", want: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			projects := testutil.NewMemProjectStore()
			parent, _ := projects.CreateProject(ctx, "parent")
			child, _ := projects.CreateProjectWithParent(ctx, "ui-users", parent.ID)
			_, _ = projects.CreateProjectWithParent(ctx, "child-b", parent.ID)
			_, _ = projects.CreateProject(ctx, "standalone")
			known := testutil.NewMemKnownIssueStore()
			if _, err := known.Create(ctx, child.ID, "flaky login", "", "", ""); err != nil {
				t.Fatal(err)
			}
			var call string
			ps := &testutil.MockPipelineStore{
				ListPipelineRunsFn: func(_ context.Context, projectID int64, branch string, page, perPage int) ([]store.PipelineRunRow, int, error) {
					call = fmt.Sprintf("project=%d branch=%s page=%d per_page=%d", projectID, branch, page, perPage)
					return tc.runs, tc.total, nil
				},
				ListAllPipelineRunsFn: func(_ context.Context, branch string, groupIDs []int64, page, perPage int) ([]store.PipelineRunRow, int, error) {
					call = fmt.Sprintf("groups=%v branch=%s page=%d per_page=%d", groupIDs, branch, page, perPage)
					return tc.runs, tc.total, nil
				},
				ListRunFailuresFn: func(_ context.Context, groupID int64, runKey string, limit int) ([]store.RunFailureRow, error) {
					call = fmt.Sprintf("group=%d run=%s limit=%d", groupID, runKey, limit)
					if !tc.fillToLimit {
						return failures, nil
					}
					rows := make([]store.RunFailureRow, limit)
					for i := range rows {
						rows[i] = store.RunFailureRow{ProjectID: child.ID, Slug: "ui-users", TestName: "t" + strconv.Itoa(i)}
					}
					return rows, nil
				},
			}
			h := NewPipelineHandler(ps, projects, known, t.TempDir(), zap.NewNop())
			fn := func(w http.ResponseWriter, r *http.Request) { tc.serve(h, w, r) }
			code, body := serveJSON(t, fn, http.MethodGet, "/api/v1/pipeline-runs?"+tc.query, "", "project_id", tc.project, "run_key", tc.runKey)
			if code != tc.want {
				t.Fatalf("status = %d, want %d: %v", code, tc.want, body)
			}
			if tc.wantCall != "" && call != tc.wantCall {
				t.Errorf("store call = %q, want %q", call, tc.wantCall)
			}
			wantJSON(t, body, tc.wantJSON)
		})
	}
}
