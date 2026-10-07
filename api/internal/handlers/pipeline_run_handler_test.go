package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestGetPipelineRun drives the single-run endpoint over a parent group (id 1)
// whose store double returns the rows of one run. The rows carry the group
// fields the way the real query does, so groupPipelineRuns builds the same
// response the feed shows for that run.
func TestGetPipelineRun(t *testing.T) {
	// run is one build of pipeline 196765 in group 1.
	run := func(projectID int64, slug string, number int, age time.Duration, passed, failed, skipped int) store.PipelineRunRow {
		return pipelineRow(store.PipelineRunRow{
			PipelineID: "196765", PipelineURL: "https://ci/pipelines/196765", CommitSHA: "6fb9dec", Branch: "feature/login",
			CIBuildURL: "https://ci/jobs/9", CreatedAt: pipelineTS.Add(-age), ProjectID: projectID, Slug: slug,
			BuildNumber: number, BuildID: projectID*1000 + int64(number), GroupProjectID: 1, GroupSlug: "parent",
		}, passed, failed, skipped, passed+failed+skipped, 1000)
	}
	tests := []struct {
		name     string
		project  string
		runKey   string
		rows     []store.PipelineRunRow
		storeErr error
		want     int
		wantCall string // "" skips the check
		wantJSON map[string]any
	}{
		{name: "found", project: "1", runKey: "196765", want: http.StatusOK, wantCall: "group=1 run=196765",
			rows: []store.PipelineRunRow{run(2, "ui-users", 5, 0, 40, 2, 0), run(3, "api", 3, time.Second, 100, 0, 0)},
			wantJSON: map[string]any{
				"metadata.message": "pipeline run retrieved",
				"data.pipeline_id": "196765", "data.pipeline_url": "https://ci/pipelines/196765", "data.commit_sha": "6fb9dec",
				"data.branch": "feature/login", "data.ci_build_url": "https://ci/jobs/9", "data.timestamp": "2026-03-25T10:00:00Z",
				"data.group_project_id": 1, "data.group_slug": "parent",
				"data.suites#": 2, "data.suites.0.slug": "ui-users", "data.suites.0.status": "degraded", "data.suites.0.failed": 2,
				"data.suites.1.slug": "api", "data.suites.1.status": "passed",
				"data.aggregate.suites_total": 2, "data.aggregate.suites_passed": 1, "data.aggregate.tests_total": 142,
				"data.aggregate.tests_passed": 140, "data.aggregate.pass_rate": 98.5,
			}},
		// The slug of the group resolves like it does for every other project route.
		{name: "group by slug", project: "parent", runKey: "196765", want: http.StatusOK, wantCall: "group=1 run=196765",
			rows: []store.PipelineRunRow{run(2, "ui-users", 5, 0, 1, 0, 0)}, wantJSON: map[string]any{"data.suites#": 1}},
		// A commit SHA is a run key too; it reaches the store untouched.
		{name: "sha key", project: "1", runKey: "6fb9dec", want: http.StatusOK, wantCall: "group=1 run=6fb9dec",
			rows: []store.PipelineRunRow{run(2, "ui-users", 5, 0, 1, 0, 0)}},
		// CI shards a suite across parallel jobs and each uploads its own build
		// under the one pipeline id: they are one suite, listed oldest-first.
		{name: "sharded suite is one suite", project: "1", runKey: "196765", want: http.StatusOK,
			rows: []store.PipelineRunRow{
				run(2, "ui-users", 656, 0, 22, 2, 0), run(2, "ui-users", 655, time.Second, 22, 2, 0), run(2, "ui-users", 654, 2*time.Minute, 21, 0, 3),
			},
			wantJSON: map[string]any{
				"data.suites#": 1, "data.suites.0.total": 72, "data.suites.0.failed": 4, "data.suites.0.skipped": 3,
				"data.suites.0.build_number": 656, "data.suites.0.builds#": 3,
				"data.suites.0.builds.0.build_number": 654, "data.suites.0.builds.2.build_number": 656,
				"data.aggregate.suites_total": 1, "data.aggregate.tests_total": 72, "data.aggregate.tests_passed": 65,
			}},
		// Nothing ran: the same "skipped" verdict and zero rate the feed gives,
		// not a failed run.
		{name: "all skipped", project: "1", runKey: "196765", want: http.StatusOK,
			rows: []store.PipelineRunRow{run(2, "ui-users", 5, 0, 0, 0, 7)},
			wantJSON: map[string]any{
				"data.suites.0.status": "skipped", "data.suites.0.pass_rate": 0, "data.suites.0.skipped": 7,
				"data.aggregate.suites_passed": 0, "data.aggregate.pass_rate": 0, "data.aggregate.tests_skipped": 7, "data.aggregate.tests_total": 7,
			}},
		{name: "unknown run", project: "1", runKey: "nope", want: http.StatusNotFound, wantCall: "group=1 run=nope",
			wantJSON: map[string]any{"metadata.message": "pipeline run not found"}},
		{name: "empty run key", project: "1", runKey: "", want: http.StatusBadRequest, wantCall: "-",
			wantJSON: map[string]any{"metadata.message": "invalid run_key"}},
		{name: "store error", project: "1", runKey: "196765", storeErr: errors.New("connection refused"), want: http.StatusInternalServerError,
			wantJSON: map[string]any{"metadata.message": "error getting pipeline run"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			projects := testutil.NewMemProjectStore()
			if _, err := projects.CreateProject(ctx, "parent"); err != nil {
				t.Fatal(err)
			}
			call := "-"
			ps := &testutil.MockPipelineStore{
				GetPipelineRunFn: func(_ context.Context, groupID int64, runKey string) ([]store.PipelineRunRow, error) {
					call = fmt.Sprintf("group=%d run=%s", groupID, runKey)
					return tc.rows, tc.storeErr
				},
			}
			h := NewPipelineHandler(ps, projects, testutil.NewMemKnownIssueStore(), t.TempDir(), zap.NewNop())
			code, body := serveJSON(t, h.GetPipelineRun, http.MethodGet, "/api/v1/projects/1/pipeline-runs/x", "", "project_id", tc.project, "run_key", tc.runKey)
			if code != tc.want {
				t.Fatalf("status = %d, want %d: %v", code, tc.want, body)
			}
			if tc.wantCall != "" && call != tc.wantCall {
				t.Errorf("store call = %q, want %q", call, tc.wantCall)
			}
			wantJSON(t, body, tc.wantJSON)
			// The body is one run, not a one-element list.
			if code == http.StatusOK {
				if _, isList := jsonAt(body, "data").([]any); isList {
					t.Errorf("data is an array, want a single run object")
				}
			}
		})
	}
}
