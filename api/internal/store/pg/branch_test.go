package pg_test

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestBranchList_UnionsAcrossChildren verifies that listing branches for a
// parent project returns the union of CI branches observed in builds across
// every child, even when each child's branches table holds a disjoint subset.
//
// Regression for the dropdown that only ever showed childIds[0]'s branches
// while pipeline rows aggregated across all children.
func TestBranchList_UnionsAcrossChildren(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	branchStore := pg.NewBranchStore(s)

	stamp := time.Now().UnixNano()

	parent, err := projectStore.CreateProject(ctx, fmt.Sprintf("test-branch-list-parent-%d", stamp))
	if err != nil {
		t.Fatalf("CreateProject parent: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), parent.ID) })

	childA, err := projectStore.CreateProjectWithParent(ctx, fmt.Sprintf("test-branch-list-childa-%d", stamp), parent.ID)
	if err != nil {
		t.Fatalf("CreateProjectWithParent childA: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), childA.ID) })

	childB, err := projectStore.CreateProjectWithParent(ctx, fmt.Sprintf("test-branch-list-childb-%d", stamp), parent.ID)
	if err != nil {
		t.Fatalf("CreateProjectWithParent childB: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), childB.ID) })

	// Child A has builds on master and v2.20, with branches table populated.
	insertBuildWithBranch(t, ctx, buildStore, childA.ID, 1, "master")
	insertBuildWithBranch(t, ctx, buildStore, childA.ID, 2, "v2.20")
	if _, _, err := branchStore.GetOrCreate(ctx, childA.ID, "master"); err != nil {
		t.Fatalf("GetOrCreate childA master: %v", err)
	}
	if _, _, err := branchStore.GetOrCreate(ctx, childA.ID, "v2.20"); err != nil {
		t.Fatalf("GetOrCreate childA v2.20: %v", err)
	}

	// Child B has a feature-branch build, but the branches row was never
	// written (simulates a silent GetOrCreate failure or an upload that
	// pre-dates migration 0010). The dropdown must still surface it.
	insertBuildWithBranch(t, ctx, buildStore, childB.ID, 1, "RG-4695-feature")

	branches, err := branchStore.List(ctx, parent.ID)
	if err != nil {
		t.Fatalf("List(parent): %v", err)
	}
	got := branchNames(branches)
	want := []string{"RG-4695-feature", "master", "v2.20"}
	if !equalSorted(got, want) {
		t.Errorf("parent branches: got %v, want %v", got, want)
	}
}

// TestBranchList_DerivesFromBuildsWhenTableEmpty covers the silent-ingestion
// case where the branches table is empty but builds carry ci_branch values.
func TestBranchList_DerivesFromBuildsWhenTableEmpty(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	branchStore := pg.NewBranchStore(s)

	proj, err := projectStore.CreateProject(ctx, fmt.Sprintf("test-branch-derive-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), proj.ID) })

	insertBuildWithBranch(t, ctx, buildStore, proj.ID, 1, "develop")
	insertBuildWithBranch(t, ctx, buildStore, proj.ID, 2, "develop")
	insertBuildWithBranch(t, ctx, buildStore, proj.ID, 3, "hotfix")

	branches, err := branchStore.List(ctx, proj.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	got := branchNames(branches)
	want := []string{"develop", "hotfix"}
	if !equalSorted(got, want) {
		t.Errorf("derived branches: got %v, want %v", got, want)
	}
}

// TestBranchList_EmptyForProjectWithoutBuilds returns no branches when there
// are no builds carrying CI metadata.
func TestBranchList_EmptyForProjectWithoutBuilds(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	branchStore := pg.NewBranchStore(s)

	proj, err := projectStore.CreateProject(ctx, fmt.Sprintf("test-branch-empty-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), proj.ID) })

	branches, err := branchStore.List(ctx, proj.ID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(branches) != 0 {
		t.Errorf("expected empty branches, got %v", branchNames(branches))
	}
}

func insertBuildWithBranch(t *testing.T, ctx context.Context, bs *pg.BuildStore, projectID int64, order int, branch string) {
	t.Helper()
	if err := bs.InsertBuild(ctx, projectID, order); err != nil {
		t.Fatalf("InsertBuild %d: %v", order, err)
	}
	if err := bs.UpdateBuildCIMetadata(ctx, projectID, order, store.CIMetadata{
		Branch:    branch,
		CommitSHA: fmt.Sprintf("sha-%d-%d", projectID, order),
	}); err != nil {
		t.Fatalf("UpdateBuildCIMetadata %d: %v", order, err)
	}
}

func branchNames(bs []store.Branch) []string {
	names := make([]string, 0, len(bs))
	for _, b := range bs {
		names = append(names, b.Name)
	}
	return names
}

func equalSorted(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aa := append([]string(nil), a...)
	bb := append([]string(nil), b...)
	sort.Strings(aa)
	sort.Strings(bb)
	for i := range aa {
		if aa[i] != bb[i] {
			return false
		}
	}
	return true
}
