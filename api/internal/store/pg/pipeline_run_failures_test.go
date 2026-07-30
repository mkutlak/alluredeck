package pg_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestListRunFailures_SpansEveryBuildInTheRun covers the case the runs feed
// depends on: a suite sharded across parallel CI jobs uploads one build per
// shard under a single pipeline ID, and a second suite uploads one build under
// the same pipeline. All of their failures must come back from one call, each
// row tagged with the suite and build it belongs to, and builds from other
// pipelines must stay out.
func TestListRunFailures_SpansEveryBuildInTheRun(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	trStore := pg.NewTestResultStore(s, logger)
	pipelineStore := pg.NewPipelineStore(s)

	stamp := time.Now().UnixNano()
	parent, err := projectStore.CreateProject(ctx, fmt.Sprintf("run-failures-parent-%d", stamp))
	if err != nil {
		t.Fatalf("CreateProject parent: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), parent.ID) })

	sharded, err := projectStore.CreateProjectWithParent(ctx, fmt.Sprintf("sharded-%d", stamp), parent.ID)
	if err != nil {
		t.Fatalf("CreateProjectWithParent sharded: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), sharded.ID) })

	single, err := projectStore.CreateProjectWithParent(ctx, fmt.Sprintf("single-%d", stamp), parent.ID)
	if err != nil {
		t.Fatalf("CreateProjectWithParent single: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), single.ID) })

	const runKey = "pipeline-under-test"

	// seedBuild creates one build carrying the given pipeline ID and inserts a
	// single failing test into it. Returns the build's surrogate ID.
	seedBuild := func(projectID int64, buildNumber int, pipelineID, testName, historyID, message string) int64 {
		t.Helper()
		if err := buildStore.InsertBuild(ctx, projectID, buildNumber); err != nil {
			t.Fatalf("InsertBuild: %v", err)
		}
		if err := buildStore.UpdateBuildCIMetadata(ctx, projectID, buildNumber, store.CIMetadata{
			PipelineID: pipelineID,
			CommitSHA:  "deadbeef",
			Branch:     "master",
		}); err != nil {
			t.Fatalf("UpdateBuildCIMetadata: %v", err)
		}
		buildID, err := trStore.GetBuildID(ctx, projectID, buildNumber)
		if err != nil {
			t.Fatalf("GetBuildID: %v", err)
		}
		if err := trStore.InsertBatch(ctx, []store.TestResult{{
			BuildID: buildID, ProjectID: projectID,
			TestName: testName, FullName: testName + ".spec.js:1:1",
			Status: "failed", HistoryID: historyID, DurationMs: 1234,
		}}); err != nil {
			t.Fatalf("InsertBatch: %v", err)
		}
		// InsertBatch intentionally omits status_message — only the parser path
		// (InsertBatchFull) populates it — so set it directly here to exercise
		// the message plumbing the failure drawer relies on.
		if _, err := s.Pool().Exec(ctx,
			`UPDATE test_results SET status_message=$1 WHERE build_id=$2 AND history_id=$3`,
			message, buildID, historyID); err != nil {
			t.Fatalf("set status_message: %v", err)
		}
		return buildID
	}

	shard1 := seedBuild(sharded.ID, 1, runKey, "shard one failure", "h-shard-1", "TimeoutError: locator.click")
	shard2 := seedBuild(sharded.ID, 2, runKey, "shard two failure", "h-shard-2", "TimeoutError: locator.evaluate")
	other := seedBuild(single.ID, 1, runKey, "other suite failure", "h-other", "assertion failed")
	// A build from a different pipeline must not leak into the run.
	seedBuild(single.ID, 2, "some-other-pipeline", "unrelated failure", "h-unrelated", "nope")

	rows, err := pipelineStore.ListRunFailures(ctx, parent.ID, runKey, 100)
	if err != nil {
		t.Fatalf("ListRunFailures: %v", err)
	}

	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3 (two shards + one single-build suite)", len(rows))
	}

	byTestName := map[string]store.RunFailureRow{}
	for _, r := range rows {
		byTestName[r.TestName] = r
	}
	for _, name := range []string{"shard one failure", "shard two failure", "other suite failure"} {
		if _, ok := byTestName[name]; !ok {
			t.Errorf("missing failure %q; got %v", name, byTestName)
		}
	}
	if _, leaked := byTestName["unrelated failure"]; leaked {
		t.Error("a build from a different pipeline leaked into the run")
	}

	// Both shard builds must be distinguishable, otherwise the client cannot
	// link a failure back to the shard that produced it.
	if got := byTestName["shard one failure"].BuildID; got != shard1 {
		t.Errorf("shard one build_id = %d, want %d", got, shard1)
	}
	if got := byTestName["shard two failure"].BuildID; got != shard2 {
		t.Errorf("shard two build_id = %d, want %d", got, shard2)
	}
	if got := byTestName["other suite failure"].BuildID; got != other {
		t.Errorf("other suite build_id = %d, want %d", got, other)
	}

	// Suite identity travels with each row.
	if got := byTestName["shard one failure"].ProjectID; got != sharded.ID {
		t.Errorf("shard one project_id = %d, want %d", got, sharded.ID)
	}
	if got := byTestName["other suite failure"].ProjectID; got != single.ID {
		t.Errorf("other suite project_id = %d, want %d", got, single.ID)
	}
	if got := byTestName["shard one failure"].StatusMessage; got != "TimeoutError: locator.click" {
		t.Errorf("shard one status_message = %q, want the seeded message", got)
	}
}

func TestListRunFailures_UnknownRunKeyReturnsEmpty(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	projectStore := pg.NewProjectStore(s, zap.NewNop())
	pipelineStore := pg.NewPipelineStore(s)

	parent, err := projectStore.CreateProject(ctx, fmt.Sprintf("run-failures-empty-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), parent.ID) })

	rows, err := pipelineStore.ListRunFailures(ctx, parent.ID, "no-such-pipeline", 100)
	if err != nil {
		t.Fatalf("ListRunFailures: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("got %d rows, want 0", len(rows))
	}
}
