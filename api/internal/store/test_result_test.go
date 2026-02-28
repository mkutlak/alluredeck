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
