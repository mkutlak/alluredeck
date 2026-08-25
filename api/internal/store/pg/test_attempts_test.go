package pg_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/parser"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// attemptsFixture spins up a project + build and returns the ids plus a live
// TestResultStore, so each attempt test starts from an empty build of its own.
func attemptsFixture(t *testing.T, namePrefix string) (*pg.PGStore, *pg.TestResultStore, int64, int64) {
	t.Helper()

	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	trStore := pg.NewTestResultStore(s, logger)

	slug := fmt.Sprintf("%s-%d", namePrefix, time.Now().UnixNano())
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
	return s, trStore, projectID, buildID
}

// TestInsertBatchFull_PersistsAttempts_AndGetAttemptsOrders covers the write and
// read halves together: InsertBatchFull must write one test_attempts row per
// parser attempt, and GetAttempts must return them ordered by attempt_index
// regardless of the order they were inserted in.
func TestInsertBatchFull_PersistsAttempts_AndGetAttemptsOrders(t *testing.T) {
	_, trStore, projectID, buildID := attemptsFixture(t, "test-attempts-write")
	ctx := context.Background()

	const historyID = "hist-attempts"
	// Attempts are handed over deliberately out of index order so a query that
	// relies on insertion order rather than ORDER BY attempt_index fails here.
	result := &parser.Result{
		Name: "retried test", FullName: "spec/retry.ts > retried test",
		HistoryID: historyID, Status: "failed", StartMs: 100, StopMs: 400, Retries: 2,
		Attempts: []parser.Attempt{
			{Index: 2, Status: "failed", StatusMessage: "third failure"},
			{Index: 0, Status: "failed", StatusMessage: "first failure"},
			{Index: 1, Status: "timedOut", StatusMessage: "Test timeout of 30000ms exceeded"},
		},
	}
	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{result}); err != nil {
		t.Fatalf("InsertBatchFull: %v", err)
	}

	got, err := trStore.GetAttempts(ctx, projectID, buildID, historyID)
	if err != nil {
		t.Fatalf("GetAttempts: %v", err)
	}
	want := []store.TestAttemptRow{
		{AttemptIndex: 0, Status: "failed", StatusMessage: "first failure"},
		{AttemptIndex: 1, Status: "timedOut", StatusMessage: "Test timeout of 30000ms exceeded"},
		{AttemptIndex: 2, Status: "failed", StatusMessage: "third failure"},
	}
	if len(got) != len(want) {
		t.Fatalf("attempts: got %d rows (%+v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("attempt %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestInsertBatchFull_ReIngestion_ReplacesAttempts pins the delete-then-insert
// rule. The parent INSERT is an upsert, so a re-upload lands on the SAME
// test_result_id; a plain insert would violate UNIQUE (test_result_id,
// attempt_index). The attempt sequence must instead converge on the latest
// upload rather than accumulating or erroring.
func TestInsertBatchFull_ReIngestion_ReplacesAttempts(t *testing.T) {
	_, trStore, projectID, buildID := attemptsFixture(t, "test-attempts-reingest")
	ctx := context.Background()

	const historyID = "hist-reingest"
	mk := func(attempts []parser.Attempt) *parser.Result {
		return &parser.Result{
			Name: "reingested", FullName: "spec/re.ts > reingested",
			HistoryID: historyID, Status: "failed", StartMs: 10, StopMs: 200,
			Attempts: attempts,
		}
	}

	first := mk([]parser.Attempt{
		{Index: 0, Status: "failed", StatusMessage: "old A"},
		{Index: 1, Status: "failed", StatusMessage: "old B"},
		{Index: 2, Status: "failed", StatusMessage: "old C"},
	})
	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{first}); err != nil {
		t.Fatalf("InsertBatchFull (first): %v", err)
	}

	// A corrected re-upload with a SHORTER sequence and different text.
	second := mk([]parser.Attempt{
		{Index: 0, Status: "failed", StatusMessage: "new A"},
		{Index: 1, Status: "passed", StatusMessage: ""},
	})
	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{second}); err != nil {
		t.Fatalf("InsertBatchFull (re-ingest): %v", err)
	}

	got, err := trStore.GetAttempts(ctx, projectID, buildID, historyID)
	if err != nil {
		t.Fatalf("GetAttempts: %v", err)
	}
	want := []store.TestAttemptRow{
		{AttemptIndex: 0, Status: "failed", StatusMessage: "new A"},
		{AttemptIndex: 1, Status: "passed", StatusMessage: ""},
	}
	if len(got) != len(want) {
		t.Fatalf("attempts after re-ingest: got %d rows (%+v), want %d — the old sequence must be replaced, not merged", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("attempt %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestInsertBatchFull_SingleAttempt_NotPersisted guards the row-count policy: a
// lone attempt supports no comparison and its outcome already sits on the
// test_results row, so it must not add a row per test per build.
func TestInsertBatchFull_SingleAttempt_NotPersisted(t *testing.T) {
	_, trStore, projectID, buildID := attemptsFixture(t, "test-attempts-single")
	ctx := context.Background()

	const historyID = "hist-single"
	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{{
		Name: "one shot", FullName: "spec/one.ts > one shot",
		HistoryID: historyID, Status: "failed", StartMs: 10, StopMs: 20,
		Attempts: []parser.Attempt{{Index: 0, Status: "failed", StatusMessage: "boom"}},
	}}); err != nil {
		t.Fatalf("InsertBatchFull: %v", err)
	}

	got, err := trStore.GetAttempts(ctx, projectID, buildID, historyID)
	if err != nil {
		t.Fatalf("GetAttempts: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("attempts for a single-attempt test: got %+v, want none", got)
	}
}

// TestInsertBatchFull_ReIngestion_SingleAttemptClearsStaleRows covers the
// boundary the row-count policy creates: dropping BELOW attemptsMinToPersist
// writes no rows, so the clearing cannot be a side effect of the insert. A
// report that no longer claims a retry sequence must still withdraw the one it
// claimed before, rather than leaving the previous upload's attempts attached
// to a test that now reports a single run.
func TestInsertBatchFull_ReIngestion_SingleAttemptClearsStaleRows(t *testing.T) {
	_, trStore, projectID, buildID := attemptsFixture(t, "test-attempts-clear")
	ctx := context.Background()

	const historyID = "hist-clear"
	mk := func(attempts []parser.Attempt) *parser.Result {
		return &parser.Result{
			Name: "withdrawn", FullName: "spec/w.ts > withdrawn",
			HistoryID: historyID, Status: "failed", StartMs: 10, StopMs: 200,
			Attempts: attempts,
		}
	}

	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{mk([]parser.Attempt{
		{Index: 0, Status: "failed", StatusMessage: "old A"},
		{Index: 1, Status: "failed", StatusMessage: "old B"},
	})}); err != nil {
		t.Fatalf("InsertBatchFull (first): %v", err)
	}

	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{mk([]parser.Attempt{
		{Index: 0, Status: "passed", StatusMessage: ""},
	})}); err != nil {
		t.Fatalf("InsertBatchFull (re-ingest): %v", err)
	}

	got, err := trStore.GetAttempts(ctx, projectID, buildID, historyID)
	if err != nil {
		t.Fatalf("GetAttempts: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("attempts after re-ingest with one attempt: got %+v, want none — the withdrawn sequence must be cleared", got)
	}
}

// TestInsertBatchFull_MultiResultBatch_ReplacesEveryResultsAttempts pins the
// batch-wide contract. Attempt rows are cleared for EVERY test result the batch
// touches, in one statement, not once per result: a batch mixing a test that
// still reports a retry sequence with one that no longer does must leave the
// first rewritten and the second empty.
func TestInsertBatchFull_MultiResultBatch_ReplacesEveryResultsAttempts(t *testing.T) {
	_, trStore, projectID, buildID := attemptsFixture(t, "test-attempts-multi")
	ctx := context.Background()

	const keepsID, dropsID = "hist-multi-keeps", "hist-multi-drops"
	mk := func(historyID, name string, attempts []parser.Attempt) *parser.Result {
		return &parser.Result{
			Name: name, FullName: "spec/multi.ts > " + name,
			HistoryID: historyID, Status: "failed", StartMs: 10, StopMs: 200,
			Attempts: attempts,
		}
	}

	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{
		mk(keepsID, "keeps", []parser.Attempt{
			{Index: 0, Status: "failed", StatusMessage: "old A"},
			{Index: 1, Status: "failed", StatusMessage: "old B"},
		}),
		mk(dropsID, "drops", []parser.Attempt{
			{Index: 0, Status: "failed", StatusMessage: "old X"},
			{Index: 1, Status: "failed", StatusMessage: "old Y"},
			{Index: 2, Status: "failed", StatusMessage: "old Z"},
		}),
	}); err != nil {
		t.Fatalf("InsertBatchFull (first): %v", err)
	}

	// Re-upload: "keeps" reports a different two-attempt sequence, "drops"
	// reports a single run and so must end up with no attempt rows at all.
	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{
		mk(keepsID, "keeps", []parser.Attempt{
			{Index: 0, Status: "failed", StatusMessage: "new A"},
			{Index: 1, Status: "passed", StatusMessage: ""},
		}),
		mk(dropsID, "drops", []parser.Attempt{
			{Index: 0, Status: "passed", StatusMessage: ""},
		}),
	}); err != nil {
		t.Fatalf("InsertBatchFull (re-ingest): %v", err)
	}

	keeps, err := trStore.GetAttempts(ctx, projectID, buildID, keepsID)
	if err != nil {
		t.Fatalf("GetAttempts(keeps): %v", err)
	}
	want := []store.TestAttemptRow{
		{AttemptIndex: 0, Status: "failed", StatusMessage: "new A"},
		{AttemptIndex: 1, Status: "passed", StatusMessage: ""},
	}
	if len(keeps) != len(want) {
		t.Fatalf("keeps attempts: got %d rows (%+v), want %d", len(keeps), keeps, len(want))
	}
	for i := range want {
		if keeps[i] != want[i] {
			t.Errorf("keeps attempt %d: got %+v, want %+v", i, keeps[i], want[i])
		}
	}

	drops, err := trStore.GetAttempts(ctx, projectID, buildID, dropsID)
	if err != nil {
		t.Fatalf("GetAttempts(drops): %v", err)
	}
	if len(drops) != 0 {
		t.Errorf("drops attempts: got %+v, want none", drops)
	}
}

// TestInsertBatchFull_AttemptMessageTruncated verifies the 2000-character write
// cap. The full text of the final attempt already lives in
// test_results.status_message/status_trace, so these rows only need enough to
// compare attempts against each other.
func TestInsertBatchFull_AttemptMessageTruncated(t *testing.T) {
	_, trStore, projectID, buildID := attemptsFixture(t, "test-attempts-truncate")
	ctx := context.Background()

	const historyID = "hist-truncate"
	long := strings.Repeat("x", 5000)
	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{{
		Name: "verbose", FullName: "spec/verbose.ts > verbose",
		HistoryID: historyID, Status: "failed", StartMs: 10, StopMs: 20,
		Attempts: []parser.Attempt{
			{Index: 0, Status: "failed", StatusMessage: long},
			{Index: 1, Status: "failed", StatusMessage: long},
		},
	}}); err != nil {
		t.Fatalf("InsertBatchFull: %v", err)
	}

	got, err := trStore.GetAttempts(ctx, projectID, buildID, historyID)
	if err != nil {
		t.Fatalf("GetAttempts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("attempts: got %d, want 2", len(got))
	}
	if len(got[0].StatusMessage) != 2000 {
		t.Errorf("status_message length: got %d, want 2000 (write-time cap)", len(got[0].StatusMessage))
	}
}

// TestGetAttempts_AbsentAndEmptyHistoryID verifies the two no-op paths: a test
// with no attempt rows yields an empty slice rather than an error, and an empty
// historyID short-circuits instead of matching every test that lacks one.
func TestGetAttempts_AbsentAndEmptyHistoryID(t *testing.T) {
	_, trStore, projectID, buildID := attemptsFixture(t, "test-attempts-absent")
	ctx := context.Background()

	// Seed a real two-attempt test under the EMPTY history_id so an unguarded
	// query would have rows to wrongly return.
	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{{
		Name: "no identity", FullName: "spec/anon.ts > no identity",
		HistoryID: "", Status: "failed", StartMs: 10, StopMs: 20,
		Attempts: []parser.Attempt{
			{Index: 0, Status: "failed", StatusMessage: "a"},
			{Index: 1, Status: "failed", StatusMessage: "b"},
		},
	}}); err != nil {
		t.Fatalf("InsertBatchFull: %v", err)
	}

	empty, err := trStore.GetAttempts(ctx, projectID, buildID, "")
	if err != nil {
		t.Fatalf("GetAttempts(empty history_id): %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("GetAttempts(empty history_id): got %+v, want none", empty)
	}

	absent, err := trStore.GetAttempts(ctx, projectID, buildID, "no-such-history-id")
	if err != nil {
		t.Fatalf("GetAttempts(absent): got error %v, want nil", err)
	}
	if len(absent) != 0 {
		t.Errorf("GetAttempts(absent): got %+v, want none", absent)
	}
}

// TestInsertBatchFull_AllureSiblings_BecomeAttempts covers the Allure ingestion
// shape. An Allure reporter writes one standalone *-result.json per attempt with
// no in-file retry structure, so a retried test arrives as several results
// sharing one historyId. Those siblings are the per-attempt record: the survivor
// must carry them as attempts, ordered oldest-first by StopMs, instead of the
// older attempts being silently dropped.
func TestInsertBatchFull_AllureSiblings_BecomeAttempts(t *testing.T) {
	s, trStore, projectID, buildID := attemptsFixture(t, "test-attempts-allure")
	ctx := context.Background()

	const historyID = "hist-allure-retry"
	mk := func(status, msg string, stopMs int64) *parser.Result {
		return &parser.Result{
			Name: "allure retried", FullName: "suite > allure retried",
			HistoryID: historyID, Status: status, StatusMessage: msg,
			StartMs: stopMs - 50, StopMs: stopMs,
			Labels: []parser.Label{{Name: "suite", Value: "s"}},
		}
	}
	// Newest first, so an implementation that trusts input order rather than
	// StopMs produces a reversed attempt sequence and fails here.
	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{
		mk("passed", "", 300),
		mk("failed", "connection refused", 100),
		mk("failed", "connection refused", 200),
	}); err != nil {
		t.Fatalf("InsertBatchFull: %v", err)
	}

	got, err := trStore.GetAttempts(ctx, projectID, buildID, historyID)
	if err != nil {
		t.Fatalf("GetAttempts: %v", err)
	}
	want := []store.TestAttemptRow{
		{AttemptIndex: 0, Status: "failed", StatusMessage: "connection refused"},
		{AttemptIndex: 1, Status: "failed", StatusMessage: "connection refused"},
		{AttemptIndex: 2, Status: "passed", StatusMessage: ""},
	}
	if len(got) != len(want) {
		t.Fatalf("attempts: got %d rows (%+v), want %d", len(got), got, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("attempt %d: got %+v, want %+v", i, got[i], want[i])
		}
	}

	// The collapse itself must be unaffected: still exactly one test_results row
	// and one set of enrichment children.
	var resultID int64
	var status string
	if err := s.Pool().QueryRow(ctx,
		"SELECT id, status FROM test_results WHERE build_id=$1 AND history_id=$2",
		buildID, historyID).Scan(&resultID, &status); err != nil {
		t.Fatalf("expected exactly one surviving row: %v", err)
	}
	if status != "passed" {
		t.Errorf("surviving status: got %q, want passed (latest attempt wins)", status)
	}
	var labels int
	if err := s.Pool().QueryRow(ctx,
		"SELECT COUNT(*) FROM test_labels WHERE test_result_id=$1", resultID).Scan(&labels); err != nil {
		t.Fatalf("count test_labels: %v", err)
	}
	if labels != 1 {
		t.Errorf("test_labels rows: got %d, want 1", labels)
	}
}

// TestDeleteByBuild_CascadesAttempts verifies the FK cascade: retention pruning
// a build must take its attempt rows with it rather than orphaning them.
func TestDeleteByBuild_CascadesAttempts(t *testing.T) {
	s, trStore, projectID, buildID := attemptsFixture(t, "test-attempts-cascade")
	ctx := context.Background()

	if err := trStore.InsertBatchFull(ctx, buildID, projectID, []*parser.Result{{
		Name: "doomed", FullName: "spec/doomed.ts > doomed",
		HistoryID: "hist-doomed", Status: "failed", StartMs: 10, StopMs: 20,
		Attempts: []parser.Attempt{
			{Index: 0, Status: "failed", StatusMessage: "a"},
			{Index: 1, Status: "failed", StatusMessage: "b"},
		},
	}}); err != nil {
		t.Fatalf("InsertBatchFull: %v", err)
	}

	countAttempts := func() int {
		var n int
		if err := s.Pool().QueryRow(ctx, `
			SELECT COUNT(*) FROM test_attempts ta
			JOIN test_results tr ON tr.id = ta.test_result_id
			WHERE tr.build_id=$1`, buildID).Scan(&n); err != nil {
			t.Fatalf("count test_attempts: %v", err)
		}
		return n
	}
	if n := countAttempts(); n != 2 {
		t.Fatalf("attempts before delete: got %d, want 2", n)
	}
	if err := trStore.DeleteByBuild(ctx, buildID); err != nil {
		t.Fatalf("DeleteByBuild: %v", err)
	}
	if n := countAttempts(); n != 0 {
		t.Errorf("attempts after DeleteByBuild: got %d, want 0 (ON DELETE CASCADE)", n)
	}
}
