package store_test

import (
	"context"
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
