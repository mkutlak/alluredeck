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

// insertBuildAt inserts a build and back-dates its created_at to ts via a direct UPDATE.
func insertBuildAt(t *testing.T, ctx context.Context, buildStore *pg.PGBuildStore, s *pg.PGStore, projectID string, order int, ts time.Time) {
	t.Helper()
	if err := buildStore.InsertBuild(ctx, projectID, order); err != nil {
		t.Fatalf("InsertBuild %d: %v", order, err)
	}
	// Back-date the build so age-based pruning can be tested deterministically.
	if _, err := s.Pool().Exec(ctx, "UPDATE builds SET created_at=$1 WHERE project_id=$2 AND build_order=$3", ts, projectID, order); err != nil {
		t.Fatalf("backdating build %d: %v", order, err)
	}
}

func TestPruneBuildsByAge_OlderBuildsRemoved(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)

	projectID := fmt.Sprintf("test-prune-age-%d", time.Now().UnixNano())
	if err := projectStore.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	now := time.Now().UTC()
	old := now.Add(-48 * time.Hour)
	recent := now.Add(-1 * time.Hour)
	cutoff := now.Add(-24 * time.Hour)

	// Build 1: old, not latest — should be pruned.
	insertBuildAt(t, ctx, buildStore, s, projectID, 1, old)
	// Build 2: recent, not latest — should be kept.
	insertBuildAt(t, ctx, buildStore, s, projectID, 2, recent)

	removed, err := buildStore.PruneBuildsByAge(ctx, projectID, cutoff)
	if err != nil {
		t.Fatalf("PruneBuildsByAge: %v", err)
	}

	if len(removed) != 1 || removed[0] != 1 {
		t.Errorf("expected [1] removed, got %v", removed)
	}

	remaining, err := buildStore.ListBuilds(ctx, projectID)
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	if len(remaining) != 1 || remaining[0].BuildOrder != 2 {
		t.Errorf("expected only build 2 remaining, got %v", remaining)
	}
}

func TestPruneBuildsByAge_LatestBuildNeverPruned(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)

	projectID := fmt.Sprintf("test-prune-age-latest-%d", time.Now().UnixNano())
	if err := projectStore.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	old := time.Now().UTC().Add(-72 * time.Hour)
	cutoff := time.Now().UTC().Add(-1 * time.Hour) // far future cutoff

	// Build 1: old AND latest — must never be pruned.
	insertBuildAt(t, ctx, buildStore, s, projectID, 1, old)
	if err := buildStore.SetLatest(ctx, projectID, 1); err != nil {
		t.Fatalf("SetLatest: %v", err)
	}

	removed, err := buildStore.PruneBuildsByAge(ctx, projectID, cutoff)
	if err != nil {
		t.Fatalf("PruneBuildsByAge: %v", err)
	}

	if len(removed) != 0 {
		t.Errorf("expected no builds removed (latest must be protected), got %v", removed)
	}

	remaining, err := buildStore.ListBuilds(ctx, projectID)
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	if len(remaining) != 1 {
		t.Errorf("expected 1 build remaining, got %d", len(remaining))
	}
}

func TestPruneBuildsByAge_EmptyWhenNoMatch(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)

	projectID := fmt.Sprintf("test-prune-age-nomatch-%d", time.Now().UnixNano())
	if err := projectStore.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	recent := time.Now().UTC().Add(-1 * time.Hour)
	cutoff := time.Now().UTC().Add(-24 * time.Hour) // cutoff in the past; recent build is newer

	insertBuildAt(t, ctx, buildStore, s, projectID, 1, recent)

	removed, err := buildStore.PruneBuildsByAge(ctx, projectID, cutoff)
	if err != nil {
		t.Fatalf("PruneBuildsByAge: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("expected empty removed list, got %v", removed)
	}
}

func TestPruneBuildsByAge_FutureCutoffPrunesAllNonLatest(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)

	projectID := fmt.Sprintf("test-prune-age-future-%d", time.Now().UnixNano())
	if err := projectStore.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	now := time.Now().UTC()

	// Build 1 and 2: not latest (should be pruned with far-future cutoff).
	insertBuildAt(t, ctx, buildStore, s, projectID, 1, now.Add(-10*time.Hour))
	insertBuildAt(t, ctx, buildStore, s, projectID, 2, now.Add(-5*time.Hour))
	// Build 3: latest (must survive).
	insertBuildAt(t, ctx, buildStore, s, projectID, 3, now.Add(-1*time.Hour))
	if err := buildStore.SetLatest(ctx, projectID, 3); err != nil {
		t.Fatalf("SetLatest: %v", err)
	}

	farFuture := now.Add(365 * 24 * time.Hour)
	removed, err := buildStore.PruneBuildsByAge(ctx, projectID, farFuture)
	if err != nil {
		t.Fatalf("PruneBuildsByAge: %v", err)
	}

	if len(removed) != 2 {
		t.Errorf("expected 2 builds removed, got %v", removed)
	}

	remaining, err := buildStore.ListBuilds(ctx, projectID)
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	if len(remaining) != 1 || remaining[0].BuildOrder != 3 {
		t.Errorf("expected only build 3 remaining (latest), got %v", remaining)
	}

	// Verify removed order is ascending.
	if removed[0] != 1 || removed[1] != 2 {
		t.Errorf("expected removed=[1,2] in ascending order, got %v", removed)
	}
}

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
