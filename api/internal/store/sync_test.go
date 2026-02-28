package store_test

import (
	"context"
	"errors"
	"go.uber.org/zap"
	"testing"

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
