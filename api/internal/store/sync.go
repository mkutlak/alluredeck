package store

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/storage"
)

// SyncMetadata scans projects via the storage store and imports any projects
// and builds not yet recorded in the database.
// Safe to call on every startup — INSERT OR IGNORE ensures already-imported rows are skipped.
func SyncMetadata(ctx context.Context, st storage.Store, s *SQLiteStore, logger *zap.Logger) error {
	projects, err := st.ListProjects(ctx)
	if err != nil {
		// Non-fatal if projects directory doesn't exist yet
		logger.Info("SyncMetadata: could not list projects (non-fatal)", zap.Error(err))
		return nil
	}

	for _, projectID := range projects {
		if err := syncProject(ctx, st, s, projectID, logger); err != nil {
			logger.Error("SyncMetadata: skipping project", zap.String("project_id", projectID), zap.Error(err))
		}
	}
	return nil
}

// syncProject inserts or updates a single project and all its builds in the database.
func syncProject(ctx context.Context, st storage.Store, s *SQLiteStore, projectID string, logger *zap.Logger) error {
	ps := NewProjectStore(s, logger)
	bs := NewBuildStore(s, logger)

	if err := ps.insertOrIgnore(ctx, projectID); err != nil {
		return err
	}

	buildOrders, err := st.ListReportBuilds(ctx, projectID)
	if err != nil {
		// No reports yet — project has no history
		return fmt.Errorf("list report builds for project %q: %w", projectID, err)
	}

	for _, buildOrder := range buildOrders {
		if err := bs.insertOrIgnore(ctx, projectID, buildOrder); err != nil {
			logger.Error("SyncMetadata: skipping build",
				zap.String("project_id", projectID), zap.Int("build_order", buildOrder), zap.Error(err))
			continue
		}

		if err := bs.syncStatsIfMissing(ctx, projectID, buildOrder, st); err != nil {
			logger.Warn("SyncMetadata: stats sync failed",
				zap.String("project_id", projectID), zap.Int("build_order", buildOrder), zap.Error(err))
		}
	}
	return nil
}

// insertOrIgnore inserts a project row, silently ignoring duplicate-key errors.
func (ps *ProjectStore) insertOrIgnore(ctx context.Context, id string) error {
	_, err := ps.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO projects(id) VALUES(?)", id)
	if err != nil {
		return fmt.Errorf("insert project %q: %w", id, err)
	}
	return nil
}

// insertOrIgnore inserts a build row, silently ignoring duplicate-key errors.
func (bs *BuildStore) insertOrIgnore(ctx context.Context, projectID string, buildOrder int) error {
	_, err := bs.db.ExecContext(ctx,
		"INSERT OR IGNORE INTO builds(project_id, build_order) VALUES(?, ?)",
		projectID, buildOrder)
	if err != nil {
		return fmt.Errorf("insert build %d for project %q: %w", buildOrder, projectID, err)
	}
	return nil
}

// syncStatsIfMissing reads widget files and updates stats only when the DB row has no stat_total yet.
func (bs *BuildStore) syncStatsIfMissing(ctx context.Context, projectID string, buildOrder int, st storage.Store) error {
	var total *int
	err := bs.db.QueryRowContext(ctx,
		"SELECT stat_total FROM builds WHERE project_id=? AND build_order=?",
		projectID, buildOrder,
	).Scan(&total)
	if err != nil {
		return fmt.Errorf("scan build stats for project %q build %d: %w", projectID, buildOrder, err)
	}
	if total != nil {
		return nil // already has stats
	}

	// Read stats from storage — skip update if unavailable so stat_total stays NULL
	// and the row is retried on next startup.
	storageStats, err := st.ReadBuildStats(ctx, projectID, buildOrder)
	if err != nil {
		bs.logger.Info("SyncMetadata: stats unavailable (will retry next startup)",
			zap.String("project_id", projectID), zap.Int("build_order", buildOrder), zap.Error(err))
		return nil
	}

	_, err = bs.db.ExecContext(ctx, `
		UPDATE builds
		SET stat_passed=?, stat_failed=?, stat_broken=?,
		    stat_skipped=?, stat_unknown=?, stat_total=?, duration_ms=?
		WHERE project_id=? AND build_order=?`,
		storageStats.Passed, storageStats.Failed, storageStats.Broken,
		storageStats.Skipped, storageStats.Unknown, storageStats.Total, storageStats.DurationMs,
		projectID, buildOrder)
	if err != nil {
		return fmt.Errorf("set stats for project %q build %d: %w", projectID, buildOrder, err)
	}
	return nil
}
