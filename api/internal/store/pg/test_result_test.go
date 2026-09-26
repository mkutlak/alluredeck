package pg_test

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/parser"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// TestInsertBatch_DuplicateHistoryID reproduces the production "duplicate key
// value violates unique constraint idx_test_results_build_history" warning:
// Allure retry attempts share one non-empty historyId in a build. InsertBatch
// must collapse them to one row keeping the latest attempt (greatest stop_ms)
// whatever the batch order — the latest is listed first, so last-writer-wins
// would keep the stale "failed" — while empty historyIds are never collapsed.
func TestInsertBatch_DuplicateHistoryID(t *testing.T) {
	f := newFixture(t)
	b := f.build(1)
	latest := f.result(b, "ui.printer.settings", "passed", "hist-printer")
	latest.Flaky, latest.Retries, latest.StartMs, latest.StopMs = true, 1, new(int64(150)), new(int64(200))
	earlier := f.result(b, "ui.printer.settings", "failed", "hist-printer")
	earlier.StartMs, earlier.StopMs = new(int64(60)), new(int64(100))
	f.insert(latest, earlier, f.result(b, "a", "passed", ""), f.result(b, "b", "failed", ""))

	if n := f.count("SELECT COUNT(*) FROM test_results WHERE build_id=$1 AND history_id=''", b); n != 2 {
		t.Errorf("empty-historyId rows = %d, want 2", n)
	}
	if n := f.count("SELECT COUNT(*) FROM test_results WHERE build_id=$1 AND history_id='hist-printer'", b); n != 1 {
		t.Fatalf("rows for (build_id, history_id) = %d, want 1", n)
	}
	var (
		status  string
		flaky   bool
		retries int
		stopMs  *int64
	)
	if err := f.s.Pool().QueryRow(f.ctx,
		"SELECT status, flaky, retries, stop_ms FROM test_results WHERE build_id=$1 AND history_id='hist-printer'",
		b).Scan(&status, &flaky, &retries, &stopMs); err != nil {
		t.Fatalf("scan surviving row: %v", err)
	}
	if status != "passed" || !flaky || retries != 1 || stopMs == nil || *stopMs != 200 {
		t.Errorf("survivor = (%q, flaky=%v, retries=%d, stop_ms=%v), want the latest attempt (passed, true, 1, 200)",
			status, flaky, retries, stopMs)
	}
}

// TestInsertBatchFull_DuplicateHistoryID covers the enrichment path for Allure
// retries: one *-result.json per attempt, sharing a historyId, collapse onto one
// test_results row with one set of labels/parameters/steps/attachments (not one
// per attempt), and the siblings become that row's attempts, ordered by StopMs
// rather than input order.
func TestInsertBatchFull_DuplicateHistoryID(t *testing.T) {
	f := newFixture(t)
	b := f.build(1)
	mk := func(status, msg string, stopMs int64) *parser.Result {
		return &parser.Result{
			Name: "printer settings", FullName: "ui.printer.settings", HistoryID: "hist-printer",
			Status: status, StatusMessage: msg, StartMs: stopMs - 40, StopMs: stopMs,
			Labels:      []parser.Label{{Name: "suite", Value: "printer"}},
			Parameters:  []parser.Parameter{{Name: "browser", Value: "chromium"}},
			Steps:       []parser.Step{{Name: "open dialog", Status: status, Order: 0}},
			Attachments: []parser.Attachment{{Name: "screenshot", Source: "shot-" + status + ".png", MimeType: "image/png"}},
		}
	}
	// Newest first, so trusting input order reverses the attempt sequence.
	f.insertFull(b, mk("passed", "", 300), mk("failed", "connection refused", 100), mk("failed", "connection refused", 200))

	q := "SELECT COUNT(*) FROM test_results WHERE build_id=$1 AND history_id='hist-printer'"
	if n := f.count(q, b); n != 1 {
		t.Fatalf("test_results rows = %d, want 1", n)
	}
	var resultID int64
	var status string
	if err := f.s.Pool().QueryRow(f.ctx, "SELECT id, status FROM test_results WHERE build_id=$1 AND history_id='hist-printer'",
		b).Scan(&resultID, &status); err != nil {
		t.Fatalf("scan surviving row: %v", err)
	}
	if status != "passed" {
		t.Errorf("surviving status = %q, want passed (latest attempt wins)", status)
	}
	for _, table := range []string{"test_labels", "test_parameters", "test_steps", "test_attachments"} {
		if n := f.count("SELECT COUNT(*) FROM "+table+" WHERE test_result_id=$1", resultID); n != 1 {
			t.Errorf("%s rows = %d, want 1 (duplicate enrichment children)", table, n)
		}
	}
	got, err := f.results.GetAttempts(f.ctx, f.id, b, "hist-printer")
	if err != nil {
		t.Fatalf("GetAttempts: %v", err)
	}
	want := []store.TestAttemptRow{
		{AttemptIndex: 0, Status: "failed", StatusMessage: "connection refused"},
		{AttemptIndex: 1, Status: "failed", StatusMessage: "connection refused"},
		{AttemptIndex: 2, Status: "passed"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("attempts = %+v, want %+v", got, want)
	}
}

// TestInsertBatchFull_MergesFlakyAndRetries: InsertBatchFull writes Flaky and
// Retries (the Playwright path), and a re-upsert carrying zeros — what the
// Allure parser yields during enrichment — must not clobber values recorded
// earlier by InsertBatchFull or by InsertBatch (Allure stability data): flaky
// is OR-merged and retries takes the max.
func TestInsertBatchFull_MergesFlakyAndRetries(t *testing.T) {
	f := newFixture(t)
	b := f.build(1)
	pw := &parser.Result{
		Name: "flaky pw test", FullName: "spec/flaky.ts > flaky pw test", HistoryID: "hist-pw",
		Status: "passed", StartMs: 100, StopMs: 200, Flaky: true, Retries: 2,
	}
	wantFlaky := func(historyID string, retries int) {
		t.Helper()
		var flaky bool
		var got int
		if err := f.s.Pool().QueryRow(f.ctx, "SELECT flaky, retries FROM test_results WHERE build_id=$1 AND history_id=$2",
			b, historyID).Scan(&flaky, &got); err != nil {
			t.Fatalf("scan %s: %v", historyID, err)
		}
		if !flaky || got != retries {
			t.Errorf("%s: flaky=%v retries=%d, want true/%d", historyID, flaky, got, retries)
		}
	}
	f.insertFull(b, pw)
	wantFlaky("hist-pw", 2)

	allure := f.result(b, "suite > flaky allure test", "passed", "hist-allure")
	allure.Flaky, allure.Retries, allure.StartMs, allure.StopMs = true, 3, new(int64(10)), new(int64(110))
	f.insert(allure)

	pw.Flaky, pw.Retries = false, 0
	f.insertFull(b, pw, &parser.Result{
		Name: "flaky allure test", FullName: "suite > flaky allure test", HistoryID: "hist-allure",
		Status: "passed", StartMs: 10, StopMs: 110,
	})
	wantFlaky("hist-pw", 2)
	wantFlaky("hist-allure", 3)
}

// TestGetTestHistory: every run of the test comes back with its flaky/retries
// and branch name, via a LEFT JOIN so a build without branch_id still appears.
func TestGetTestHistory(t *testing.T) {
	f := newFixture(t)
	main := f.branch("main")
	for i, status := range []string{"failed", "passed", "failed"} {
		order := i + 1
		r := f.result(f.build(order), "spec/branch.ts > branch test", status, "hist-branch")
		if order == 2 {
			r.Flaky, r.Retries = true, 3
		}
		f.insert(r)
		if order > 1 {
			if err := f.builds.UpdateBuildBranchID(f.ctx, f.id, order, main.ID); err != nil {
				t.Fatalf("UpdateBuildBranchID %d: %v", order, err)
			}
		}
	}

	entries, err := f.results.GetTestHistory(f.ctx, f.id, "hist-branch", nil, 10)
	if err != nil {
		t.Fatalf("GetTestHistory: %v", err)
	}
	got := map[int]store.TestHistoryEntry{}
	for _, e := range entries {
		got[e.BuildNumber] = store.TestHistoryEntry{BuildNumber: e.BuildNumber, Status: e.Status, Flaky: e.Flaky, Retries: e.Retries, BranchName: e.BranchName}
	}
	want := map[int]store.TestHistoryEntry{
		1: {BuildNumber: 1, Status: "failed"},
		2: {BuildNumber: 2, Status: "passed", Flaky: true, Retries: 3, BranchName: "main"},
		3: {BuildNumber: 3, Status: "failed", BranchName: "main"},
	}
	if len(entries) != 3 || !reflect.DeepEqual(got, want) {
		t.Errorf("history = %+v, want %+v", got, want)
	}
}

// TestGetLastPassingBuild finds the latest pass strictly before a build_order,
// optionally scoped to a branch. The "hOoO" builds are ingested as 10, 7, 5
// (backfills), so their IDENTITY ids run opposite to build_order: ordering by
// builds.id answers 5 instead of 7, and bounding by build 10's id finds none.
func TestGetLastPassingBuild(t *testing.T) {
	f := newFixture(t)
	main, feature := f.branch("main"), f.branch("feature")
	ids := map[int]int64{}
	for _, r := range []struct {
		order  int
		branch *store.Branch
		status string
	}{{1, main, "passed"}, {2, feature, "failed"}, {3, feature, "passed"}, {4, nil, "failed"}} {
		ids[r.order] = f.build(r.order)
		if r.branch != nil {
			if err := f.builds.UpdateBuildBranchID(f.ctx, f.id, r.order, r.branch.ID); err != nil {
				t.Fatalf("UpdateBuildBranchID %d: %v", r.order, err)
			}
		}
		f.insert(f.result(ids[r.order], "suite > lg test", r.status, "h"))
	}
	f.insert(f.result(ids[4], "suite > never", "failed", "hNever"), f.result(ids[1], "suite > unrelated", "passed", ""))
	if err := f.builds.UpdateBuildCIMetadata(f.ctx, f.id, 3, store.CIMetadata{CommitSHA: "sha-order-3"}); err != nil {
		t.Fatalf("UpdateBuildCIMetadata: %v", err)
	}
	for _, r := range []struct {
		order  int
		status string
	}{{10, "failed"}, {7, "passed"}, {5, "passed"}} {
		ids[r.order] = f.build(r.order)
		if err := f.builds.UpdateBuildBranchID(f.ctx, f.id, r.order, feature.ID); err != nil {
			t.Fatalf("UpdateBuildBranchID %d: %v", r.order, err)
		}
		f.insert(f.result(ids[r.order], "suite > out of order", r.status, "hOoO"))
	}
	if ids[5] <= ids[7] || ids[7] <= ids[10] {
		t.Fatalf("premise: backfilled ids must run id(5) > id(7) > id(10), got %d, %d, %d", ids[5], ids[7], ids[10])
	}

	tests := []struct {
		name      string
		historyID string
		branchID  *int64
		before    int
		want      int // build_order; 0 = no last-good build
	}{
		{"most recent prior pass", "h", nil, 4, 3},
		{"bound is exclusive", "h", nil, 3, 1},
		{"no prior pass", "h", nil, 1, 0},
		{"scoped to main", "h", &main.ID, 4, 1},
		{"scoped to feature", "h", &feature.ID, 4, 3},
		{"never passed", "hNever", nil, 4, 0},
		{"empty historyId never matches unrelated empty-history rows", "", nil, 2, 0},
		{"orders by build_order, not builds.id", "hOoO", nil, 10, 7},
		{"orders by build_order within a branch", "hOoO", &feature.ID, 10, 7},
	}
	for _, tt := range tests {
		got, err := f.results.GetLastPassingBuild(f.ctx, f.id, tt.historyID, tt.branchID, tt.before)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		gotOrder := 0
		if got != nil {
			gotOrder = got.BuildNumber
		}
		if gotOrder != tt.want {
			t.Errorf("%s: last-good build = %d, want %d", tt.name, gotOrder, tt.want)
		}
	}

	got, err := f.results.GetLastPassingBuild(f.ctx, f.id, "h", nil, 4)
	if err != nil || got == nil {
		t.Fatalf("GetLastPassingBuild = %v, %v", got, err)
	}
	got.CreatedAt = time.Time{}
	sha := "sha-order-3"
	want := store.TestHistoryEntry{BuildNumber: 3, BuildID: ids[3], Status: "passed", DurationMs: 100, CICommitSHA: &sha, BranchName: "feature"}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("last-good = %+v, want %+v", *got, want)
	}
}

// TestCountFailedByBuild_CountsDistinctFullNames: Playwright writes an enriched
// row and an empty shell per test under two history_id schemes, so the count
// is over DISTINCT full_name of failed+broken rows.
func TestCountFailedByBuild_CountsDistinctFullNames(t *testing.T) {
	f := newFixture(t)
	b := f.build(1)
	f.insert(f.result(b, "spec/a.ts > login", "failed", "abc:abc"), f.result(b, "spec/a.ts > login", "failed", "abc.abc"),
		f.result(b, "spec/b.ts > checkout", "broken", "def:def"), f.result(b, "spec/c.ts > search", "passed", "ghi:ghi"))
	if got, err := f.results.CountFailedByBuild(f.ctx, f.id, b); err != nil || got != 2 {
		t.Errorf("CountFailedByBuild = %d, %v; want 2", got, err)
	}
}

// TestGetByHistoryID_AnyStatusAndAbsent: GetByHistoryID returns the row
// whatever its status, with its surrogate id and status_message; a missing or
// empty historyId is (nil, nil). ListFailedByBuild selects the same surrogate
// id so callers can address the row without a re-lookup.
func TestGetByHistoryID_AnyStatusAndAbsent(t *testing.T) {
	f := newFixture(t)
	b := f.build(1)
	pass := f.result(b, "spec/green.ts > green test", "passed", "hist-passing")
	pass.Retries, pass.Flaky = 2, true
	f.insert(pass, f.result(b, "spec/red.ts > red test", "failed", "hist-red"))
	// InsertBatch does not write status_message.
	f.exec("UPDATE test_results SET status_message='assert failed' WHERE build_id=$1 AND history_id='hist-passing'", b)

	got, err := f.results.GetByHistoryID(f.ctx, f.id, b, "hist-passing")
	if err != nil || got == nil {
		t.Fatalf("GetByHistoryID(passing) = %v, %v; want the row", got, err)
	}
	if got.ID == 0 || got.Status != "passed" || got.StatusMessage != "assert failed" || got.Retries != 2 || !got.Flaky {
		t.Errorf("GetByHistoryID(passing) = %+v", got)
	}
	for _, hid := range []string{"no-such-history-id", ""} {
		if got, err := f.results.GetByHistoryID(f.ctx, f.id, b, hid); err != nil || got != nil {
			t.Errorf("GetByHistoryID(%q) = %+v, %v; want nil, nil", hid, got, err)
		}
	}

	red, err := f.results.GetByHistoryID(f.ctx, f.id, b, "hist-red")
	if err != nil || red == nil {
		t.Fatalf("GetByHistoryID(red) = %v, %v", red, err)
	}
	rows, err := f.results.ListFailedByBuild(f.ctx, f.id, b, 10)
	if err != nil {
		t.Fatalf("ListFailedByBuild: %v", err)
	}
	if len(rows) != 1 || rows[0].ID == 0 || rows[0].ID != red.ID {
		t.Errorf("ListFailedByBuild = %+v, want the one failed row with id %d", rows, red.ID)
	}
}
