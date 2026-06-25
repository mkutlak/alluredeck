package pg_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/parser"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// i64p returns a pointer to v, for optional start/stop millis fields.
func i64p(v int64) *int64 { return &v }

// TestInsertBatch_DuplicateHistoryID_LatestAttemptWins reproduces the
// production "duplicate key value violates unique constraint
// idx_test_results_build_history" warning (runner/allure.go).
//
// When a single build contains multiple test results sharing the same
// non-empty historyId — the Allure retry/flaky case, where every attempt is a
// separate *-result.json and parseStabilityEntries emits one stabilityEntry
// per file — InsertBatch must NOT abort the whole transaction. It must collapse
// the attempts into a single row keyed by (build_id, history_id) and keep the
// latest attempt (greatest stop_ms) as the surviving row, so a retried test
// records its final outcome.
func TestInsertBatch_DuplicateHistoryID_LatestAttemptWins(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	trStore := pg.NewTestResultStore(s, logger)

	slug := fmt.Sprintf("test-insertbatch-dup-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	if err := buildStore.InsertBuild(ctx, projectID, 1); err != nil {
		t.Fatalf("InsertBuild: %v", err)
	}
	buildID, err := trStore.GetBuildID(ctx, projectID, 1)
	if err != nil {
		t.Fatalf("GetBuildID: %v", err)
	}

	// Two attempts of the same test (same historyId). The first attempt failed;
	// the retry passed. The final outcome is "passed". The latest attempt is
	// listed FIRST here on purpose: a naive last-writer-wins (DO UPDATE with no
	// ordering guard) would leave the stale "failed" row, so this asserts the
	// implementation keeps the row with the greatest stop_ms regardless of
	// batch order.
	results := []store.TestResult{
		{
			BuildID: buildID, ProjectID: projectID,
			TestName: "printer settings", FullName: "ui.printer.settings",
			Status: "passed", HistoryID: "hist-printer", DurationMs: 50,
			Flaky: true, Retries: 1, StartMs: i64p(150), StopMs: i64p(200),
		},
		{
			BuildID: buildID, ProjectID: projectID,
			TestName: "printer settings", FullName: "ui.printer.settings",
			Status: "failed", HistoryID: "hist-printer", DurationMs: 40,
			Flaky: false, Retries: 0, StartMs: i64p(60), StopMs: i64p(100),
		},
	}

	if err := trStore.InsertBatch(ctx, results); err != nil {
		t.Fatalf("InsertBatch with duplicate historyId returned error (want nil): %v", err)
	}

	// Exactly one row must survive for (build_id, history_id).
	var count int
	if err := s.Pool().QueryRow(ctx,
		"SELECT COUNT(*) FROM test_results WHERE build_id=$1 AND history_id=$2",
		buildID, "hist-printer").Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count for (build_id, history_id): got %d, want 1", count)
	}

	// The surviving row must be the latest attempt: passed, flaky, retries=1.
	var (
		status  string
		flaky   bool
		retries int
		stopMs  *int64
	)
	if err := s.Pool().QueryRow(ctx,
		"SELECT status, flaky, retries, stop_ms FROM test_results WHERE build_id=$1 AND history_id=$2",
		buildID, "hist-printer").Scan(&status, &flaky, &retries, &stopMs); err != nil {
		t.Fatalf("scan surviving row: %v", err)
	}
	if status != "passed" {
		t.Errorf("status: got %q, want %q (latest attempt should win)", status, "passed")
	}
	if !flaky {
		t.Errorf("flaky: got false, want true (from latest attempt)")
	}
	if retries != 1 {
		t.Errorf("retries: got %d, want 1 (from latest attempt)", retries)
	}
	if stopMs == nil || *stopMs != 200 {
		t.Errorf("stop_ms: got %v, want 200 (latest attempt)", stopMs)
	}
}

// TestInsertBatch_EmptyHistoryID_AllRowsInserted verifies the partial unique
// index (WHERE history_id != ”) does NOT collapse rows with an empty
// historyId: two such results must both be inserted, and the ON CONFLICT clause
// must not erroneously swallow them.
func TestInsertBatch_EmptyHistoryID_AllRowsInserted(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	trStore := pg.NewTestResultStore(s, logger)

	slug := fmt.Sprintf("test-insertbatch-empty-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	if err := buildStore.InsertBuild(ctx, projectID, 1); err != nil {
		t.Fatalf("InsertBuild: %v", err)
	}
	buildID, err := trStore.GetBuildID(ctx, projectID, 1)
	if err != nil {
		t.Fatalf("GetBuildID: %v", err)
	}

	results := []store.TestResult{
		{BuildID: buildID, ProjectID: projectID, TestName: "a", Status: "passed", HistoryID: ""},
		{BuildID: buildID, ProjectID: projectID, TestName: "b", Status: "failed", HistoryID: ""},
	}
	if err := trStore.InsertBatch(ctx, results); err != nil {
		t.Fatalf("InsertBatch with empty historyId returned error: %v", err)
	}

	var count int
	if err := s.Pool().QueryRow(ctx,
		"SELECT COUNT(*) FROM test_results WHERE build_id=$1 AND history_id=''", buildID).Scan(&count); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("empty-historyId rows: got %d, want 2", count)
	}
}

// TestInsertBatchFull_DuplicateHistoryID_NoDuplicateChildren guards the
// enrichment path: when two retry attempts of the same test (same historyId)
// reach InsertBatchFull, they collapse onto one test_results row. Without
// de-duplication, each attempt's labels/parameters/steps/attachments would be
// re-inserted under that single surviving row id, doubling the child rows. The
// fix keeps only the latest attempt (greatest StopMs), so exactly one row and
// one set of children survive.
func TestInsertBatchFull_DuplicateHistoryID_NoDuplicateChildren(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	trStore := pg.NewTestResultStore(s, logger)

	slug := fmt.Sprintf("test-insertbatchfull-dup-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	if err := buildStore.InsertBuild(ctx, projectID, 1); err != nil {
		t.Fatalf("InsertBuild: %v", err)
	}
	buildID, err := trStore.GetBuildID(ctx, projectID, 1)
	if err != nil {
		t.Fatalf("GetBuildID: %v", err)
	}

	// Two attempts of the same test. The latest (greater StopMs) passed; each
	// carries its own label, parameter, step and attachment.
	mkResult := func(status string, stopMs int64) *parser.Result {
		return &parser.Result{
			Name: "printer settings", FullName: "ui.printer.settings",
			HistoryID: "hist-printer", Status: status, StartMs: stopMs - 40, StopMs: stopMs,
			Labels:      []parser.Label{{Name: "suite", Value: "printer"}},
			Parameters:  []parser.Parameter{{Name: "browser", Value: "chromium"}},
			Steps:       []parser.Step{{Name: "open dialog", Status: status, Order: 0}},
			Attachments: []parser.Attachment{{Name: "screenshot", Source: "shot-" + status + ".png", MimeType: "image/png"}},
		}
	}
	results := []*parser.Result{
		mkResult("failed", 100),
		mkResult("passed", 200),
	}

	if err := trStore.InsertBatchFull(ctx, buildID, projectID, results); err != nil {
		t.Fatalf("InsertBatchFull with duplicate historyId returned error: %v", err)
	}

	// Exactly one test_results row, and exactly one of each child kind.
	var resultID int64
	var status string
	if err := s.Pool().QueryRow(ctx,
		"SELECT id, status FROM test_results WHERE build_id=$1 AND history_id=$2",
		buildID, "hist-printer").Scan(&resultID, &status); err != nil {
		t.Fatalf("expected exactly one surviving row: %v", err)
	}

	for _, c := range []struct {
		table string
		want  int
	}{
		{"test_labels", 1},
		{"test_parameters", 1},
		{"test_steps", 1},
		{"test_attachments", 1},
	} {
		var n int
		if err := s.Pool().QueryRow(ctx,
			"SELECT COUNT(*) FROM "+c.table+" WHERE test_result_id=$1", resultID).Scan(&n); err != nil {
			t.Fatalf("count %s: %v", c.table, err)
		}
		if n != c.want {
			t.Errorf("%s rows: got %d, want %d (duplicate enrichment children)", c.table, n, c.want)
		}
	}
}
