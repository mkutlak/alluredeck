package store_test

import (
	"context"
	"go.uber.org/zap"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

func TestBuildStore_NextOrder_First(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ctx := context.Background()

	ps := store.NewProjectStore(s, zap.NewNop())
	_ = ps.CreateProject(ctx, "p1")

	order, err := bs.NextBuildOrder(ctx, "p1")
	if err != nil {
		t.Fatalf("NextBuildOrder: %v", err)
	}
	if order != 1 {
		t.Errorf("expected 1, got %d", order)
	}
}

func TestBuildStore_NextOrder_Increments(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "p2")
	_ = bs.InsertBuild(ctx, "p2", 1)
	_ = bs.InsertBuild(ctx, "p2", 2)

	order, err := bs.NextBuildOrder(ctx, "p2")
	if err != nil {
		t.Fatalf("NextBuildOrder: %v", err)
	}
	if order != 3 {
		t.Errorf("expected 3, got %d", order)
	}
}

func TestBuildStore_UpdateStats(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "stats-proj")
	_ = bs.InsertBuild(ctx, "stats-proj", 1)

	stats := store.BuildStats{Passed: 10, Failed: 2, Total: 12}
	if err := bs.UpdateBuildStats(ctx, "stats-proj", 1, stats); err != nil {
		t.Fatalf("UpdateBuildStats: %v", err)
	}

	builds, err := bs.ListBuilds(ctx, "stats-proj")
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	if len(builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(builds))
	}
	b := builds[0]
	if b.StatPassed == nil || *b.StatPassed != 10 {
		t.Errorf("StatPassed: got %v", b.StatPassed)
	}
	if b.StatTotal == nil || *b.StatTotal != 12 {
		t.Errorf("StatTotal: got %v", b.StatTotal)
	}
}

func TestBuildStore_ListDescending(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "order-proj")
	for i := 1; i <= 5; i++ {
		_ = bs.InsertBuild(ctx, "order-proj", i)
	}

	builds, err := bs.ListBuilds(ctx, "order-proj")
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	if len(builds) != 5 {
		t.Fatalf("expected 5, got %d", len(builds))
	}
	// Should be descending: 5, 4, 3, 2, 1
	for i, b := range builds {
		expected := 5 - i
		if b.BuildOrder != expected {
			t.Errorf("builds[%d].BuildOrder = %d, want %d", i, b.BuildOrder, expected)
		}
	}
}

func TestBuildStore_PruneKeepsNewest(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "prune-proj")
	for i := 1; i <= 5; i++ {
		_ = bs.InsertBuild(ctx, "prune-proj", i)
	}

	removed, err := bs.PruneBuilds(ctx, "prune-proj", 3)
	if err != nil {
		t.Fatalf("PruneBuilds: %v", err)
	}
	if len(removed) != 2 {
		t.Errorf("expected 2 removed, got %d: %v", len(removed), removed)
	}

	remaining, _ := bs.ListBuilds(ctx, "prune-proj")
	if len(remaining) != 3 {
		t.Errorf("expected 3 remaining, got %d", len(remaining))
	}
	// Remaining should be 5, 4, 3 (newest)
	for i, b := range remaining {
		expected := 5 - i
		if b.BuildOrder != expected {
			t.Errorf("remaining[%d] = %d, want %d", i, b.BuildOrder, expected)
		}
	}
}

// TestBuildStore_ListBuilds_InvalidCreatedAt verifies that a build with a
// corrupt created_at value is still returned with zero-value CreatedAt.
func TestBuildStore_ListBuilds_InvalidCreatedAt(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "bad-ts")
	_ = bs.InsertBuild(ctx, "bad-ts", 1)

	// Corrupt the created_at value via raw SQL.
	_, err := s.DB().ExecContext(ctx,
		"UPDATE builds SET created_at='not-a-timestamp' WHERE project_id=? AND build_order=?",
		"bad-ts", 1)
	if err != nil {
		t.Fatalf("corrupt created_at: %v", err)
	}

	builds, err := bs.ListBuilds(ctx, "bad-ts")
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	if len(builds) != 1 {
		t.Fatalf("expected 1 build, got %d", len(builds))
	}
	if !builds[0].CreatedAt.IsZero() {
		t.Errorf("expected zero CreatedAt for invalid timestamp, got %v", builds[0].CreatedAt)
	}
}

func TestBuildStore_SetLatest(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ps := store.NewProjectStore(s, zap.NewNop())
	ctx := context.Background()

	_ = ps.CreateProject(ctx, "latest-proj")
	_ = bs.InsertBuild(ctx, "latest-proj", 1)
	_ = bs.InsertBuild(ctx, "latest-proj", 2)
	_ = bs.InsertBuild(ctx, "latest-proj", 3)

	if err := bs.SetLatest(ctx, "latest-proj", 3); err != nil {
		t.Fatalf("SetLatest: %v", err)
	}

	builds, _ := bs.ListBuilds(ctx, "latest-proj")
	for _, b := range builds {
		if b.BuildOrder == 3 && !b.IsLatest {
			t.Error("build 3 should be latest")
		}
		if b.BuildOrder != 3 && b.IsLatest {
			t.Errorf("build %d should not be latest", b.BuildOrder)
		}
	}
}
