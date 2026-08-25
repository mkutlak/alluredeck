package pg_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// insertBuildAt inserts a build and back-dates its created_at to ts via a direct UPDATE.
func insertBuildAt(t *testing.T, ctx context.Context, buildStore *pg.BuildStore, s *pg.PGStore, projectID int64, order int, ts time.Time) {
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

	slug := fmt.Sprintf("test-prune-age-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
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
	if len(remaining) != 1 || remaining[0].BuildNumber != 2 {
		t.Errorf("expected only build 2 remaining, got %v", remaining)
	}
}

func TestPruneBuildsByAge_LatestBuildNeverPruned(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)

	slug := fmt.Sprintf("test-prune-age-latest-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
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

	slug := fmt.Sprintf("test-prune-age-nomatch-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
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

	slug := fmt.Sprintf("test-prune-age-future-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
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
	if len(remaining) != 1 || remaining[0].BuildNumber != 3 {
		t.Errorf("expected only build 3 remaining (latest), got %v", remaining)
	}

	// Verify removed order is ascending.
	if removed[0] != 1 || removed[1] != 2 {
		t.Errorf("expected removed=[1,2] in ascending order, got %v", removed)
	}
}

// TestScanBuild_BranchIDNonNull verifies that when a build row has a non-NULL
// branch_id, the scanned Build.BranchID field is populated with that value.
func TestScanBuild_BranchIDNonNull(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	branchStore := pg.NewBranchStore(s)

	slug := fmt.Sprintf("test-scan-branchid-nonnull-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), proj.ID) })

	branch, _, err := branchStore.GetOrCreate(ctx, proj.ID, "main")
	if err != nil {
		t.Fatalf("GetOrCreate branch: %v", err)
	}

	if err := buildStore.InsertBuild(ctx, proj.ID, 1); err != nil {
		t.Fatalf("InsertBuild: %v", err)
	}
	if err := buildStore.UpdateBuildBranchID(ctx, proj.ID, 1, branch.ID); err != nil {
		t.Fatalf("UpdateBuildBranchID: %v", err)
	}

	b, err := buildStore.GetBuildByNumber(ctx, proj.ID, 1)
	if err != nil {
		t.Fatalf("GetBuildByNumber: %v", err)
	}
	if b.BranchID == nil {
		t.Fatal("BranchID: got nil, want non-nil")
	}
	if *b.BranchID != branch.ID {
		t.Errorf("BranchID: got %d, want %d", *b.BranchID, branch.ID)
	}
}

// TestScanBuild_BranchIDNull verifies that when a build row has a NULL
// branch_id (no branch association), the scanned Build.BranchID field is nil.
func TestScanBuild_BranchIDNull(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)

	slug := fmt.Sprintf("test-scan-branchid-null-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), proj.ID) })

	// InsertBuild does not set branch_id — it defaults to NULL.
	if err := buildStore.InsertBuild(ctx, proj.ID, 1); err != nil {
		t.Fatalf("InsertBuild: %v", err)
	}

	b, err := buildStore.GetBuildByNumber(ctx, proj.ID, 1)
	if err != nil {
		t.Fatalf("GetBuildByNumber: %v", err)
	}
	if b.BranchID != nil {
		t.Errorf("BranchID: got %d, want nil", *b.BranchID)
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

	slug := fmt.Sprintf("test-dashboard-multibranch-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
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
	dashboard, err := buildStore.GetDashboardData(ctx, 5)
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
		t.Fatalf("expected exactly 1 DashboardProject for project %d, got %d", projectID, len(found))
	}
	if found[0].Latest == nil {
		t.Fatal("expected Latest to be non-nil")
	}
	if found[0].Latest.BuildNumber != 2 {
		t.Errorf("expected Latest.BuildNumber=2 (highest build_order), got %d", found[0].Latest.BuildNumber)
	}
}

// TestReserveBuild_ConcurrentNoDuplicatesContiguous spawns N goroutines that
// each call ReserveBuild on one fresh project and asserts the allocated orders
// are unique and gap-free (1..N) — the race the plain NextBuildNumber+InsertBuild
// path could not guarantee.
func TestReserveBuild_ConcurrentNoDuplicatesContiguous(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)

	slug := fmt.Sprintf("test-reserve-concurrent-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	// 10 goroutines sits comfortably under ReserveBuild's raised 50-attempt cap,
	// keeping this reliable even without the production per-project gen lock.
	const n = 10
	var (
		mu     sync.Mutex
		orders []int
		wg     sync.WaitGroup
	)
	errCh := make(chan error, n)
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			order, err := buildStore.ReserveBuild(ctx, projectID)
			if err != nil {
				errCh <- err
				return
			}
			mu.Lock()
			orders = append(orders, order)
			mu.Unlock()
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("ReserveBuild: %v", err)
	}

	if len(orders) != n {
		t.Fatalf("expected %d orders, got %d: %v", n, len(orders), orders)
	}
	sort.Ints(orders)
	for i, o := range orders {
		if o != i+1 {
			t.Fatalf("expected contiguous 1..%d, got %v (orders[%d]=%d)", n, orders, i, o)
		}
	}
}

// TestReserveBuild_RacesWithInsertMissingBuilds runs several ReserveBuild
// goroutines concurrently with the unlocked Sync import path
// (InsertMissingBuilds) and asserts no error surfaces and the final set of
// build_orders is duplicate-free.
func TestReserveBuild_RacesWithInsertMissingBuilds(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)

	slug := fmt.Sprintf("test-reserve-race-sync-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	const reservers = 10
	var wg sync.WaitGroup
	errCh := make(chan error, reservers+1)

	wg.Go(func() {
		// Mimic the unlocked Sync import path inserting a run of build_orders.
		if err := buildStore.InsertMissingBuilds(ctx, projectID, []int{1, 2, 3, 4, 5}); err != nil {
			errCh <- err
		}
	})

	wg.Add(reservers)
	for range reservers {
		go func() {
			defer wg.Done()
			if _, err := buildStore.ReserveBuild(ctx, projectID); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrent reserve/insert: %v", err)
	}

	// Final invariant: build_orders are duplicate-free.
	builds, err := buildStore.ListBuilds(ctx, projectID)
	if err != nil {
		t.Fatalf("ListBuilds: %v", err)
	}
	seen := make(map[int]struct{}, len(builds))
	for _, b := range builds {
		if _, dup := seen[b.BuildNumber]; dup {
			t.Fatalf("duplicate build_order %d among %d builds", b.BuildNumber, len(builds))
		}
		seen[b.BuildNumber] = struct{}{}
	}
}

// TestPruneStaleBranches seeds a project with the default branch (STALE — its
// newest build is 40 days old, yet exempt from pruning because it is default), a
// stale non-default branch ("feature-x", newest build 40 days old), and a fresh
// non-default branch, then asserts PruneStaleBranches removes only the stale
// non-default branch's builds and branches row, returns its build_orders, and
// leaves the (stale) default and fresh branches intact. The stale default build
// proves the is_default exemption holds even under staleness.
func TestPruneStaleBranches(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	branchStore := pg.NewBranchStore(s)

	slug := fmt.Sprintf("test-prune-stale-branches-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	// "main" is created first → becomes the default branch; the others are
	// non-default.
	if _, _, err := branchStore.GetOrCreate(ctx, projectID, "main"); err != nil {
		t.Fatalf("GetOrCreate main: %v", err)
	}
	if _, _, err := branchStore.GetOrCreate(ctx, projectID, "feature-x"); err != nil {
		t.Fatalf("GetOrCreate feature-x: %v", err)
	}
	if _, _, err := branchStore.GetOrCreate(ctx, projectID, "feature-fresh"); err != nil {
		t.Fatalf("GetOrCreate feature-fresh: %v", err)
	}

	now := time.Now().UTC()
	// seedBranchBuild inserts a build and sets its ci_branch + created_at directly.
	seedBranchBuild := func(order int, branch string, ts time.Time) {
		t.Helper()
		if err := buildStore.InsertBuild(ctx, projectID, order); err != nil {
			t.Fatalf("InsertBuild %d: %v", order, err)
		}
		if _, err := s.Pool().Exec(ctx,
			"UPDATE builds SET ci_branch=$1, created_at=$2 WHERE project_id=$3 AND build_order=$4",
			branch, ts, projectID, order); err != nil {
			t.Fatalf("seed build %d: %v", order, err)
		}
	}

	// Build 1: default branch, STALE (40 days old) — must survive because the
	// default branch is exempt from stale-branch pruning even when stale.
	seedBranchBuild(1, "main", now.AddDate(0, 0, -40))
	// Build 2: stale non-default branch — must be pruned.
	seedBranchBuild(2, "feature-x", now.AddDate(0, 0, -40))
	// Build 3: fresh non-default branch — must survive.
	seedBranchBuild(3, "feature-fresh", now)

	cutoff := now.AddDate(0, 0, -3)
	removed, err := buildStore.PruneStaleBranches(ctx, projectID, cutoff)
	if err != nil {
		t.Fatalf("PruneStaleBranches: %v", err)
	}

	if len(removed) != 1 || removed[0] != 2 {
		t.Errorf("expected removed=[2] (feature-x's build_order), got %v", removed)
	}

	// feature-x's build must be gone.
	if _, err := buildStore.GetBuildByNumber(ctx, projectID, 2); !errors.Is(err, store.ErrBuildNotFound) {
		t.Errorf("expected build 2 gone (ErrBuildNotFound), got err=%v", err)
	}
	// feature-x's branches row must be gone.
	if _, err := branchStore.GetByName(ctx, projectID, "feature-x"); !errors.Is(err, store.ErrBranchNotFound) {
		t.Errorf("expected feature-x branch row gone (ErrBranchNotFound), got err=%v", err)
	}

	// Default branch build + row survive.
	if _, err := buildStore.GetBuildByNumber(ctx, projectID, 1); err != nil {
		t.Errorf("expected build 1 (main) to survive, got err=%v", err)
	}
	if _, err := branchStore.GetByName(ctx, projectID, "main"); err != nil {
		t.Errorf("expected main branch row to survive, got err=%v", err)
	}
	// Fresh non-default branch build + row survive.
	if _, err := buildStore.GetBuildByNumber(ctx, projectID, 3); err != nil {
		t.Errorf("expected build 3 (feature-fresh) to survive, got err=%v", err)
	}
	if _, err := branchStore.GetByName(ctx, projectID, "feature-fresh"); err != nil {
		t.Errorf("expected feature-fresh branch row to survive, got err=%v", err)
	}
}

// TestPruneStaleBranches_BatchedAcrossCalls seeds 55 stale non-default branches
// (one build each) — five more than the per-call batch cap of 50 — plus a stale
// default branch and a fresh non-default branch. The first call must prune
// exactly 50 branches and the second call the remaining 5, proving a large
// backlog drains incrementally across calls instead of requiring one oversized
// transaction. The default and fresh branches must survive both sweeps.
func TestPruneStaleBranches_BatchedAcrossCalls(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	branchStore := pg.NewBranchStore(s)

	slug := fmt.Sprintf("test-prune-stale-batched-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	// "main" is created first → becomes the default branch.
	if _, _, err := branchStore.GetOrCreate(ctx, projectID, "main"); err != nil {
		t.Fatalf("GetOrCreate main: %v", err)
	}

	now := time.Now().UTC()
	seedBranchBuild := func(order int, branch string, ts time.Time) {
		t.Helper()
		if _, _, err := branchStore.GetOrCreate(ctx, projectID, branch); err != nil {
			t.Fatalf("GetOrCreate %s: %v", branch, err)
		}
		if err := buildStore.InsertBuild(ctx, projectID, order); err != nil {
			t.Fatalf("InsertBuild %d: %v", order, err)
		}
		if _, err := s.Pool().Exec(ctx,
			"UPDATE builds SET ci_branch=$1, created_at=$2 WHERE project_id=$3 AND build_order=$4",
			branch, ts, projectID, order); err != nil {
			t.Fatalf("seed build %d: %v", order, err)
		}
	}

	// 55 stale non-default branches, one 40-day-old build each (orders 1..55).
	const staleCount = 55
	for i := range staleCount {
		seedBranchBuild(i+1, fmt.Sprintf("bstale-%02d", i), now.AddDate(0, 0, -40))
	}
	// Stale default branch build — exempt — and a fresh non-default branch.
	seedBranchBuild(100, "main", now.AddDate(0, 0, -40))
	seedBranchBuild(101, "feature-fresh", now)

	cutoff := now.AddDate(0, 0, -3)

	removed1, err := buildStore.PruneStaleBranches(ctx, projectID, cutoff)
	if err != nil {
		t.Fatalf("PruneStaleBranches call 1: %v", err)
	}
	if len(removed1) != 50 {
		t.Errorf("call 1: expected 50 removed builds (batch cap), got %d", len(removed1))
	}

	removed2, err := buildStore.PruneStaleBranches(ctx, projectID, cutoff)
	if err != nil {
		t.Fatalf("PruneStaleBranches call 2: %v", err)
	}
	if len(removed2) != 5 {
		t.Errorf("call 2: expected 5 removed builds (backlog remainder), got %d", len(removed2))
	}

	// Together the two sweeps must have removed exactly orders 1..55.
	all := append(append([]int{}, removed1...), removed2...)
	sort.Ints(all)
	for i := range staleCount {
		if i >= len(all) || all[i] != i+1 {
			t.Fatalf("expected removed orders 1..55, got %v", all)
		}
	}

	// Every stale branch row is gone; default and fresh rows survive.
	for i := range staleCount {
		name := fmt.Sprintf("bstale-%02d", i)
		if _, err := branchStore.GetByName(ctx, projectID, name); !errors.Is(err, store.ErrBranchNotFound) {
			t.Errorf("expected %s branch row gone (ErrBranchNotFound), got err=%v", name, err)
		}
	}
	if _, err := branchStore.GetByName(ctx, projectID, "main"); err != nil {
		t.Errorf("expected main branch row to survive, got err=%v", err)
	}
	if _, err := buildStore.GetBuildByNumber(ctx, projectID, 100); err != nil {
		t.Errorf("expected build 100 (stale default) to survive, got err=%v", err)
	}
	if _, err := buildStore.GetBuildByNumber(ctx, projectID, 101); err != nil {
		t.Errorf("expected build 101 (fresh branch) to survive, got err=%v", err)
	}
}

// TestDeleteOrphanBranches seeds a default branch, a branch with a build, an
// aged orphan branch row with no builds (as left behind by a failed ingest or
// a pre-GC prune), and a freshly-created orphan row (as an in-flight ingest
// produces before it writes the build's ci_branch), then asserts
// DeleteOrphanBranches removes only the aged orphan — the fresh row is spared
// by the one-hour age guard — and reports the deleted count. A second call
// must find nothing to delete.
func TestDeleteOrphanBranches(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	branchStore := pg.NewBranchStore(s)

	slug := fmt.Sprintf("test-delete-orphan-branches-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	// "main" first → default (no builds; must survive on is_default alone).
	if _, _, err := branchStore.GetOrCreate(ctx, projectID, "main"); err != nil {
		t.Fatalf("GetOrCreate main: %v", err)
	}
	// Aged orphan: row exists, no build ever references it, older than the
	// one-hour in-flight-ingest guard.
	if _, _, err := branchStore.GetOrCreate(ctx, projectID, "ghost"); err != nil {
		t.Fatalf("GetOrCreate ghost: %v", err)
	}
	if _, err := s.Pool().Exec(ctx,
		"UPDATE branches SET created_at = now() - interval '2 hours' WHERE project_id=$1 AND name='ghost'", projectID); err != nil {
		t.Fatalf("backdate ghost: %v", err)
	}
	// Fresh orphan: also build-less, but created just now — an in-flight ingest
	// looks exactly like this, so the age guard must spare it.
	if _, _, err := branchStore.GetOrCreate(ctx, projectID, "newghost"); err != nil {
		t.Fatalf("GetOrCreate newghost: %v", err)
	}
	// Active: row plus one build referencing it via ci_branch.
	if _, _, err := branchStore.GetOrCreate(ctx, projectID, "active"); err != nil {
		t.Fatalf("GetOrCreate active: %v", err)
	}
	if err := buildStore.InsertBuild(ctx, projectID, 1); err != nil {
		t.Fatalf("InsertBuild: %v", err)
	}
	if _, err := s.Pool().Exec(ctx,
		"UPDATE builds SET ci_branch='active' WHERE project_id=$1 AND build_order=1", projectID); err != nil {
		t.Fatalf("set ci_branch: %v", err)
	}

	n, err := buildStore.DeleteOrphanBranches(ctx, projectID)
	if err != nil {
		t.Fatalf("DeleteOrphanBranches: %v", err)
	}
	if n != 1 {
		t.Errorf("expected 1 orphan row deleted, got %d", n)
	}

	if _, err := branchStore.GetByName(ctx, projectID, "ghost"); !errors.Is(err, store.ErrBranchNotFound) {
		t.Errorf("expected ghost branch row gone (ErrBranchNotFound), got err=%v", err)
	}
	if _, err := branchStore.GetByName(ctx, projectID, "newghost"); err != nil {
		t.Errorf("expected fresh newghost branch row to survive the age guard, got err=%v", err)
	}
	if _, err := branchStore.GetByName(ctx, projectID, "main"); err != nil {
		t.Errorf("expected default main branch row to survive, got err=%v", err)
	}
	if _, err := branchStore.GetByName(ctx, projectID, "active"); err != nil {
		t.Errorf("expected active branch row to survive, got err=%v", err)
	}

	n, err = buildStore.DeleteOrphanBranches(ctx, projectID)
	if err != nil {
		t.Fatalf("DeleteOrphanBranches second call: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 rows on second call, got %d", n)
	}
}

// TestPruneStaleBranches_PromotedToDefaultMidSweepSurvives simulates the race
// where a stale branch is promoted to default (SetDefault) between the
// stale-name SELECT and its per-branch transaction. The public call must skip
// it via the outer is_default filter, and — the actual regression under test —
// the per-branch delete's in-transaction is_default re-check (driven directly
// through the test hook, as the mid-sweep flip would reach it) must also leave
// the branch's builds and row untouched and return no build_orders, so storage
// pruning cannot run for the surviving builds.
func TestPruneStaleBranches_PromotedToDefaultMidSweepSurvives(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()
	logger := zap.NewNop()

	projectStore := pg.NewProjectStore(s, logger)
	buildStore := pg.NewBuildStore(s, logger)
	branchStore := pg.NewBranchStore(s)

	slug := fmt.Sprintf("test-prune-stale-promoted-%d", time.Now().UnixNano())
	proj, err := projectStore.CreateProject(ctx, slug)
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projectID := proj.ID
	t.Cleanup(func() { _ = projectStore.DeleteProject(context.Background(), projectID) })

	// "main" first → default; "flip" is non-default with one stale build.
	if _, _, err := branchStore.GetOrCreate(ctx, projectID, "main"); err != nil {
		t.Fatalf("GetOrCreate main: %v", err)
	}
	flip, _, err := branchStore.GetOrCreate(ctx, projectID, "flip")
	if err != nil {
		t.Fatalf("GetOrCreate flip: %v", err)
	}
	now := time.Now().UTC()
	if err := buildStore.InsertBuild(ctx, projectID, 1); err != nil {
		t.Fatalf("InsertBuild: %v", err)
	}
	if _, err := s.Pool().Exec(ctx,
		"UPDATE builds SET ci_branch='flip', created_at=$1 WHERE project_id=$2 AND build_order=1",
		now.AddDate(0, 0, -40), projectID); err != nil {
		t.Fatalf("seed build: %v", err)
	}

	// Promote the stale branch to default — the mid-sweep flip.
	if err := branchStore.SetDefault(ctx, projectID, flip.ID); err != nil {
		t.Fatalf("SetDefault: %v", err)
	}

	cutoff := now.AddDate(0, 0, -3)

	// Public sweep: outer stale-name SELECT must already exclude the branch.
	removed, err := buildStore.PruneStaleBranches(ctx, projectID, cutoff)
	if err != nil {
		t.Fatalf("PruneStaleBranches: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("expected no builds removed for promoted default branch, got %v", removed)
	}

	// Per-branch step driven directly, as the mid-sweep flip would reach it:
	// the in-transaction is_default re-check must skip the delete.
	removed, err = pg.PruneStaleBranchForTest(buildStore, ctx, projectID, "flip", cutoff)
	if err != nil {
		t.Fatalf("pruneStaleBranch: %v", err)
	}
	if len(removed) != 0 {
		t.Errorf("expected per-branch delete to skip promoted default branch, got orders %v", removed)
	}

	if _, err := buildStore.GetBuildByNumber(ctx, projectID, 1); err != nil {
		t.Errorf("expected promoted branch's build to survive, got err=%v", err)
	}
	if _, err := branchStore.GetByName(ctx, projectID, "flip"); err != nil {
		t.Errorf("expected promoted branch row to survive, got err=%v", err)
	}
}
