package pg_test

import (
	"errors"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// setLatestOn creates build `order` on the named branch and makes it that
// branch's is_latest build, the way the ingest path does.
func (f *fixture) setLatestOn(order int, branchName string) {
	f.t.Helper()
	branch := f.branch(branchName)
	f.build(order)
	if err := f.builds.UpdateBuildBranchID(f.ctx, f.id, order, branch.ID); err != nil {
		f.t.Fatalf("UpdateBuildBranchID %d: %v", order, err)
	}
	if err := f.builds.SetLatestBranch(f.ctx, f.id, order, &branch.ID); err != nil {
		f.t.Fatalf("SetLatestBranch %d: %v", order, err)
	}
}

// TestGetLatestBuild_MultiBranch: is_latest is maintained per branch, so a
// project can have several is_latest rows. The branchless "latest" alias must
// resolve to the newest of them (highest build_order), not whichever the
// planner returns first. The older feature-branch build is inserted first so
// the unordered physical scan would hand it back.
func TestGetLatestBuild_MultiBranch(t *testing.T) {
	f := newFixture(t)
	f.setLatestOn(1, "feature/x")
	f.setLatestOn(2, "main")

	got, err := f.builds.GetLatestBuild(f.ctx, f.id)
	if err != nil {
		t.Fatalf("GetLatestBuild: %v", err)
	}
	if got.BuildNumber != 2 {
		t.Errorf("GetLatestBuild = build %d, want 2 (newest across branches)", got.BuildNumber)
	}

	// A newer branch build supersedes only its own branch's flag: both rows
	// stay is_latest, and the alias follows the highest build_order.
	f.setLatestOn(3, "feature/x")
	got, err = f.builds.GetLatestBuild(f.ctx, f.id)
	if err != nil {
		t.Fatalf("GetLatestBuild after build 3: %v", err)
	}
	if got.BuildNumber != 3 {
		t.Errorf("GetLatestBuild = build %d, want 3", got.BuildNumber)
	}

	// Per-branch flags are untouched: build 2 is still main's latest.
	b2, err := f.builds.GetBuildByNumber(f.ctx, f.id, 2)
	if err != nil {
		t.Fatalf("GetBuildByNumber 2: %v", err)
	}
	if !b2.IsLatest {
		t.Error("build 2 lost its per-branch is_latest flag")
	}
}

// TestGetLatestBuild_IgnoresUnpublishedBuild: a reserved build that has not
// been published yet (is_latest=FALSE) must not win just because it has the
// highest build_order.
func TestGetLatestBuild_IgnoresUnpublishedBuild(t *testing.T) {
	f := newFixture(t)
	f.setLatestOn(1, "main")
	f.build(2) // reserved, never marked latest

	got, err := f.builds.GetLatestBuild(f.ctx, f.id)
	if err != nil {
		t.Fatalf("GetLatestBuild: %v", err)
	}
	if got.BuildNumber != 1 {
		t.Errorf("GetLatestBuild = build %d, want 1", got.BuildNumber)
	}
}

func TestGetLatestBuild_NoLatest(t *testing.T) {
	f := newFixture(t)
	if _, err := f.builds.GetLatestBuild(f.ctx, f.id); !errors.Is(err, store.ErrBuildNotFound) {
		t.Errorf("GetLatestBuild on empty project err = %v, want ErrBuildNotFound", err)
	}
}
