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

// TestGetDashboardData_MultiBranch_ReturnsOneProjectEntry verifies that a project
// with builds on multiple branches (each with is_latest=TRUE) appears exactly once
// in the dashboard result, with the most recent build (highest build_order) as Latest.
func TestGetDashboardData_MultiBranch_ReturnsOneProjectEntry(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	branchStore := pg.NewBranchStore(s)

	projectID := fmt.Sprintf("test-dashboard-multibranch-%d", time.Now().UnixNano())
	if err := projectStore.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	// Create two branches — each will have its own is_latest=TRUE build.
	mainBranch, _, err := branchStore.GetOrCreate(ctx, projectID, "main")
	if err != nil {
		t.Fatalf("GetOrCreate main: %v", err)
	}
	featureBranch, _, err := branchStore.GetOrCreate(ctx, projectID, "feature-a")
	if err != nil {
		t.Fatalf("GetOrCreate feature-a: %v", err)
	}

	// Build 1 on main branch (lower build_order).
	if err := buildStore.InsertBuild(ctx, projectID, 1); err != nil {
		t.Fatalf("InsertBuild 1: %v", err)
	}
	if err := buildStore.UpdateBuildBranchID(ctx, projectID, 1, mainBranch.ID); err != nil {
		t.Fatalf("UpdateBuildBranchID 1: %v", err)
	}
	if err := buildStore.SetLatestBranch(ctx, projectID, 1, &mainBranch.ID); err != nil {
		t.Fatalf("SetLatestBranch 1: %v", err)
	}

	// Build 2 on feature-a branch (higher build_order — must be the dashboard entry).
	if err := buildStore.InsertBuild(ctx, projectID, 2); err != nil {
		t.Fatalf("InsertBuild 2: %v", err)
	}
	if err := buildStore.UpdateBuildBranchID(ctx, projectID, 2, featureBranch.ID); err != nil {
		t.Fatalf("UpdateBuildBranchID 2: %v", err)
	}
	if err := buildStore.SetLatestBranch(ctx, projectID, 2, &featureBranch.ID); err != nil {
		t.Fatalf("SetLatestBranch 2: %v", err)
	}

	// Precondition: both builds must have is_latest=TRUE to reproduce the bug scenario.
	allBuilds, err := buildStore.ListBuilds(ctx, projectID)
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	var latestCount int
	for _, b := range allBuilds {
		if b.IsLatest {
			latestCount++
		}
	}
	if latestCount != 2 {
		t.Fatalf("precondition: expected 2 builds with is_latest=TRUE, got %d", latestCount)
	}

	// GetDashboardData must return exactly one entry for this project.
	dashboard, err := buildStore.GetDashboardData(ctx, 5, "")
	if err != nil {
		t.Fatalf("GetDashboardData: %v", err)
	}

	var found []store.DashboardProject
	for _, dp := range dashboard {
		if dp.ProjectID == projectID {
			found = append(found, dp)
		}
	}

	if len(found) != 1 {
		t.Fatalf("expected exactly 1 DashboardProject for project %q, got %d", projectID, len(found))
	}
	if found[0].Latest == nil {
		t.Fatal("expected Latest to be non-nil")
	}
	if found[0].Latest.BuildOrder != 2 {
		t.Errorf("expected Latest.BuildOrder=2 (highest build_order), got %d", found[0].Latest.BuildOrder)
	}
}
