package store_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// TestSyncStatsIfMissing_ReadBuildStatsError_SkipsUpdate verifies that when
// ReadBuildStats returns an error, SyncMetadata skips the stats UPDATE and
// leaves stat_total as NULL so the row is retried on next startup.
func TestSyncStatsIfMissing_ReadBuildStatsError_SkipsUpdate(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	// Seed a project and one build via the mock store.
	mock := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{"proj-sync"}, nil
		},
		ListReportBuildsFn: func(_ context.Context, _ string) ([]int, error) {
			return []int{1}, nil
		},
		ReadBuildStatsFn: func(_ context.Context, _ string, _ int) (storage.BuildStats, error) {
			return storage.BuildStats{}, errors.New("storage unavailable")
		},
	}

	if err := store.SyncMetadata(ctx, mock, s, zap.NewNop()); err != nil {
		t.Fatalf("SyncMetadata: %v", err)
	}

	// Verify stat_total remains NULL (not zero).
	var statTotal *int
	err := s.DB().QueryRowContext(ctx,
		"SELECT stat_total FROM builds WHERE project_id=? AND build_order=?",
		"proj-sync", 1,
	).Scan(&statTotal)
	if err != nil {
		t.Fatalf("scan stat_total: %v", err)
	}
	if statTotal != nil {
		t.Errorf("expected stat_total to be NULL, got %d", *statTotal)
	}
}

// --- Phase 1: Diff logic tests ---

func TestSyncProject_WarmRestart_NoInserts(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const projectID = "proj-warm"

	// Pre-insert project and builds 1,2,3.
	seedProjectAndBuilds(t, s, ctx, projectID, []int{1, 2, 3})

	mock := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{projectID}, nil
		},
		ListReportBuildsFn: func(_ context.Context, _ string) ([]int, error) {
			return []int{1, 2, 3}, nil
		},
		ReadBuildStatsFn: func(_ context.Context, _ string, _ int) (storage.BuildStats, error) {
			return storage.BuildStats{}, errors.New("no stats")
		},
	}

	if err := store.SyncMetadata(ctx, mock, s, zap.NewNop()); err != nil {
		t.Fatalf("SyncMetadata: %v", err)
	}

	count := countBuilds(t, s, ctx, projectID)
	if count != 3 {
		t.Errorf("expected 3 builds, got %d", count)
	}
}

func TestSyncProject_ColdStart_InsertsAllBuilds(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const projectID = "proj-cold"

	mock := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{projectID}, nil
		},
		ListReportBuildsFn: func(_ context.Context, _ string) ([]int, error) {
			return []int{1, 2, 3}, nil
		},
		ReadBuildStatsFn: func(_ context.Context, _ string, _ int) (storage.BuildStats, error) {
			return storage.BuildStats{}, errors.New("no stats")
		},
	}

	if err := store.SyncMetadata(ctx, mock, s, zap.NewNop()); err != nil {
		t.Fatalf("SyncMetadata: %v", err)
	}

	count := countBuilds(t, s, ctx, projectID)
	if count != 3 {
		t.Errorf("expected 3 builds, got %d", count)
	}
}

func TestSyncProject_PartialSync_InsertsOnlyMissing(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const projectID = "proj-partial"

	// Pre-insert builds 1,2,3.
	seedProjectAndBuilds(t, s, ctx, projectID, []int{1, 2, 3})

	mock := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{projectID}, nil
		},
		ListReportBuildsFn: func(_ context.Context, _ string) ([]int, error) {
			return []int{1, 2, 3, 4, 5}, nil
		},
		ReadBuildStatsFn: func(_ context.Context, _ string, _ int) (storage.BuildStats, error) {
			return storage.BuildStats{}, errors.New("no stats")
		},
	}

	if err := store.SyncMetadata(ctx, mock, s, zap.NewNop()); err != nil {
		t.Fatalf("SyncMetadata: %v", err)
	}

	count := countBuilds(t, s, ctx, projectID)
	if count != 5 {
		t.Errorf("expected 5 builds, got %d", count)
	}

	// Verify builds 4 and 5 exist.
	for _, bo := range []int{4, 5} {
		var exists int
		err := s.DB().QueryRowContext(ctx,
			"SELECT COUNT(*) FROM builds WHERE project_id=? AND build_order=?",
			projectID, bo,
		).Scan(&exists)
		if err != nil {
			t.Fatalf("check build %d: %v", bo, err)
		}
		if exists != 1 {
			t.Errorf("build %d should exist", bo)
		}
	}
}

func TestSyncProject_BatchInsert_LargeCount(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const projectID = "proj-large"

	orders := make([]int, 100)
	for i := range orders {
		orders[i] = i + 1
	}

	mock := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{projectID}, nil
		},
		ListReportBuildsFn: func(_ context.Context, _ string) ([]int, error) {
			return orders, nil
		},
		ReadBuildStatsFn: func(_ context.Context, _ string, _ int) (storage.BuildStats, error) {
			return storage.BuildStats{}, errors.New("no stats")
		},
	}

	if err := store.SyncMetadata(ctx, mock, s, zap.NewNop()); err != nil {
		t.Fatalf("SyncMetadata: %v", err)
	}

	count := countBuilds(t, s, ctx, projectID)
	if count != 100 {
		t.Errorf("expected 100 builds, got %d", count)
	}
}

func TestSyncProject_EmptyStorageBuilds_NoError(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const projectID = "proj-empty"

	mock := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{projectID}, nil
		},
		ListReportBuildsFn: func(_ context.Context, _ string) ([]int, error) {
			return []int{}, nil
		},
	}

	if err := store.SyncMetadata(ctx, mock, s, zap.NewNop()); err != nil {
		t.Fatalf("SyncMetadata: %v", err)
	}

	count := countBuilds(t, s, ctx, projectID)
	if count != 0 {
		t.Errorf("expected 0 builds, got %d", count)
	}
}

// --- Phase 3: Batch stats tests ---

func TestSyncProject_BatchStatsSync_AllMissing(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const projectID = "proj-stats-all"

	mock := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{projectID}, nil
		},
		ListReportBuildsFn: func(_ context.Context, _ string) ([]int, error) {
			return []int{1, 2, 3}, nil
		},
		ReadBuildStatsFn: func(_ context.Context, _ string, bo int) (storage.BuildStats, error) {
			return storage.BuildStats{
				Passed:     bo * 10,
				Failed:     bo,
				Total:      bo*10 + bo,
				DurationMs: int64(bo * 1000),
			}, nil
		},
	}

	if err := store.SyncMetadata(ctx, mock, s, zap.NewNop()); err != nil {
		t.Fatalf("SyncMetadata: %v", err)
	}

	for _, bo := range []int{1, 2, 3} {
		var statTotal, statPassed, statFailed *int
		var durationMs *int64
		err := s.DB().QueryRowContext(ctx,
			"SELECT stat_total, stat_passed, stat_failed, duration_ms FROM builds WHERE project_id=? AND build_order=?",
			projectID, bo,
		).Scan(&statTotal, &statPassed, &statFailed, &durationMs)
		if err != nil {
			t.Fatalf("scan build %d: %v", bo, err)
		}
		wantTotal := bo*10 + bo
		if statTotal == nil || *statTotal != wantTotal {
			t.Errorf("build %d: want stat_total=%d, got %v", bo, wantTotal, statTotal)
		}
		wantPassed := bo * 10
		if statPassed == nil || *statPassed != wantPassed {
			t.Errorf("build %d: want stat_passed=%d, got %v", bo, wantPassed, statPassed)
		}
		wantFailed := bo
		if statFailed == nil || *statFailed != wantFailed {
			t.Errorf("build %d: want stat_failed=%d, got %v", bo, wantFailed, statFailed)
		}
		wantDuration := int64(bo * 1000)
		if durationMs == nil || *durationMs != wantDuration {
			t.Errorf("build %d: want duration_ms=%d, got %v", bo, wantDuration, durationMs)
		}
	}
}

func TestSyncProject_BatchStatsSync_PartialAlreadySynced(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const projectID = "proj-stats-partial"

	// Pre-insert project and builds, then set stats on builds 1 and 2.
	seedProjectAndBuilds(t, s, ctx, projectID, []int{1, 2, 3})
	for _, bo := range []int{1, 2} {
		_, err := s.DB().ExecContext(ctx, `
			UPDATE builds SET stat_passed=99, stat_failed=1, stat_total=100, duration_ms=5000
			WHERE project_id=? AND build_order=?`, projectID, bo)
		if err != nil {
			t.Fatalf("pre-set stats for build %d: %v", bo, err)
		}
	}

	var readCalls atomic.Int32
	mock := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{projectID}, nil
		},
		ListReportBuildsFn: func(_ context.Context, _ string) ([]int, error) {
			return []int{1, 2, 3}, nil
		},
		ReadBuildStatsFn: func(_ context.Context, _ string, bo int) (storage.BuildStats, error) {
			readCalls.Add(1)
			return storage.BuildStats{
				Passed:     7,
				Failed:     3,
				Total:      10,
				DurationMs: 2000,
			}, nil
		},
	}

	if err := store.SyncMetadata(ctx, mock, s, zap.NewNop()); err != nil {
		t.Fatalf("SyncMetadata: %v", err)
	}

	// Builds 1 and 2 should keep original stats (99 passed).
	for _, bo := range []int{1, 2} {
		var statPassed *int
		err := s.DB().QueryRowContext(ctx,
			"SELECT stat_passed FROM builds WHERE project_id=? AND build_order=?",
			projectID, bo,
		).Scan(&statPassed)
		if err != nil {
			t.Fatalf("scan build %d: %v", bo, err)
		}
		if statPassed == nil || *statPassed != 99 {
			t.Errorf("build %d: expected stat_passed=99 (unchanged), got %v", bo, statPassed)
		}
	}

	// Build 3 should have new stats.
	var statTotal *int
	err := s.DB().QueryRowContext(ctx,
		"SELECT stat_total FROM builds WHERE project_id=? AND build_order=?",
		projectID, 3,
	).Scan(&statTotal)
	if err != nil {
		t.Fatalf("scan build 3: %v", err)
	}
	if statTotal == nil || *statTotal != 10 {
		t.Errorf("build 3: expected stat_total=10, got %v", statTotal)
	}

	// ReadBuildStatsFn should have been called only once (for build 3).
	if got := readCalls.Load(); got != 1 {
		t.Errorf("expected ReadBuildStatsFn called 1 time, got %d", got)
	}
}

func TestSyncProject_BatchStatsSync_StorageError_LeavesNullForRetry(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const projectID = "proj-stats-err"

	mock := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{projectID}, nil
		},
		ListReportBuildsFn: func(_ context.Context, _ string) ([]int, error) {
			return []int{1, 2}, nil
		},
		ReadBuildStatsFn: func(_ context.Context, _ string, bo int) (storage.BuildStats, error) {
			if bo == 2 {
				return storage.BuildStats{}, errors.New("disk failure")
			}
			return storage.BuildStats{Passed: 5, Total: 5, DurationMs: 100}, nil
		},
	}

	if err := store.SyncMetadata(ctx, mock, s, zap.NewNop()); err != nil {
		t.Fatalf("SyncMetadata: %v", err)
	}

	// Build 1 should have stats.
	var statTotal1 *int
	err := s.DB().QueryRowContext(ctx,
		"SELECT stat_total FROM builds WHERE project_id=? AND build_order=?",
		projectID, 1,
	).Scan(&statTotal1)
	if err != nil {
		t.Fatalf("scan build 1: %v", err)
	}
	if statTotal1 == nil || *statTotal1 != 5 {
		t.Errorf("build 1: expected stat_total=5, got %v", statTotal1)
	}

	// Build 2 should have NULL stat_total.
	var statTotal2 *int
	err = s.DB().QueryRowContext(ctx,
		"SELECT stat_total FROM builds WHERE project_id=? AND build_order=?",
		projectID, 2,
	).Scan(&statTotal2)
	if err != nil {
		t.Fatalf("scan build 2: %v", err)
	}
	if statTotal2 != nil {
		t.Errorf("build 2: expected stat_total=NULL, got %d", *statTotal2)
	}
}

func TestSyncProject_BatchStatsSync_AllStorageErrors_NoUpdates(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const projectID = "proj-stats-allfail"

	mock := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{projectID}, nil
		},
		ListReportBuildsFn: func(_ context.Context, _ string) ([]int, error) {
			return []int{1, 2}, nil
		},
		ReadBuildStatsFn: func(_ context.Context, _ string, _ int) (storage.BuildStats, error) {
			return storage.BuildStats{}, errors.New("all storage down")
		},
	}

	if err := store.SyncMetadata(ctx, mock, s, zap.NewNop()); err != nil {
		t.Fatalf("SyncMetadata: %v", err)
	}

	for _, bo := range []int{1, 2} {
		var statTotal *int
		err := s.DB().QueryRowContext(ctx,
			"SELECT stat_total FROM builds WHERE project_id=? AND build_order=?",
			projectID, bo,
		).Scan(&statTotal)
		if err != nil {
			t.Fatalf("scan build %d: %v", bo, err)
		}
		if statTotal != nil {
			t.Errorf("build %d: expected stat_total=NULL, got %d", bo, *statTotal)
		}
	}
}

func TestSyncProject_SingleBuild_WorksCorrectly(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	const projectID = "proj-single"

	mock := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{projectID}, nil
		},
		ListReportBuildsFn: func(_ context.Context, _ string) ([]int, error) {
			return []int{1}, nil
		},
		ReadBuildStatsFn: func(_ context.Context, _ string, _ int) (storage.BuildStats, error) {
			return storage.BuildStats{
				Passed:     8,
				Failed:     2,
				Broken:     1,
				Skipped:    3,
				Unknown:    0,
				Total:      14,
				DurationMs: 3500,
			}, nil
		},
	}

	if err := store.SyncMetadata(ctx, mock, s, zap.NewNop()); err != nil {
		t.Fatalf("SyncMetadata: %v", err)
	}

	count := countBuilds(t, s, ctx, projectID)
	if count != 1 {
		t.Fatalf("expected 1 build, got %d", count)
	}

	var statTotal, statPassed, statBroken, statSkipped *int
	var durationMs *int64
	err := s.DB().QueryRowContext(ctx,
		"SELECT stat_total, stat_passed, stat_broken, stat_skipped, duration_ms FROM builds WHERE project_id=? AND build_order=?",
		projectID, 1,
	).Scan(&statTotal, &statPassed, &statBroken, &statSkipped, &durationMs)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if statTotal == nil || *statTotal != 14 {
		t.Errorf("want stat_total=14, got %v", statTotal)
	}
	if statPassed == nil || *statPassed != 8 {
		t.Errorf("want stat_passed=8, got %v", statPassed)
	}
	if statBroken == nil || *statBroken != 1 {
		t.Errorf("want stat_broken=1, got %v", statBroken)
	}
	if statSkipped == nil || *statSkipped != 3 {
		t.Errorf("want stat_skipped=3, got %v", statSkipped)
	}
	if durationMs == nil || *durationMs != 3500 {
		t.Errorf("want duration_ms=3500, got %v", durationMs)
	}
}

// --- Test helpers ---

func seedProjectAndBuilds(t *testing.T, s *store.SQLiteStore, ctx context.Context, projectID string, buildOrders []int) {
	t.Helper()
	_, err := s.DB().ExecContext(ctx, "INSERT OR IGNORE INTO projects(id) VALUES(?)", projectID)
	if err != nil {
		t.Fatalf("seed project %q: %v", projectID, err)
	}
	for _, bo := range buildOrders {
		_, err := s.DB().ExecContext(ctx,
			"INSERT OR IGNORE INTO builds(project_id, build_order) VALUES(?, ?)",
			projectID, bo)
		if err != nil {
			t.Fatalf("seed build %d: %v", bo, err)
		}
	}
}

func countBuilds(t *testing.T, s *store.SQLiteStore, ctx context.Context, projectID string) int {
	t.Helper()
	var count int
	err := s.DB().QueryRowContext(ctx,
		"SELECT COUNT(*) FROM builds WHERE project_id=?", projectID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("count builds: %v", err)
	}
	return count
}
