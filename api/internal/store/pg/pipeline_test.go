package pg_test

import (
	"slices"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// ciBuild inserts build order into projectID under a CI pipeline id.
func (f *fixture) ciBuild(projectID int64, order int, pipelineID string) int64 {
	f.t.Helper()
	id := f.buildIn(projectID, order)
	if err := f.builds.UpdateBuildCIMetadata(f.ctx, projectID, order, store.CIMetadata{
		PipelineID: pipelineID, CommitSHA: "deadbeef", Branch: "master",
	}); err != nil {
		f.t.Fatalf("UpdateBuildCIMetadata %d: %v", order, err)
	}
	return id
}

// TestListRunFailures_SpansEveryBuildInTheRun: a suite sharded across CI jobs
// uploads one build per shard under one pipeline id, and a second suite one
// build under the same id. One call returns every failure of the run, each
// row tagged with its suite, build and message; other pipelines stay out.
func TestListRunFailures_SpansEveryBuildInTheRun(t *testing.T) {
	f := newFixture(t) // the parent
	sharded, single := f.newProject(f.id), f.newProject(f.id)
	const runKey = "pipeline-under-test"
	seed := func(projectID int64, order int, pipelineID, testName, message string) store.RunFailureRow {
		buildID := f.ciBuild(projectID, order, pipelineID)
		r := f.result(buildID, testName, "failed", testName)
		r.ProjectID, r.FullName, r.DurationMs = projectID, testName+".spec.js:1:1", 1234
		f.insert(r)
		// InsertBatch omits status_message; only InsertBatchFull writes it.
		f.exec("UPDATE test_results SET status_message=$1 WHERE build_id=$2", message, buildID)
		return store.RunFailureRow{ProjectID: projectID, BuildID: buildID, TestName: testName, StatusMessage: message}
	}
	want := map[string]store.RunFailureRow{}
	for _, w := range []store.RunFailureRow{
		seed(sharded.ID, 1, runKey, "shard one failure", "TimeoutError: locator.click"),
		seed(sharded.ID, 2, runKey, "shard two failure", "TimeoutError: locator.evaluate"),
		seed(single.ID, 1, runKey, "other suite failure", "assertion failed"),
	} {
		want[w.TestName] = w
	}
	seed(single.ID, 2, "some-other-pipeline", "unrelated failure", "nope")

	ps := pg.NewPipelineStore(f.s)
	rows, err := ps.ListRunFailures(f.ctx, f.id, runKey, 100)
	if err != nil {
		t.Fatalf("ListRunFailures: %v", err)
	}
	if len(rows) != len(want) {
		t.Errorf("got %d rows, want %d (two shards + one single-build suite): %+v", len(rows), len(want), rows)
	}
	for _, r := range rows {
		w, ok := want[r.TestName]
		if !ok || r.ProjectID != w.ProjectID || r.BuildID != w.BuildID || r.StatusMessage != w.StatusMessage {
			t.Errorf("row %+v, want %+v", r, w)
		}
	}

	if rows, err := ps.ListRunFailures(f.ctx, f.id, "no-such-pipeline", 100); err != nil || len(rows) != 0 {
		t.Errorf("unknown run key = %+v, %v; want no rows", rows, err)
	}
}

// TestListBuildsByPipelineID pins the truncation protocol: builds come back in
// build_order ASC, up to limit+1 so the caller can tell "exactly limit" from
// "more than limit", and a non-positive limit (which PostgreSQL would reject
// or read as "no rows") means the store default. A neighbouring build of
// another pipeline is never returned.
func TestListBuildsByPipelineID(t *testing.T) {
	f := newFixture(t)
	pipelineID := unique("pipeline")
	for order := 1; order <= 6; order++ {
		f.ciBuild(f.id, order, pipelineID)
	}
	f.ciBuild(f.id, 7, pipelineID+"-other")
	ps := pg.NewPipelineStore(f.s)

	all := []int{1, 2, 3, 4, 5, 6}
	for _, tt := range []struct {
		limit int
		want  []int
	}{{3, []int{1, 2, 3, 4}}, {50, all}, {0, all}, {-1, all}} {
		builds, err := ps.ListBuildsByPipelineID(f.ctx, f.id, pipelineID, tt.limit)
		if err != nil {
			t.Fatalf("limit=%d: %v", tt.limit, err)
		}
		var got []int
		for _, b := range builds {
			got = append(got, b.BuildNumber)
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("limit=%d: build numbers = %v, want %v", tt.limit, got, tt.want)
		}
	}
}
