package store_test

import (
	"context"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

func TestGetDashboardData_NoProjects(t *testing.T) {
	s := openTestStore(t)
	bs := store.NewBuildStore(s, zap.NewNop())
	ctx := context.Background()

	result, err := bs.GetDashboardData(ctx, 10, "")
	if err != nil {
		t.Fatalf("GetDashboardData: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty slice, got %d entries", len(result))
	}
}

func TestGetDashboardData_ProjectWithNoBuilds(t *testing.T) {
	s := openTestStore(t)
	ps := store.NewProjectStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ctx := context.Background()

	if err := ps.CreateProject(ctx, "proj-no-builds"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	result, err := bs.GetDashboardData(ctx, 10, "")
	if err != nil {
		t.Fatalf("GetDashboardData: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result))
	}
	if result[0].ProjectID != "proj-no-builds" {
		t.Errorf("expected project ID %q, got %q", "proj-no-builds", result[0].ProjectID)
	}
	if result[0].Latest != nil {
		t.Errorf("expected Latest=nil for project with no builds, got non-nil")
	}
	if len(result[0].Sparkline) != 0 {
		t.Errorf("expected empty sparkline, got %d entries", len(result[0].Sparkline))
	}
}

func TestGetDashboardData_ProjectWithBuilds(t *testing.T) {
	s := openTestStore(t)
	ps := store.NewProjectStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ctx := context.Background()

	projID := "proj-with-builds"
	if err := ps.CreateProject(ctx, projID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Insert 3 builds with stats.
	for i := 1; i <= 3; i++ {
		if err := bs.InsertBuild(ctx, projID, i); err != nil {
			t.Fatalf("InsertBuild %d: %v", i, err)
		}
		stats := store.BuildStats{
			Passed: 80 + i,
			Failed: 5,
			Total:  100,
		}
		if err := bs.UpdateBuildStats(ctx, projID, i, stats); err != nil {
			t.Fatalf("UpdateBuildStats %d: %v", i, err)
		}
	}
	// Mark build 3 as latest.
	if err := bs.SetLatest(ctx, projID, 3); err != nil {
		t.Fatalf("SetLatest: %v", err)
	}

	result, err := bs.GetDashboardData(ctx, 10, "")
	if err != nil {
		t.Fatalf("GetDashboardData: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result))
	}

	dp := result[0]
	if dp.ProjectID != projID {
		t.Errorf("project ID: got %q, want %q", dp.ProjectID, projID)
	}
	if dp.Latest == nil {
		t.Fatal("expected Latest != nil")
	}
	if dp.Latest.BuildOrder != 3 {
		t.Errorf("latest build order: got %d, want 3", dp.Latest.BuildOrder)
	}
	if !dp.Latest.IsLatest {
		t.Errorf("expected Latest.IsLatest=true")
	}

	// Sparkline: 3 builds, ascending build_order.
	if len(dp.Sparkline) != 3 {
		t.Fatalf("expected 3 sparkline points, got %d", len(dp.Sparkline))
	}
	// Verify ascending order.
	for i := 1; i < len(dp.Sparkline); i++ {
		if dp.Sparkline[i].BuildOrder <= dp.Sparkline[i-1].BuildOrder {
			t.Errorf("sparkline not in ascending order at index %d", i)
		}
	}
	// Verify pass rates: build 1 passed=81/100, build 2 passed=82/100, build 3 passed=83/100.
	expectedRates := []float64{81.0, 82.0, 83.0}
	for i, pt := range dp.Sparkline {
		if pt.PassRate != expectedRates[i] {
			t.Errorf("sparkline[%d].PassRate: got %.2f, want %.2f", i, pt.PassRate, expectedRates[i])
		}
	}
}

func TestGetDashboardData_SparklineDepth(t *testing.T) {
	s := openTestStore(t)
	ps := store.NewProjectStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ctx := context.Background()

	projID := "proj-depth"
	if err := ps.CreateProject(ctx, projID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Insert 15 builds all with stats.
	for i := 1; i <= 15; i++ {
		if err := bs.InsertBuild(ctx, projID, i); err != nil {
			t.Fatalf("InsertBuild %d: %v", i, err)
		}
		stats := store.BuildStats{
			Passed: 70 + i,
			Total:  100,
		}
		if err := bs.UpdateBuildStats(ctx, projID, i, stats); err != nil {
			t.Fatalf("UpdateBuildStats %d: %v", i, err)
		}
	}
	if err := bs.SetLatest(ctx, projID, 15); err != nil {
		t.Fatalf("SetLatest: %v", err)
	}

	result, err := bs.GetDashboardData(ctx, 10, "")
	if err != nil {
		t.Fatalf("GetDashboardData: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 project, got %d", len(result))
	}

	dp := result[0]
	if len(dp.Sparkline) != 10 {
		t.Errorf("expected 10 sparkline points with depth=10, got %d", len(dp.Sparkline))
	}

	// With depth=10 and 15 builds, we expect the most recent 10 (builds 6..15).
	// They should be in ascending order.
	if len(dp.Sparkline) == 10 {
		if dp.Sparkline[0].BuildOrder != 6 {
			t.Errorf("first sparkline build order: got %d, want 6", dp.Sparkline[0].BuildOrder)
		}
		if dp.Sparkline[9].BuildOrder != 15 {
			t.Errorf("last sparkline build order: got %d, want 15", dp.Sparkline[9].BuildOrder)
		}
	}
}

func TestGetDashboardData_MultipleProjects(t *testing.T) {
	s := openTestStore(t)
	ps := store.NewProjectStore(s, zap.NewNop())
	bs := store.NewBuildStore(s, zap.NewNop())
	ctx := context.Background()

	projects := []struct {
		id         string
		buildCount int
	}{
		{"alpha", 2},
		{"beta", 0},
		{"gamma", 5},
	}

	for _, p := range projects {
		if err := ps.CreateProject(ctx, p.id); err != nil {
			t.Fatalf("CreateProject %q: %v", p.id, err)
		}
		for i := 1; i <= p.buildCount; i++ {
			if err := bs.InsertBuild(ctx, p.id, i); err != nil {
				t.Fatalf("InsertBuild %q/%d: %v", p.id, i, err)
			}
			if err := bs.UpdateBuildStats(ctx, p.id, i, store.BuildStats{Passed: 50, Total: 100}); err != nil {
				t.Fatalf("UpdateBuildStats %q/%d: %v", p.id, i, err)
			}
		}
		if p.buildCount > 0 {
			if err := bs.SetLatest(ctx, p.id, p.buildCount); err != nil {
				t.Fatalf("SetLatest %q: %v", p.id, err)
			}
		}
	}

	result, err := bs.GetDashboardData(ctx, 10, "")
	if err != nil {
		t.Fatalf("GetDashboardData: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 projects, got %d", len(result))
	}

	// Build a map for easy lookup.
	byID := make(map[string]store.DashboardProject)
	for _, dp := range result {
		byID[dp.ProjectID] = dp
	}

	for _, p := range projects {
		dp, ok := byID[p.id]
		if !ok {
			t.Errorf("project %q not found in result", p.id)
			continue
		}
		if p.buildCount == 0 {
			if dp.Latest != nil {
				t.Errorf("project %q: expected Latest=nil, got non-nil", p.id)
			}
			if len(dp.Sparkline) != 0 {
				t.Errorf("project %q: expected empty sparkline, got %d", p.id, len(dp.Sparkline))
			}
		} else {
			if dp.Latest == nil {
				t.Errorf("project %q: expected Latest != nil", p.id)
			} else if dp.Latest.BuildOrder != p.buildCount {
				t.Errorf("project %q: latest build order: got %d, want %d", p.id, dp.Latest.BuildOrder, p.buildCount)
			}
			if len(dp.Sparkline) != p.buildCount {
				t.Errorf("project %q: expected %d sparkline points, got %d", p.id, p.buildCount, len(dp.Sparkline))
			}
		}
	}
}
