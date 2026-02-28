package store

import (
	"context"
	"fmt"
	"sync"

	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

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
// Uses batch operations to minimize DB round-trips:
//  1. List storage builds and diff against DB in Go memory
//  2. Batch INSERT missing builds in a single transaction
//  3. Query builds with NULL stat_total
//  4. Bounded concurrent storage reads + single UPDATE transaction
func syncProject(ctx context.Context, st storage.Store, s *SQLiteStore, projectID string, logger *zap.Logger) error {
	ps := NewProjectStore(s, logger)
	bs := NewBuildStore(s, logger)

	if err := ps.insertOrIgnore(ctx, projectID); err != nil {
		return err
	}

	storageOrders, err := st.ListReportBuilds(ctx, projectID)
	if err != nil {
		return fmt.Errorf("list report builds for project %q: %w", projectID, err)
	}
	if len(storageOrders) == 0 {
		return nil
	}

	// Phase 1: Diff builds in Go memory.
	existing, err := bs.existingBuildOrders(ctx, projectID)
	if err != nil {
		return err
	}

	var missing []int
	for _, bo := range storageOrders {
		if _, ok := existing[bo]; !ok {
			missing = append(missing, bo)
		}
	}

	// Phase 2: Batch INSERT missing builds.
	if err := bs.insertMissingBuilds(ctx, projectID, missing); err != nil {
		return err
	}

	// Phase 3: Batch stats sync.
	needStats, err := bs.buildsWithMissingStats(ctx, projectID)
	if err != nil {
		return err
	}
	if err := bs.batchSyncStats(ctx, projectID, needStats, st); err != nil {
		return err
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

// existingBuildOrders returns the set of build orders already in the DB for a project.
// Single query using the idx_builds_project index.
func (bs *BuildStore) existingBuildOrders(ctx context.Context, projectID string) (map[int]struct{}, error) {
	rows, err := bs.db.QueryContext(ctx,
		"SELECT build_order FROM builds WHERE project_id = ?", projectID)
	if err != nil {
		return nil, fmt.Errorf("existing build orders for %q: %w", projectID, err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[int]struct{})
	for rows.Next() {
		var bo int
		if err := rows.Scan(&bo); err != nil {
			return nil, fmt.Errorf("scan build order: %w", err)
		}
		result[bo] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate build orders: %w", err)
	}
	return result, nil
}

// insertMissingBuilds inserts builds in a single transaction using a prepared statement.
// Returns nil for empty slice. Uses INSERT OR IGNORE for idempotency.
func (bs *BuildStore) insertMissingBuilds(ctx context.Context, projectID string, missing []int) error {
	if len(missing) == 0 {
		return nil
	}

	tx, err := bs.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx,
		"INSERT OR IGNORE INTO builds(project_id, build_order) VALUES(?, ?)")
	if err != nil {
		return fmt.Errorf("prepare insert build: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for _, bo := range missing {
		if _, err := stmt.ExecContext(ctx, projectID, bo); err != nil {
			return fmt.Errorf("insert build %d for project %q: %w", bo, projectID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit insert builds: %w", err)
	}
	return nil
}

// buildsWithMissingStats returns build orders where stat_total is NULL.
func (bs *BuildStore) buildsWithMissingStats(ctx context.Context, projectID string) ([]int, error) {
	rows, err := bs.db.QueryContext(ctx,
		"SELECT build_order FROM builds WHERE project_id = ? AND stat_total IS NULL", projectID)
	if err != nil {
		return nil, fmt.Errorf("builds with missing stats for %q: %w", projectID, err)
	}
	defer func() { _ = rows.Close() }()

	var orders []int
	for rows.Next() {
		var bo int
		if err := rows.Scan(&bo); err != nil {
			return nil, fmt.Errorf("scan build order: %w", err)
		}
		orders = append(orders, bo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate missing stats: %w", err)
	}
	return orders, nil
}

// statsResult pairs a build order with its successfully-read stats.
type statsResult struct {
	buildOrder int
	stats      storage.BuildStats
}

// batchSyncStats reads stats from storage concurrently (bounded to 10 goroutines)
// and writes all successful results in a single DB transaction.
// Failed reads are logged and skipped — stat_total stays NULL for retry on next startup.
func (bs *BuildStore) batchSyncStats(ctx context.Context, projectID string, buildOrders []int, st storage.Store) error {
	if len(buildOrders) == 0 {
		return nil
	}

	var mu sync.Mutex
	var results []statsResult

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(10)

	for _, bo := range buildOrders {
		g.Go(func() error {
			stats, err := st.ReadBuildStats(gctx, projectID, bo)
			if err != nil {
				bs.logger.Info("SyncMetadata: stats unavailable (will retry next startup)",
					zap.String("project_id", projectID), zap.Int("build_order", bo), zap.Error(err))
				return nil // non-fatal: leave stat_total NULL
			}
			mu.Lock()
			results = append(results, statsResult{buildOrder: bo, stats: stats})
			mu.Unlock()
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("batch read stats for %q: %w", projectID, err)
	}

	if len(results) == 0 {
		return nil
	}

	// Single transaction for all successful stats updates.
	tx, err := bs.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `
		UPDATE builds
		SET stat_passed=?, stat_failed=?, stat_broken=?,
		    stat_skipped=?, stat_unknown=?, stat_total=?, duration_ms=?
		WHERE project_id=? AND build_order=?`)
	if err != nil {
		return fmt.Errorf("prepare update stats: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for i := range results {
		r := &results[i]
		if _, err := stmt.ExecContext(ctx,
			r.stats.Passed, r.stats.Failed, r.stats.Broken,
			r.stats.Skipped, r.stats.Unknown, r.stats.Total, r.stats.DurationMs,
			projectID, r.buildOrder,
		); err != nil {
			return fmt.Errorf("update stats for build %d: %w", r.buildOrder, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit stats update: %w", err)
	}
	return nil
}
