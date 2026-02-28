package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"
)

// ErrBuildNotFound is returned when a build does not exist.
var ErrBuildNotFound = errors.New("build not found")

// Build holds build metadata from the database.
type Build struct {
	ID          int64
	ProjectID   string
	BuildOrder  int
	CreatedAt   time.Time
	StatPassed  *int
	StatFailed  *int
	StatBroken  *int
	StatSkipped *int
	StatUnknown *int
	StatTotal   *int
	DurationMs  *int64
	IsLatest    bool
}

// BuildStats holds the statistics for a completed build.
type BuildStats struct {
	Passed     int
	Failed     int
	Broken     int
	Skipped    int
	Unknown    int
	Total      int
	DurationMs int64
}

// BuildStore provides operations on the builds table.
type BuildStore struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewBuildStore creates a BuildStore backed by the given SQLiteStore.
func NewBuildStore(s *SQLiteStore, logger *zap.Logger) *BuildStore {
	return &BuildStore{db: s.db, logger: logger}
}

// NextBuildOrder atomically determines the next build order for a project (MAX+1, min 1).
func (bs *BuildStore) NextBuildOrder(ctx context.Context, projectID string) (int, error) {
	var maxOrder sql.NullInt64
	err := bs.db.QueryRowContext(ctx,
		"SELECT MAX(build_order) FROM builds WHERE project_id = ?", projectID,
	).Scan(&maxOrder)
	if err != nil {
		return 0, fmt.Errorf("next build order: %w", err)
	}
	if maxOrder.Valid {
		return int(maxOrder.Int64) + 1, nil
	}
	return 1, nil
}

// InsertBuild records a new build with the given order.
func (bs *BuildStore) InsertBuild(ctx context.Context, projectID string, buildOrder int) error {
	_, err := bs.db.ExecContext(ctx,
		"INSERT INTO builds(project_id, build_order) VALUES(?, ?)",
		projectID, buildOrder)
	if err != nil {
		return fmt.Errorf("insert build: %w", err)
	}
	return nil
}

// UpdateBuildStats updates statistics for the given build.
func (bs *BuildStore) UpdateBuildStats(ctx context.Context, projectID string, buildOrder int, stats BuildStats) error {
	res, err := bs.db.ExecContext(ctx, `
		UPDATE builds
		SET stat_passed=?, stat_failed=?, stat_broken=?,
		    stat_skipped=?, stat_unknown=?, stat_total=?,
		    duration_ms=?
		WHERE project_id=? AND build_order=?`,
		stats.Passed, stats.Failed, stats.Broken,
		stats.Skipped, stats.Unknown, stats.Total,
		stats.DurationMs,
		projectID, buildOrder)
	if err != nil {
		return fmt.Errorf("update build stats: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: project=%s build=%d", ErrBuildNotFound, projectID, buildOrder)
	}
	return nil
}

// ListBuilds returns all builds for a project in descending build_order.
func (bs *BuildStore) ListBuilds(ctx context.Context, projectID string) ([]Build, error) {
	rows, err := bs.db.QueryContext(ctx, `
		SELECT id, project_id, build_order, created_at,
		       stat_passed, stat_failed, stat_broken, stat_skipped, stat_unknown, stat_total,
		       duration_ms, is_latest
		FROM builds
		WHERE project_id = ?
		ORDER BY build_order DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list builds: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var builds []Build
	for rows.Next() {
		var b Build
		var createdAt string
		var passed, failed, broken, skipped, unknown, total sql.NullInt64
		var durationMs sql.NullInt64
		var isLatest int
		if err := rows.Scan(
			&b.ID, &b.ProjectID, &b.BuildOrder, &createdAt,
			&passed, &failed, &broken, &skipped, &unknown, &total,
			&durationMs, &isLatest,
		); err != nil {
			return nil, fmt.Errorf("scan build: %w", err)
		}
		if t, err := time.Parse("2006-01-02T15:04:05Z", createdAt); err != nil {
			bs.logger.Warn("invalid created_at for build",
				zap.String("created_at", createdAt),
				zap.String("project_id", projectID),
				zap.Int("build_order", b.BuildOrder),
				zap.Error(err))
		} else {
			b.CreatedAt = t
		}
		b.IsLatest = isLatest == 1
		if passed.Valid {
			v := int(passed.Int64)
			b.StatPassed = &v
		}
		if failed.Valid {
			v := int(failed.Int64)
			b.StatFailed = &v
		}
		if broken.Valid {
			v := int(broken.Int64)
			b.StatBroken = &v
		}
		if skipped.Valid {
			v := int(skipped.Int64)
			b.StatSkipped = &v
		}
		if unknown.Valid {
			v := int(unknown.Int64)
			b.StatUnknown = &v
		}
		if total.Valid {
			v := int(total.Int64)
			b.StatTotal = &v
		}
		if durationMs.Valid {
			b.DurationMs = &durationMs.Int64
		}
		builds = append(builds, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate build rows: %w", err)
	}
	return builds, nil
}

// ListBuildsPaginated returns a page of builds for a project in descending build_order, plus the total count.
func (bs *BuildStore) ListBuildsPaginated(ctx context.Context, projectID string, page, perPage int) ([]Build, int, error) {
	var totalCount int
	if err := bs.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM builds WHERE project_id = ?", projectID,
	).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("count builds: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := bs.db.QueryContext(ctx, `
		SELECT id, project_id, build_order, created_at,
		       stat_passed, stat_failed, stat_broken, stat_skipped, stat_unknown, stat_total,
		       duration_ms, is_latest
		FROM builds
		WHERE project_id = ?
		ORDER BY build_order DESC
		LIMIT ? OFFSET ?`, projectID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list builds paginated: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var builds []Build
	for rows.Next() {
		var b Build
		var createdAt string
		var passed, failed, broken, skipped, unknown, total sql.NullInt64
		var durationMs sql.NullInt64
		var isLatest int
		if err := rows.Scan(
			&b.ID, &b.ProjectID, &b.BuildOrder, &createdAt,
			&passed, &failed, &broken, &skipped, &unknown, &total,
			&durationMs, &isLatest,
		); err != nil {
			return nil, 0, fmt.Errorf("scan build: %w", err)
		}
		if t, err := time.Parse("2006-01-02T15:04:05Z", createdAt); err != nil {
			bs.logger.Warn("invalid created_at for build",
				zap.String("created_at", createdAt),
				zap.String("project_id", projectID),
				zap.Int("build_order", b.BuildOrder),
				zap.Error(err))
		} else {
			b.CreatedAt = t
		}
		b.IsLatest = isLatest == 1
		if passed.Valid {
			v := int(passed.Int64)
			b.StatPassed = &v
		}
		if failed.Valid {
			v := int(failed.Int64)
			b.StatFailed = &v
		}
		if broken.Valid {
			v := int(broken.Int64)
			b.StatBroken = &v
		}
		if skipped.Valid {
			v := int(skipped.Int64)
			b.StatSkipped = &v
		}
		if unknown.Valid {
			v := int(unknown.Int64)
			b.StatUnknown = &v
		}
		if total.Valid {
			v := int(total.Int64)
			b.StatTotal = &v
		}
		if durationMs.Valid {
			b.DurationMs = &durationMs.Int64
		}
		builds = append(builds, b)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate build rows: %w", err)
	}
	return builds, totalCount, nil
}

// PruneBuilds removes the oldest builds exceeding `keep` count.
// Returns the build_orders of removed builds (for filesystem cleanup).
func (bs *BuildStore) PruneBuilds(ctx context.Context, projectID string, keep int) ([]int, error) {
	if keep <= 0 {
		return nil, nil
	}

	rows, err := bs.db.QueryContext(ctx, `
		SELECT build_order FROM builds
		WHERE project_id = ?
		ORDER BY build_order DESC
		LIMIT -1 OFFSET ?`, projectID, keep)
	if err != nil {
		return nil, fmt.Errorf("prune builds query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var toRemove []int
	for rows.Next() {
		var bo int
		if err := rows.Scan(&bo); err != nil {
			return nil, fmt.Errorf("scan build_order: %w", err)
		}
		toRemove = append(toRemove, bo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate prune rows: %w", err)
	}

	for _, bo := range toRemove {
		if _, err := bs.db.ExecContext(ctx,
			"DELETE FROM builds WHERE project_id=? AND build_order=?",
			projectID, bo); err != nil {
			return nil, fmt.Errorf("delete build %d: %w", bo, err)
		}
	}
	return toRemove, nil
}

// SetLatest marks the given build_order as latest and clears the flag on all others.
func (bs *BuildStore) SetLatest(ctx context.Context, projectID string, buildOrder int) error {
	tx, err := bs.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // tx.Rollback after Commit always returns ErrTxDone

	if _, err := tx.ExecContext(ctx,
		"UPDATE builds SET is_latest=0 WHERE project_id=?", projectID); err != nil {
		return fmt.Errorf("clear is_latest: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE builds SET is_latest=1 WHERE project_id=? AND build_order=?",
		projectID, buildOrder); err != nil {
		return fmt.Errorf("set is_latest: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit build prune transaction: %w", err)
	}
	return nil
}

// DeleteBuild removes a single build by build_order from the database.
func (bs *BuildStore) DeleteBuild(ctx context.Context, projectID string, buildOrder int) error {
	res, err := bs.db.ExecContext(ctx,
		"DELETE FROM builds WHERE project_id=? AND build_order=?",
		projectID, buildOrder)
	if err != nil {
		return fmt.Errorf("delete build: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Not in DB is non-fatal — build may have been created before SQLite was introduced.
		return nil
	}
	return nil
}
