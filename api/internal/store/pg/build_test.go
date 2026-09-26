package pg_test

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// seedAt inserts a build, back-dated to ts and (when branch != "") carrying
// that ci_branch, so age- and branch-based pruning is deterministic.
func (f *fixture) seedAt(order int, branch string, ts time.Time) {
	f.t.Helper()
	f.build(order)
	f.exec("UPDATE builds SET created_at=$1, ci_branch=NULLIF($2, '') WHERE project_id=$3 AND build_order=$4",
		ts, branch, f.id, order)
}

func (f *fixture) buildNumbers() []int {
	f.t.Helper()
	builds, err := f.builds.ListBuilds(f.ctx, f.id)
	if err != nil {
		f.t.Fatalf("ListBuilds: %v", err)
	}
	var orders []int
	for i := range builds {
		orders = append(orders, builds[i].BuildNumber)
	}
	sort.Ints(orders)
	return orders
}

// wantBranch asserts whether the branches row name exists.
func (f *fixture) wantBranch(name string, exists bool) {
	f.t.Helper()
	_, err := f.branches.GetByName(f.ctx, f.id, name)
	if exists && err != nil {
		f.t.Errorf("branch %q: want it to survive, got err=%v", name, err)
	}
	if !exists && !errors.Is(err, store.ErrBranchNotFound) {
		f.t.Errorf("branch %q: want it gone (ErrBranchNotFound), got err=%v", name, err)
	}
}

// wantBuild asserts whether build_order order exists.
func (f *fixture) wantBuild(order int, exists bool) {
	f.t.Helper()
	_, err := f.builds.GetBuildByNumber(f.ctx, f.id, order)
	if exists && err != nil {
		f.t.Errorf("build %d: want it to survive, got err=%v", order, err)
	}
	if !exists && !errors.Is(err, store.ErrBuildNotFound) {
		f.t.Errorf("build %d: want it gone (ErrBuildNotFound), got err=%v", order, err)
	}
}

func TestPruneBuildsByAge(t *testing.T) {
	now := time.Now().UTC()
	type seed struct {
		order  int
		age    time.Duration
		latest bool
	}
	tests := []struct {
		name          string
		seeds         []seed
		cutoff        time.Time
		wantRemoved   []int
		wantRemaining []int
	}{
		{"older non-latest build removed", []seed{{1, 48 * time.Hour, false}, {2, time.Hour, false}},
			now.Add(-24 * time.Hour), []int{1}, []int{2}},
		{"latest build never pruned", []seed{{1, 72 * time.Hour, true}},
			now.Add(-time.Hour), nil, []int{1}},
		{"nothing older than the cutoff", []seed{{1, time.Hour, false}},
			now.Add(-24 * time.Hour), nil, []int{1}},
		{"future cutoff prunes every non-latest build, ascending",
			[]seed{{1, 10 * time.Hour, false}, {2, 5 * time.Hour, false}, {3, time.Hour, true}},
			now.Add(365 * 24 * time.Hour), []int{1, 2}, []int{3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			for _, sd := range tt.seeds {
				f.seedAt(sd.order, "", now.Add(-sd.age))
				if sd.latest {
					if err := f.builds.SetLatest(f.ctx, f.id, sd.order); err != nil {
						t.Fatalf("SetLatest: %v", err)
					}
				}
			}
			removed, err := f.builds.PruneBuildsByAge(f.ctx, f.id, tt.cutoff)
			if err != nil {
				t.Fatalf("PruneBuildsByAge: %v", err)
			}
			if !slices.Equal(removed, tt.wantRemoved) {
				t.Errorf("removed = %v, want %v", removed, tt.wantRemoved)
			}
			if got := f.buildNumbers(); !slices.Equal(got, tt.wantRemaining) {
				t.Errorf("remaining = %v, want %v", got, tt.wantRemaining)
			}
		})
	}
}

// TestScanBuild_BranchID verifies the nullable branch_id column scans into
// Build.BranchID: the id when set, nil when NULL.
func TestScanBuild_BranchID(t *testing.T) {
	f := newFixture(t)
	branch := f.branch("main")
	f.build(1)
	f.build(2) // InsertBuild leaves branch_id NULL
	if err := f.builds.UpdateBuildBranchID(f.ctx, f.id, 1, branch.ID); err != nil {
		t.Fatalf("UpdateBuildBranchID: %v", err)
	}
	for order, want := range map[int]*int64{1: &branch.ID, 2: nil} {
		b, err := f.builds.GetBuildByNumber(f.ctx, f.id, order)
		if err != nil {
			t.Fatalf("GetBuildByNumber %d: %v", order, err)
		}
		if (b.BranchID == nil) != (want == nil) || (want != nil && *b.BranchID != *want) {
			t.Errorf("build %d BranchID = %v, want %v", order, b.BranchID, want)
		}
	}
}

// TestGetDashboardData_MultiBranch_ReturnsOneProjectEntry: a project whose
// branches each hold an is_latest build appears once, with the highest
// build_order as Latest.
func TestGetDashboardData_MultiBranch_ReturnsOneProjectEntry(t *testing.T) {
	f := newFixture(t)
	for i, name := range []string{"main", "feature-a"} {
		order := i + 1
		branch := f.branch(name)
		f.build(order)
		if err := f.builds.UpdateBuildBranchID(f.ctx, f.id, order, branch.ID); err != nil {
			t.Fatalf("UpdateBuildBranchID %d: %v", order, err)
		}
		if err := f.builds.SetLatestBranch(f.ctx, f.id, order, &branch.ID); err != nil {
			t.Fatalf("SetLatestBranch %d: %v", order, err)
		}
	}
	all, err := f.builds.ListBuilds(f.ctx, f.id)
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	if notLatest := slices.IndexFunc(all, func(b store.Build) bool { return !b.IsLatest }); len(all) != 2 || notLatest != -1 {
		t.Fatalf("precondition: want 2 builds, both is_latest, got %+v", all)
	}

	dashboard, err := f.builds.GetDashboardData(f.ctx, 5)
	if err != nil {
		t.Fatalf("GetDashboardData: %v", err)
	}
	var found []store.DashboardProject
	for _, dp := range dashboard {
		if dp.ProjectID == f.id {
			found = append(found, dp)
		}
	}
	if len(found) != 1 || found[0].Latest == nil || found[0].Latest.BuildNumber != 2 {
		t.Fatalf("want one entry with Latest build 2 (highest build_order), got %+v", found)
	}
}

// TestReserveBuild_ConcurrentNoDuplicatesContiguous: concurrent ReserveBuild
// calls on one project allocate unique, gap-free orders 1..N.
func TestReserveBuild_ConcurrentNoDuplicatesContiguous(t *testing.T) {
	f := newFixture(t)
	// 10 goroutines sits under ReserveBuild's 50-attempt cap even without the
	// production per-project gen lock.
	const n = 10
	var (
		mu     sync.Mutex
		orders []int
		wg     sync.WaitGroup
	)
	errCh := make(chan error, n)
	for range n {
		wg.Go(func() {
			order, err := f.builds.ReserveBuild(f.ctx, f.id)
			if err != nil {
				errCh <- err
				return
			}
			mu.Lock()
			orders = append(orders, order)
			mu.Unlock()
		})
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("ReserveBuild: %v", err)
	}
	sort.Ints(orders)
	if want := []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}; !slices.Equal(orders, want) {
		t.Fatalf("orders = %v, want contiguous %v", orders, want)
	}
}

// TestReserveBuild_RacesWithInsertMissingBuilds runs ReserveBuild concurrently
// with the unlocked Sync import path; no error may surface and the final
// build_orders must be duplicate-free.
func TestReserveBuild_RacesWithInsertMissingBuilds(t *testing.T) {
	f := newFixture(t)
	const reservers = 10
	var wg sync.WaitGroup
	errCh := make(chan error, reservers+1)
	wg.Go(func() {
		if err := f.builds.InsertMissingBuilds(f.ctx, f.id, []int{1, 2, 3, 4, 5}); err != nil {
			errCh <- err
		}
	})
	for range reservers {
		wg.Go(func() {
			if _, err := f.builds.ReserveBuild(f.ctx, f.id); err != nil {
				errCh <- err
			}
		})
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent reserve/insert: %v", err)
	}
	orders := f.buildNumbers()
	if len(slices.Compact(slices.Clone(orders))) != len(orders) {
		t.Fatalf("duplicate build_order among %v", orders)
	}
}

// TestPruneStaleBranches seeds 55 stale non-default branches — five more than
// the per-call batch cap of 50 — plus a stale default branch and a fresh
// non-default one. The first call prunes exactly 50 branches and the second
// the remaining 5, returning their build_orders, deleting their builds and
// branches rows; the default branch is exempt even when stale, and the fresh
// branch survives both sweeps.
func TestPruneStaleBranches(t *testing.T) {
	f := newFixture(t)
	f.branch("main") // created first → default
	now := time.Now().UTC()
	stale := now.AddDate(0, 0, -40)
	const staleCount = 55
	for i := range staleCount {
		name := fmt.Sprintf("bstale-%02d", i)
		f.branch(name)
		f.seedAt(i+1, name, stale)
	}
	f.seedAt(100, "main", stale)
	f.branch("feature-fresh")
	f.seedAt(101, "feature-fresh", now)

	cutoff := now.AddDate(0, 0, -3)
	var all []int
	for call, want := range []int{50, 5} {
		removed, err := f.builds.PruneStaleBranches(f.ctx, f.id, cutoff)
		if err != nil {
			t.Fatalf("PruneStaleBranches call %d: %v", call+1, err)
		}
		if len(removed) != want {
			t.Errorf("call %d: removed %d builds, want %d", call+1, len(removed), want)
		}
		all = append(all, removed...)
	}
	sort.Ints(all)
	for i := range staleCount {
		if i >= len(all) || all[i] != i+1 {
			t.Fatalf("removed orders = %v, want 1..%d", all, staleCount)
		}
		f.wantBuild(i+1, false)
		f.wantBranch(fmt.Sprintf("bstale-%02d", i), false)
	}
	f.wantBuild(100, true)
	f.wantBranch("main", true)
	f.wantBuild(101, true)
	f.wantBranch("feature-fresh", true)
}

// TestDeleteOrphanBranches removes only branch rows with no builds that are
// older than the one-hour in-flight-ingest guard, reports the count, and is
// idempotent. The default branch survives on is_default alone.
func TestDeleteOrphanBranches(t *testing.T) {
	f := newFixture(t)
	f.branch("main")     // default, no builds
	f.branch("ghost")    // aged orphan
	f.branch("newghost") // fresh orphan: what an in-flight ingest looks like
	f.branch("active")
	f.exec("UPDATE branches SET created_at = now() - interval '2 hours' WHERE project_id=$1 AND name='ghost'", f.id)
	f.seedAt(1, "active", time.Now())

	for call, want := range []int64{1, 0} {
		n, err := f.builds.DeleteOrphanBranches(f.ctx, f.id)
		if err != nil {
			t.Fatalf("DeleteOrphanBranches call %d: %v", call+1, err)
		}
		if n != want {
			t.Errorf("call %d deleted %d rows, want %d", call+1, n, want)
		}
	}
	f.wantBranch("ghost", false)
	for _, name := range []string{"newghost", "main", "active"} {
		f.wantBranch(name, true)
	}
}

// TestPruneStaleBranches_PromotedToDefaultMidSweepSurvives: a stale branch
// promoted to default between the stale-name SELECT and its per-branch
// transaction is skipped by the outer is_default filter and — the regression —
// by the in-transaction re-check (driven through the test hook), so no builds
// are deleted and no build_orders are returned for storage pruning.
func TestPruneStaleBranches_PromotedToDefaultMidSweepSurvives(t *testing.T) {
	f := newFixture(t)
	f.branch("main")
	flip := f.branch("flip")
	now := time.Now().UTC()
	f.seedAt(1, "flip", now.AddDate(0, 0, -40))
	if err := f.branches.SetDefault(f.ctx, f.id, flip.ID); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}

	cutoff := now.AddDate(0, 0, -3)
	removed, err := f.builds.PruneStaleBranches(f.ctx, f.id, cutoff)
	if err != nil || len(removed) != 0 {
		t.Errorf("PruneStaleBranches = %v, %v; want no builds removed", removed, err)
	}
	removed, err = pg.PruneStaleBranchForTest(f.builds, f.ctx, f.id, "flip", cutoff)
	if err != nil || len(removed) != 0 {
		t.Errorf("per-branch prune = %v, %v; want the promoted default skipped", removed, err)
	}
	f.wantBuild(1, true)
	f.wantBranch("flip", true)
}
