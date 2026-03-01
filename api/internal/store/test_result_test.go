package store_test

import (
	"context"
	"fmt"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

func TestTestResultStore_InsertBatch_RoundTrip(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "tr-proj")
	_ = bs.InsertBuild(ctx, "tr-proj", 1)

	buildID, err := ts.GetBuildID(ctx, "tr-proj", 1)
	if err != nil {
		t.Fatalf("GetBuildID: %v", err)
	}

	results := []store.TestResult{
		{BuildID: buildID, ProjectID: "tr-proj", TestName: "TestA", FullName: "pkg.TestA", Status: "passed", DurationMs: 100, HistoryID: "h1"},
		{BuildID: buildID, ProjectID: "tr-proj", TestName: "TestB", FullName: "pkg.TestB", Status: "failed", DurationMs: 200, HistoryID: "h2", Flaky: true},
	}

	if err := ts.InsertBatch(ctx, results); err != nil {
		t.Fatalf("InsertBatch: %v", err)
	}

	// Verify by querying directly
	var count int
	if err := s.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM test_results WHERE project_id='tr-proj'",
	).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 rows, got %d", count)
	}
}

func TestTestResultStore_InsertBatch_Empty(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	ctx := context.Background()

	if err := ts.InsertBatch(ctx, nil); err != nil {
		t.Fatalf("InsertBatch(nil) should not error: %v", err)
	}
}

func TestTestResultStore_ListSlowest_Ordering(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "slow-proj")
	_ = bs.InsertBuild(ctx, "slow-proj", 1)
	buildID, _ := ts.GetBuildID(ctx, "slow-proj", 1)

	results := []store.TestResult{
		{BuildID: buildID, ProjectID: "slow-proj", TestName: "Fast", FullName: "pkg.Fast", Status: "passed", DurationMs: 100, HistoryID: "h-fast"},
		{BuildID: buildID, ProjectID: "slow-proj", TestName: "Slow", FullName: "pkg.Slow", Status: "passed", DurationMs: 5000, HistoryID: "h-slow"},
		{BuildID: buildID, ProjectID: "slow-proj", TestName: "Medium", FullName: "pkg.Medium", Status: "passed", DurationMs: 500, HistoryID: "h-med"},
	}
	_ = ts.InsertBatch(ctx, results)

	slowest, err := ts.ListSlowest(ctx, "slow-proj", 10, 10)
	if err != nil {
		t.Fatalf("ListSlowest: %v", err)
	}
	if len(slowest) != 3 {
		t.Fatalf("expected 3, got %d", len(slowest))
	}
	if slowest[0].TestName != "Slow" {
		t.Errorf("expected Slow first, got %s", slowest[0].TestName)
	}
	if slowest[2].TestName != "Fast" {
		t.Errorf("expected Fast last, got %s", slowest[2].TestName)
	}
}

func TestTestResultStore_ListLeastReliable_Ordering(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "reliable-proj")
	_ = bs.InsertBuild(ctx, "reliable-proj", 1)
	buildID, _ := ts.GetBuildID(ctx, "reliable-proj", 1)

	results := []store.TestResult{
		{BuildID: buildID, ProjectID: "reliable-proj", TestName: "Reliable", Status: "passed", DurationMs: 100, HistoryID: "h-rel"},
		{BuildID: buildID, ProjectID: "reliable-proj", TestName: "Flaky", Status: "failed", DurationMs: 200, HistoryID: "h-flaky"},
	}
	_ = ts.InsertBatch(ctx, results)

	unreliable, err := ts.ListLeastReliable(ctx, "reliable-proj", 10, 10)
	if err != nil {
		t.Fatalf("ListLeastReliable: %v", err)
	}
	if len(unreliable) != 1 {
		t.Fatalf("expected 1 (only failing tests), got %d", len(unreliable))
	}
	if unreliable[0].TestName != "Flaky" {
		t.Errorf("expected Flaky, got %s", unreliable[0].TestName)
	}
}

func TestTestResultStore_BuildsLimit(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "limit-proj")

	// Insert 5 builds, each with the same test
	for i := 1; i <= 5; i++ {
		_ = bs.InsertBuild(ctx, "limit-proj", i)
		buildID, _ := ts.GetBuildID(ctx, "limit-proj", i)
		_ = ts.InsertBatch(ctx, []store.TestResult{
			{BuildID: buildID, ProjectID: "limit-proj", TestName: "TestX", Status: "passed", DurationMs: int64(i * 100), HistoryID: "h-x"},
		})
	}

	// With builds=2, should only see 2 builds worth of data
	slowest, err := ts.ListSlowest(ctx, "limit-proj", 2, 10)
	if err != nil {
		t.Fatalf("ListSlowest: %v", err)
	}
	if len(slowest) != 1 {
		t.Fatalf("expected 1 test, got %d", len(slowest))
	}
	// build_count should be 2 (last 2 builds)
	if slowest[0].BuildCount != 2 {
		t.Errorf("expected build_count=2, got %d", slowest[0].BuildCount)
	}
	// trend should have 2 entries
	if len(slowest[0].Trend) != 2 {
		t.Errorf("expected trend len=2, got %d", len(slowest[0].Trend))
	}
}

func TestTestResultStore_ListSlowest_BatchTrends(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "batch-proj")

	// 3 builds, 3 tests with distinct duration patterns per build.
	durations := map[string][3]int64{
		"h-alpha": {100, 150, 200}, // Alpha: increasing
		"h-beta":  {300, 250, 200}, // Beta:  decreasing
		"h-gamma": {500, 600, 700}, // Gamma: increasing (slowest)
	}
	names := map[string]string{"h-alpha": "Alpha", "h-beta": "Beta", "h-gamma": "Gamma"}

	for buildOrder := 1; buildOrder <= 3; buildOrder++ {
		_ = bs.InsertBuild(ctx, "batch-proj", buildOrder)
		buildID, _ := ts.GetBuildID(ctx, "batch-proj", buildOrder)
		var batch []store.TestResult
		for hid, durs := range durations {
			batch = append(batch, store.TestResult{
				BuildID: buildID, ProjectID: "batch-proj",
				TestName: names[hid], FullName: "pkg." + names[hid],
				Status: "passed", DurationMs: durs[buildOrder-1], HistoryID: hid,
			})
		}
		_ = ts.InsertBatch(ctx, batch)
	}

	slowest, err := ts.ListSlowest(ctx, "batch-proj", 3, 10)
	if err != nil {
		t.Fatalf("ListSlowest: %v", err)
	}
	if len(slowest) != 3 {
		t.Fatalf("expected 3 tests, got %d", len(slowest))
	}

	trendMap := make(map[string][]float64)
	for _, s := range slowest {
		trendMap[s.TestName] = s.Trend
	}

	// Each test must have exactly 3 trend points (oldest→newest) mapped correctly.
	assertTrend(t, "Alpha", trendMap["Alpha"], []float64{100, 150, 200})
	assertTrend(t, "Beta", trendMap["Beta"], []float64{300, 250, 200})
	assertTrend(t, "Gamma", trendMap["Gamma"], []float64{500, 600, 700})
}

func TestTestResultStore_ListLeastReliable_BatchTrends(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "batch-rel-proj")

	// Build 1: TestA fails, TestB passes
	_ = bs.InsertBuild(ctx, "batch-rel-proj", 1)
	bid1, _ := ts.GetBuildID(ctx, "batch-rel-proj", 1)
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: bid1, ProjectID: "batch-rel-proj", TestName: "TestA", Status: "failed", DurationMs: 100, HistoryID: "h-a"},
		{BuildID: bid1, ProjectID: "batch-rel-proj", TestName: "TestB", Status: "passed", DurationMs: 100, HistoryID: "h-b"},
	})

	// Build 2: TestA passes, TestB fails
	_ = bs.InsertBuild(ctx, "batch-rel-proj", 2)
	bid2, _ := ts.GetBuildID(ctx, "batch-rel-proj", 2)
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: bid2, ProjectID: "batch-rel-proj", TestName: "TestA", Status: "passed", DurationMs: 100, HistoryID: "h-a"},
		{BuildID: bid2, ProjectID: "batch-rel-proj", TestName: "TestB", Status: "failed", DurationMs: 100, HistoryID: "h-b"},
	})

	unreliable, err := ts.ListLeastReliable(ctx, "batch-rel-proj", 2, 10)
	if err != nil {
		t.Fatalf("ListLeastReliable: %v", err)
	}
	if len(unreliable) != 2 {
		t.Fatalf("expected 2 unreliable tests, got %d", len(unreliable))
	}

	trendMap := make(map[string][]float64)
	for _, u := range unreliable {
		trendMap[u.TestName] = u.Trend
	}

	// TestA: build1=failed(1.0), build2=passed(0.0)
	assertTrend(t, "TestA", trendMap["TestA"], []float64{1.0, 0.0})
	// TestB: build1=passed(0.0), build2=failed(1.0)
	assertTrend(t, "TestB", trendMap["TestB"], []float64{0.0, 1.0})
}

func assertTrend(t *testing.T, name string, got, want []float64) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: trend len %d, want %d", name, len(got), len(want))
		return
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("%s: trend[%d] = %.1f, want %.1f", name, i, got[i], want[i])
		}
	}
}

func TestTestResultStore_AnalyticsIndexExists(t *testing.T) {
	s := openTestStore(t)
	var name string
	err := s.DB().QueryRow(
		"SELECT name FROM sqlite_master WHERE type='index' AND name='idx_test_results_analytics'",
	).Scan(&name)
	if err != nil {
		t.Fatalf("analytics covering index not found: %v", err)
	}
}

func TestTestResultStore_InsertBatch_TimelineFields(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "tl-proj")
	_ = bs.InsertBuild(ctx, "tl-proj", 1)
	buildID, _ := ts.GetBuildID(ctx, "tl-proj", 1)

	startMs := int64(1700000000000)
	stopMs := int64(1700000001000)
	results := []store.TestResult{
		{
			BuildID: buildID, ProjectID: "tl-proj",
			TestName: "TestTimeline", FullName: "pkg.TestTimeline",
			Status: "passed", DurationMs: 1000, HistoryID: "h-tl",
			StartMs: &startMs, StopMs: &stopMs,
			Thread: "pool-1-thread-3", Host: "worker-01",
		},
	}

	if err := ts.InsertBatch(ctx, results); err != nil {
		t.Fatalf("InsertBatch with timeline fields: %v", err)
	}

	// Verify timeline fields were stored.
	rows, err := ts.ListTimeline(ctx, "tl-proj", buildID, 100)
	if err != nil {
		t.Fatalf("ListTimeline: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 timeline row, got %d", len(rows))
	}
	r := rows[0]
	if r.TestName != "TestTimeline" {
		t.Errorf("TestName = %q, want %q", r.TestName, "TestTimeline")
	}
	if r.FullName != "pkg.TestTimeline" {
		t.Errorf("FullName = %q, want %q", r.FullName, "pkg.TestTimeline")
	}
	if r.Status != "passed" {
		t.Errorf("Status = %q, want %q", r.Status, "passed")
	}
	if r.StartMs != startMs {
		t.Errorf("StartMs = %d, want %d", r.StartMs, startMs)
	}
	if r.StopMs != stopMs {
		t.Errorf("StopMs = %d, want %d", r.StopMs, stopMs)
	}
	if r.Thread != "pool-1-thread-3" {
		t.Errorf("Thread = %q, want %q", r.Thread, "pool-1-thread-3")
	}
	if r.Host != "worker-01" {
		t.Errorf("Host = %q, want %q", r.Host, "worker-01")
	}
}

func TestTestResultStore_ListTimeline_Ordering(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "tl-order")
	_ = bs.InsertBuild(ctx, "tl-order", 1)
	buildID, _ := ts.GetBuildID(ctx, "tl-order", 1)

	start1, stop1 := int64(1000), int64(2000)
	start2, stop2 := int64(500), int64(1500)
	start3, stop3 := int64(1500), int64(3000)
	results := []store.TestResult{
		{BuildID: buildID, ProjectID: "tl-order", TestName: "B", FullName: "pkg.B", Status: "passed", DurationMs: 1000, HistoryID: "h1", StartMs: &start1, StopMs: &stop1},
		{BuildID: buildID, ProjectID: "tl-order", TestName: "A", FullName: "pkg.A", Status: "failed", DurationMs: 1000, HistoryID: "h2", StartMs: &start2, StopMs: &stop2},
		{BuildID: buildID, ProjectID: "tl-order", TestName: "C", FullName: "pkg.C", Status: "broken", DurationMs: 1500, HistoryID: "h3", StartMs: &start3, StopMs: &stop3},
	}
	_ = ts.InsertBatch(ctx, results)

	rows, err := ts.ListTimeline(ctx, "tl-order", buildID, 100)
	if err != nil {
		t.Fatalf("ListTimeline: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Should be ordered by start_ms ASC: A(500), B(1000), C(1500)
	if rows[0].TestName != "A" {
		t.Errorf("first row should be A (start=500), got %s", rows[0].TestName)
	}
	if rows[1].TestName != "B" {
		t.Errorf("second row should be B (start=1000), got %s", rows[1].TestName)
	}
	if rows[2].TestName != "C" {
		t.Errorf("third row should be C (start=1500), got %s", rows[2].TestName)
	}
}

func TestTestResultStore_ListTimeline_SkipsPreMigrationRows(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "tl-legacy")
	_ = bs.InsertBuild(ctx, "tl-legacy", 1)
	buildID, _ := ts.GetBuildID(ctx, "tl-legacy", 1)

	startMs := int64(1000)
	stopMs := int64(2000)
	results := []store.TestResult{
		// Pre-migration row: no start_ms/stop_ms (nil pointers)
		{BuildID: buildID, ProjectID: "tl-legacy", TestName: "OldTest", FullName: "pkg.OldTest", Status: "passed", DurationMs: 100, HistoryID: "h-old"},
		// Post-migration row: has start_ms
		{BuildID: buildID, ProjectID: "tl-legacy", TestName: "NewTest", FullName: "pkg.NewTest", Status: "passed", DurationMs: 200, HistoryID: "h-new", StartMs: &startMs, StopMs: &stopMs, Thread: "main", Host: "host1"},
	}
	_ = ts.InsertBatch(ctx, results)

	rows, err := ts.ListTimeline(ctx, "tl-legacy", buildID, 100)
	if err != nil {
		t.Fatalf("ListTimeline: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected 1 row (pre-migration excluded), got %d", len(rows))
	}
	if rows[0].TestName != "NewTest" {
		t.Errorf("expected NewTest, got %s", rows[0].TestName)
	}
}

func TestTestResultStore_ListTimeline_Empty(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	ctx := context.Background()

	rows, err := ts.ListTimeline(ctx, "nonexistent", 999, 100)
	if err != nil {
		t.Fatalf("ListTimeline on empty: %v", err)
	}
	if len(rows) != 0 {
		t.Errorf("expected empty slice, got %d", len(rows))
	}
}

func TestTestResultStore_ListTimeline_RespectsLimit(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "tl-limit")
	_ = bs.InsertBuild(ctx, "tl-limit", 1)
	buildID, _ := ts.GetBuildID(ctx, "tl-limit", 1)

	var batch []store.TestResult
	for i := 0; i < 10; i++ {
		start := int64(i * 1000)
		stop := start + 500
		batch = append(batch, store.TestResult{
			BuildID: buildID, ProjectID: "tl-limit",
			TestName: fmt.Sprintf("Test%d", i), FullName: fmt.Sprintf("pkg.Test%d", i),
			Status: "passed", DurationMs: 500, HistoryID: fmt.Sprintf("h-%d", i),
			StartMs: &start, StopMs: &stop,
		})
	}
	_ = ts.InsertBatch(ctx, batch)

	rows, err := ts.ListTimeline(ctx, "tl-limit", buildID, 3)
	if err != nil {
		t.Fatalf("ListTimeline: %v", err)
	}
	if len(rows) != 3 {
		t.Errorf("expected 3 rows (limit), got %d", len(rows))
	}
}

func TestTestResultStore_TimelineColumnsExist(t *testing.T) {
	s := openTestStore(t)
	cols := []string{"start_ms", "stop_ms", "thread", "host"}
	for _, col := range cols {
		var exists int
		err := s.DB().QueryRow(
			"SELECT COUNT(*) FROM pragma_table_info('test_results') WHERE name=?", col,
		).Scan(&exists)
		if err != nil {
			t.Errorf("checking column %q: %v", col, err)
		} else if exists == 0 {
			t.Errorf("column %q not found in test_results table", col)
		}
	}
}

func TestTestResultStore_ListFailedByBuild(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "fail-proj")
	_ = bs.InsertBuild(ctx, "fail-proj", 1)
	buildID, _ := ts.GetBuildID(ctx, "fail-proj", 1)

	results := []store.TestResult{
		{BuildID: buildID, ProjectID: "fail-proj", TestName: "PassingTest", FullName: "pkg.PassingTest", Status: "passed", DurationMs: 100, HistoryID: "h1"},
		{BuildID: buildID, ProjectID: "fail-proj", TestName: "FailedSlow", FullName: "pkg.FailedSlow", Status: "failed", DurationMs: 5000, HistoryID: "h2", NewFailed: true},
		{BuildID: buildID, ProjectID: "fail-proj", TestName: "BrokenTest", FullName: "pkg.BrokenTest", Status: "broken", DurationMs: 3000, HistoryID: "h3", Flaky: true},
		{BuildID: buildID, ProjectID: "fail-proj", TestName: "SkippedTest", FullName: "pkg.SkippedTest", Status: "skipped", DurationMs: 0, HistoryID: "h4"},
		{BuildID: buildID, ProjectID: "fail-proj", TestName: "FailedFast", FullName: "pkg.FailedFast", Status: "failed", DurationMs: 200, HistoryID: "h5"},
	}
	_ = ts.InsertBatch(ctx, results)

	// Should return only failed+broken, ordered by duration DESC.
	failures, err := ts.ListFailedByBuild(ctx, "fail-proj", buildID, 10)
	if err != nil {
		t.Fatalf("ListFailedByBuild: %v", err)
	}
	if len(failures) != 3 {
		t.Fatalf("expected 3 failed/broken results, got %d", len(failures))
	}
	// Ordered by duration DESC: FailedSlow(5000), BrokenTest(3000), FailedFast(200)
	if failures[0].TestName != "FailedSlow" {
		t.Errorf("first failure should be FailedSlow, got %s", failures[0].TestName)
	}
	if failures[1].TestName != "BrokenTest" {
		t.Errorf("second failure should be BrokenTest, got %s", failures[1].TestName)
	}
	if failures[2].TestName != "FailedFast" {
		t.Errorf("third failure should be FailedFast, got %s", failures[2].TestName)
	}
	// Verify NewFailed and Flaky flags.
	if !failures[0].NewFailed {
		t.Error("FailedSlow should have NewFailed=true")
	}
	if !failures[1].Flaky {
		t.Error("BrokenTest should have Flaky=true")
	}

	// Limit respected.
	limited, err := ts.ListFailedByBuild(ctx, "fail-proj", buildID, 2)
	if err != nil {
		t.Fatalf("ListFailedByBuild limit: %v", err)
	}
	if len(limited) != 2 {
		t.Errorf("expected 2 results with limit=2, got %d", len(limited))
	}

	// Empty result for project with no failures.
	_ = ps.CreateProject(ctx, "no-fail")
	_ = bs.InsertBuild(ctx, "no-fail", 1)
	noFailBID, _ := ts.GetBuildID(ctx, "no-fail", 1)
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: noFailBID, ProjectID: "no-fail", TestName: "AllGood", FullName: "pkg.AllGood", Status: "passed", DurationMs: 50, HistoryID: "h-ok"},
	})
	empty, err := ts.ListFailedByBuild(ctx, "no-fail", noFailBID, 10)
	if err != nil {
		t.Fatalf("ListFailedByBuild empty: %v", err)
	}
	if len(empty) != 0 {
		t.Errorf("expected 0 failures, got %d", len(empty))
	}
}

func TestTestResultStore_DeleteByBuild(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "dbb-proj")
	_ = bs.InsertBuild(ctx, "dbb-proj", 1)
	_ = bs.InsertBuild(ctx, "dbb-proj", 2)
	bid1, _ := ts.GetBuildID(ctx, "dbb-proj", 1)
	bid2, _ := ts.GetBuildID(ctx, "dbb-proj", 2)

	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: bid1, ProjectID: "dbb-proj", TestName: "TestA", FullName: "pkg.TestA", Status: "passed", DurationMs: 100, HistoryID: "h1"},
		{BuildID: bid1, ProjectID: "dbb-proj", TestName: "TestB", FullName: "pkg.TestB", Status: "failed", DurationMs: 200, HistoryID: "h2"},
	})
	_ = ts.InsertBatch(ctx, []store.TestResult{
		{BuildID: bid2, ProjectID: "dbb-proj", TestName: "TestA", FullName: "pkg.TestA", Status: "passed", DurationMs: 150, HistoryID: "h1"},
	})

	// Delete test results for build 1 only.
	if err := ts.DeleteByBuild(ctx, bid1); err != nil {
		t.Fatalf("DeleteByBuild: %v", err)
	}

	// Build 1 results should be gone.
	var count1 int
	_ = s.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM test_results WHERE build_id=?", bid1,
	).Scan(&count1)
	if count1 != 0 {
		t.Errorf("expected 0 results for build 1, got %d", count1)
	}

	// Build 2 results should be untouched.
	var count2 int
	_ = s.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM test_results WHERE build_id=?", bid2,
	).Scan(&count2)
	if count2 != 1 {
		t.Errorf("expected 1 result for build 2, got %d", count2)
	}
}

func TestTestResultStore_EmptyProject(t *testing.T) {
	s := openTestStore(t)
	ts := store.NewTestResultStore(s, zap.NewNop())
	ctx := context.Background()

	slowest, err := ts.ListSlowest(ctx, "nonexistent", 10, 10)
	if err != nil {
		t.Fatalf("ListSlowest on empty: %v", err)
	}
	if len(slowest) != 0 {
		t.Errorf("expected empty, got %d", len(slowest))
	}
}
