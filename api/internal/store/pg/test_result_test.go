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

// TestInsertBatchFull_PersistsFlakyAndRetries verifies InsertBatchFull writes
// the Flaky/Retries fields from parser.Result (the Playwright enrichment path,
// which — unlike Allure's InsertBatch — previously had no column mapping for
// them at all) and that a re-upsert (ON CONFLICT) updates them in place.
func TestInsertBatchFull_PersistsFlakyAndRetries(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	trStore := pg.NewTestResultStore(s, logger)

	slug := fmt.Sprintf("test-insertbatchfull-flaky-%d", time.Now().UnixNano())
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

	result := &parser.Result{
		Name: "flaky pw test", FullName: "spec/flaky.ts > flaky pw test",
		HistoryID: "hist-flaky-pw", Status: "passed",
		StartMs: 100, StopMs: 200, Flaky: true, Retries: 2,
	}

	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{result}); err != nil {
		t.Fatalf("InsertBatchFull: %v", err)
	}

	var flaky bool
	var retries int
	if err := s.Pool().QueryRow(ctx,
		"SELECT flaky, retries FROM test_results WHERE build_id=$1 AND history_id=$2",
		buildID, "hist-flaky-pw").Scan(&flaky, &retries); err != nil {
		t.Fatalf("scan flaky/retries: %v", err)
	}
	if !flaky {
		t.Error("flaky: got false, want true")
	}
	if retries != 2 {
		t.Errorf("retries: got %d, want 2", retries)
	}

	// Re-upsert must MERGE, not clobber: a later pass carrying flaky=false /
	// retries=0 (exactly what the Allure parser produces on parser.Result during
	// the enrichment pass) must NOT wipe a flaky flag an earlier pass recorded.
	// flaky is OR-merged; retries takes the max.
	result.Flaky = false
	result.Retries = 0
	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{result}); err != nil {
		t.Fatalf("InsertBatchFull (re-upsert): %v", err)
	}
	if err := s.Pool().QueryRow(ctx,
		"SELECT flaky, retries FROM test_results WHERE build_id=$1 AND history_id=$2",
		buildID, "hist-flaky-pw").Scan(&flaky, &retries); err != nil {
		t.Fatalf("scan flaky/retries (post re-upsert): %v", err)
	}
	if !flaky {
		t.Error("flaky: got false, want true preserved after a clobbering re-upsert")
	}
	if retries != 2 {
		t.Errorf("retries: got %d, want 2 preserved after re-upsert", retries)
	}
}

// TestInsertBatch_ThenInsertBatchFull_PreservesAllureFlaky reproduces the real
// Allure ingest sequence: InsertBatch records the authoritative flaky/retries
// from stability entries, then InsertBatchFull runs for enrichment with
// parser.Result rows whose Flaky/Retries are zero (the Allure parser never sets
// them). The enrichment UPSERT must NOT clobber the flaky flag back to false.
func TestInsertBatch_ThenInsertBatchFull_PreservesAllureFlaky(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	trStore := pg.NewTestResultStore(s, logger)

	slug := fmt.Sprintf("test-allure-flaky-preserve-%d", time.Now().UnixNano())
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

	// 1. InsertBatch: the authoritative flaky/retries from Allure stability data.
	if err := trStore.InsertBatch(ctx, []store.TestResult{{
		BuildID: buildID, ProjectID: projectID,
		TestName: "flaky allure test", FullName: "suite > flaky allure test",
		HistoryID: "hist-allure-flaky", Status: "passed", DurationMs: 100,
		Flaky: true, Retries: 3, StartMs: i64p(10), StopMs: i64p(110),
	}}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	// 2. InsertBatchFull enrichment: the Allure parser leaves Flaky/Retries zero.
	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{{
		Name: "flaky allure test", FullName: "suite > flaky allure test",
		HistoryID: "hist-allure-flaky", Status: "passed",
		StartMs: 10, StopMs: 110, // Flaky/Retries deliberately zero, as Allure yields
	}}); err != nil {
		t.Fatalf("InsertBatchFull: %v", err)
	}

	var flaky bool
	var retries int
	if err := s.Pool().QueryRow(ctx,
		"SELECT flaky, retries FROM test_results WHERE build_id=$1 AND history_id=$2",
		buildID, "hist-allure-flaky").Scan(&flaky, &retries); err != nil {
		t.Fatalf("scan flaky/retries: %v", err)
	}
	if !flaky {
		t.Error("flaky: got false, want true — InsertBatchFull clobbered the Allure flaky flag")
	}
	if retries != 3 {
		t.Errorf("retries: got %d, want 3 — InsertBatchFull clobbered the Allure retries", retries)
	}
}

// TestGetTestHistory_ReturnsFlakyAndRetries verifies GetTestHistory surfaces
// the per-run Flaky/Retries fields alongside status and duration.
func TestGetTestHistory_ReturnsFlakyAndRetries(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	trStore := pg.NewTestResultStore(s, logger)

	slug := fmt.Sprintf("test-history-flaky-%d", time.Now().UnixNano())
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

	const historyID = "hist-flaky-history"
	results := []store.TestResult{
		{
			BuildID: buildID, ProjectID: projectID,
			TestName: "flaky history test", FullName: "spec/history.ts > flaky history test",
			Status: "passed", HistoryID: historyID, DurationMs: 500,
			Flaky: true, Retries: 3,
		},
	}
	if err := trStore.InsertBatch(ctx, results); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	entries, err := trStore.GetTestHistory(ctx, projectID, historyID, nil, 10)
	if err != nil {
		t.Fatalf("GetTestHistory: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("entries count: got %d, want 1", len(entries))
	}
	if !entries[0].Flaky {
		t.Error("entries[0].Flaky: got false, want true")
	}
	if entries[0].Retries != 3 {
		t.Errorf("entries[0].Retries: got %d, want 3", entries[0].Retries)
	}
}

// TestGetLastPassingBuild_ReturnsMostRecentPriorPass exercises the new
// GetLastPassingBuild query against a real Postgres. It verifies: (1) it returns
// the most recent build STRICTLY before beforeBuildOrder where the test passed;
// (2) the beforeBuildOrder bound is exclusive; (3) (nil, nil) is returned when
// the test never passed; and (4) commit_sha is surfaced from the build row.
func TestGetLastPassingBuild_ReturnsMostRecentPriorPass(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	trStore := pg.NewTestResultStore(s, logger)

	slug := fmt.Sprintf("test-lastgood-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	const historyID = "hLG"

	// Build a per-test history: order 1 passed, 2 failed, 3 passed, 4 failed
	// (current). Builds inserted in ascending order so builds.id (a monotonic
	// IDENTITY) tracks build_order.
	statuses := []struct {
		order  int
		status string
	}{
		{1, "passed"},
		{2, "failed"},
		{3, "passed"},
		{4, "failed"},
	}
	buildIDByOrder := make(map[int]int64, len(statuses))
	for _, st := range statuses {
		if err := buildStore.InsertBuild(ctx, projectID, st.order); err != nil {
			t.Fatalf("InsertBuild %d: %v", st.order, err)
		}
		bid, err := trStore.GetBuildID(ctx, projectID, st.order)
		if err != nil {
			t.Fatalf("GetBuildID %d: %v", st.order, err)
		}
		buildIDByOrder[st.order] = bid
		if err := trStore.InsertBatch(ctx, []store.TestResult{{
			BuildID: bid, ProjectID: projectID,
			TestName: "lg test", FullName: "suite > lg test",
			HistoryID: historyID, Status: st.status, DurationMs: 100,
		}}); err != nil {
			t.Fatalf("InsertBatch order %d: %v", st.order, err)
		}
	}
	// Attach a commit SHA to the last-good build (order 3) to confirm it surfaces.
	if err := buildStore.UpdateBuildCIMetadata(ctx, projectID, 3, store.CIMetadata{CommitSHA: "sha-order-3"}); err != nil {
		t.Fatalf("UpdateBuildCIMetadata: %v", err)
	}

	// 1. Before the current build (order 4): most recent prior pass is order 3.
	got, err := trStore.GetLastPassingBuild(ctx, projectID, historyID, nil, 4)
	if err != nil {
		t.Fatalf("GetLastPassingBuild(before order 4): %v", err)
	}
	if got == nil {
		t.Fatal("want a last-good build before order 4, got nil")
	}
	if got.BuildNumber != 3 {
		t.Errorf("build_number: got %d, want 3 (most recent prior pass)", got.BuildNumber)
	}
	if got.BuildID != buildIDByOrder[3] {
		t.Errorf("build_id: got %d, want %d", got.BuildID, buildIDByOrder[3])
	}
	if got.Status != "passed" {
		t.Errorf("status: got %q, want passed", got.Status)
	}
	if got.CICommitSHA == nil || *got.CICommitSHA != "sha-order-3" {
		t.Errorf("ci_commit_sha: got %v, want sha-order-3", got.CICommitSHA)
	}

	// 2. beforeBuildOrder is exclusive: before order 3 (the pass itself), the
	//    query must skip order 3 and return the earlier pass at order 1.
	got, err = trStore.GetLastPassingBuild(ctx, projectID, historyID, nil, 3)
	if err != nil {
		t.Fatalf("GetLastPassingBuild(before order 3): %v", err)
	}
	if got == nil {
		t.Fatal("want a last-good build before order 3, got nil")
	}
	if got.BuildNumber != 1 {
		t.Errorf("build_number: got %d, want 1 (exclusive bound skips order 3)", got.BuildNumber)
	}

	// 3. Before order 1 there is no prior pass → (nil, nil).
	got, err = trStore.GetLastPassingBuild(ctx, projectID, historyID, nil, 1)
	if err != nil {
		t.Fatalf("GetLastPassingBuild(before order 1): %v", err)
	}
	if got != nil {
		t.Errorf("want nil when no prior passing build exists, got %+v", got)
	}

	// 4. A test that never passed → (nil, nil).
	if err := trStore.InsertBatch(ctx, []store.TestResult{{
		BuildID: buildIDByOrder[4], ProjectID: projectID,
		TestName: "never", FullName: "suite > never", HistoryID: "hNever",
		Status: "failed", DurationMs: 50,
	}}); err != nil {
		t.Fatalf("InsertBatch never: %v", err)
	}
	got, err = trStore.GetLastPassingBuild(ctx, projectID, "hNever", nil, 4)
	if err != nil {
		t.Fatalf("GetLastPassingBuild(hNever): %v", err)
	}
	if got != nil {
		t.Errorf("want nil for a test that never passed, got %+v", got)
	}
}

// TestGetLastPassingBuild_EmptyHistoryIDGuard verifies that an empty historyID
// short-circuits to (nil, nil) instead of matching the many unrelated
// test_results rows that share the empty history_id (tests without a stable
// identity). Without the guard, `tr.history_id=”` would match any such row
// with status='passed' below the bound, yielding a spurious last-good result
// for a test that isn't even the one being diagnosed.
func TestGetLastPassingBuild_EmptyHistoryIDGuard(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	trStore := pg.NewTestResultStore(s, logger)

	slug := fmt.Sprintf("test-lastgood-emptyhid-%d", time.Now().UnixNano())
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
	// A passed row with an empty history_id — unrelated to any specific test's
	// identity, but exactly the kind of row an unguarded query would match.
	if err := trStore.InsertBatch(ctx, []store.TestResult{{
		BuildID: buildID, ProjectID: projectID,
		TestName: "unrelated", FullName: "suite > unrelated",
		HistoryID: "", Status: "passed", DurationMs: 10,
	}}); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	got, err := trStore.GetLastPassingBuild(ctx, projectID, "", nil, 2)
	if err != nil {
		t.Fatalf("GetLastPassingBuild(\"\"): %v", err)
	}
	if got != nil {
		t.Errorf("want nil for an empty historyID (must not match unrelated empty-history_id rows), got %+v", got)
	}
}

// TestGetLastPassingBuild_OutOfOrderIngestion_UsesBuildOrderNotID is a
// regression test for keying GetLastPassingBuild on build_order instead of the
// surrogate builds.id. Builds are not guaranteed to be ingested in build_order
// sequence: a backfill/reconciliation pass can insert an older (lower
// build_order) build AFTER newer ones already exist, so its IDENTITY-generated
// id is HIGHER than builds with a greater build_order. This test constructs
// exactly that: build_order 5 and 10 are inserted first (ids increasing with
// insertion order), then build_order 7 is backfilled afterward and receives a
// HIGHER id than build_order 10 despite being chronologically earlier. The
// correct last-good build (by build_order, the human-facing build number)
// before build_order 10 is build_order 7 — the one with the greatest
// build_order below the bound — NOT the one with the greatest id.
func TestGetLastPassingBuild_OutOfOrderIngestion_UsesBuildOrderNotID(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	trStore := pg.NewTestResultStore(s, logger)

	slug := fmt.Sprintf("test-lastgood-outoforder-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	const historyID = "hOutOfOrder"

	// Ingestion order (matches insertion order below, NOT build_order order):
	//   1st inserted: build_order=5,  status=passed  → gets the LOWEST id
	//   2nd inserted: build_order=10, status=failed   → gets a HIGHER id (current)
	//   3rd inserted: build_order=7,  status=passed   → backfilled LAST, gets the
	//                                                    HIGHEST id despite build_order
	//                                                    7 < 10.
	type seed struct {
		order  int
		status string
	}
	for _, sd := range []seed{
		{5, "passed"},
		{10, "failed"},
		{7, "passed"}, // backfilled out of order
	} {
		if err := buildStore.InsertBuild(ctx, projectID, sd.order); err != nil {
			t.Fatalf("InsertBuild order=%d: %v", sd.order, err)
		}
		bid, err := trStore.GetBuildID(ctx, projectID, sd.order)
		if err != nil {
			t.Fatalf("GetBuildID order=%d: %v", sd.order, err)
		}
		if err := trStore.InsertBatch(ctx, []store.TestResult{{
			BuildID: bid, ProjectID: projectID,
			TestName: "out of order test", FullName: "suite > out of order test",
			HistoryID: historyID, Status: sd.status, DurationMs: 100,
		}}); err != nil {
			t.Fatalf("InsertBatch order=%d: %v", sd.order, err)
		}
	}

	// Sanity-check the premise: build_order=7's id must be greater than
	// build_order=10's id (the whole point of the regression). If this ever
	// stops holding (e.g. IDENTITY behavior changes), the test premise itself
	// is invalid, so fail loudly rather than silently passing for the wrong
	// reason.
	idOf := func(order int) int64 {
		bid, err := trStore.GetBuildID(ctx, projectID, order)
		if err != nil {
			t.Fatalf("GetBuildID order=%d: %v", order, err)
		}
		return bid
	}
	if idOf(7) <= idOf(10) {
		t.Fatalf("test premise violated: expected build_order=7's id (%d) > build_order=10's id (%d)", idOf(7), idOf(10))
	}

	got, err := trStore.GetLastPassingBuild(ctx, projectID, historyID, nil, 10)
	if err != nil {
		t.Fatalf("GetLastPassingBuild(before order 10): %v", err)
	}
	if got == nil {
		t.Fatal("want a last-good build before order 10, got nil")
	}
	if got.BuildNumber != 7 {
		t.Errorf("build_number: got %d, want 7 (greatest build_order below the bound) — "+
			"a build_order=5 result here means the query is still keying on builds.id", got.BuildNumber)
	}
}

// TestGetLastPassingBuild_BranchScoped verifies that when branchID is non-nil
// the query only considers builds on that branch, and that a nil branchID scopes
// cross-branch (returning the most recent prior pass on any branch).
func TestGetLastPassingBuild_BranchScoped(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	branchStore := pg.NewBranchStore(s)
	trStore := pg.NewTestResultStore(s, logger)

	slug := fmt.Sprintf("test-lastgood-branch-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	mainBranch, _, err := branchStore.GetOrCreate(ctx, projectID, "main")
	if err != nil {
		t.Fatalf("GetOrCreate main: %v", err)
	}
	featureBranch, _, err := branchStore.GetOrCreate(ctx, projectID, "feature")
	if err != nil {
		t.Fatalf("GetOrCreate feature: %v", err)
	}

	const historyID = "hBr"
	// order 1 (main): passed; order 2 (feature): passed; order 3 (feature):
	// failed (current). Inserted ascending so ids track build_order.
	builds := []struct {
		order    int
		branchID int64
		status   string
	}{
		{1, mainBranch.ID, "passed"},
		{2, featureBranch.ID, "passed"},
		{3, featureBranch.ID, "failed"},
	}
	buildIDByOrder := make(map[int]int64, len(builds))
	for _, b := range builds {
		if err := buildStore.InsertBuild(ctx, projectID, b.order); err != nil {
			t.Fatalf("InsertBuild %d: %v", b.order, err)
		}
		if err := buildStore.UpdateBuildBranchID(ctx, projectID, b.order, b.branchID); err != nil {
			t.Fatalf("UpdateBuildBranchID %d: %v", b.order, err)
		}
		bid, err := trStore.GetBuildID(ctx, projectID, b.order)
		if err != nil {
			t.Fatalf("GetBuildID %d: %v", b.order, err)
		}
		buildIDByOrder[b.order] = bid
		if err := trStore.InsertBatch(ctx, []store.TestResult{{
			BuildID: bid, ProjectID: projectID,
			TestName: "br test", FullName: "suite > br test",
			HistoryID: historyID, Status: b.status, DurationMs: 100,
		}}); err != nil {
			t.Fatalf("InsertBatch order %d: %v", b.order, err)
		}
	}

	const currentOrder = 3

	// Feature-scoped: only the feature-branch pass at order 2 qualifies.
	got, err := trStore.GetLastPassingBuild(ctx, projectID, historyID, &featureBranch.ID, currentOrder)
	if err != nil {
		t.Fatalf("GetLastPassingBuild(feature): %v", err)
	}
	if got == nil || got.BuildNumber != 2 {
		t.Fatalf("feature-scoped: got %+v, want build_number 2", got)
	}

	// Main-scoped: only the main-branch pass at order 1 qualifies.
	got, err = trStore.GetLastPassingBuild(ctx, projectID, historyID, &mainBranch.ID, currentOrder)
	if err != nil {
		t.Fatalf("GetLastPassingBuild(main): %v", err)
	}
	if got == nil || got.BuildNumber != 1 {
		t.Fatalf("main-scoped: got %+v, want build_number 1", got)
	}

	// Cross-branch (nil): most recent prior pass on any branch is order 2.
	got, err = trStore.GetLastPassingBuild(ctx, projectID, historyID, nil, currentOrder)
	if err != nil {
		t.Fatalf("GetLastPassingBuild(cross-branch): %v", err)
	}
	if got == nil || got.BuildNumber != 2 {
		t.Fatalf("cross-branch: got %+v, want build_number 2", got)
	}
}
