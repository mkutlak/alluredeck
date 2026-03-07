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
	ID             int64
	ProjectID      string
	BuildOrder     int
	CreatedAt      time.Time
	StatPassed     *int
	StatFailed     *int
	StatBroken     *int
	StatSkipped    *int
	StatUnknown    *int
	StatTotal      *int
	DurationMs     *int64
	FlakyCount     *int
	RetriedCount   *int
	NewFailedCount *int
	NewPassedCount *int
	IsLatest       bool
	CIProvider     *string
	CIBuildURL     *string
	CIBranch       *string
	CICommitSHA    *string
}

// CIMetadata holds CI context captured at report generation time.
type CIMetadata struct {
	Provider  string
	BuildURL  string
	Branch    string
	CommitSHA string
}

// DashboardProject holds a project's latest build + sparkline data.
type DashboardProject struct {
	ProjectID string
	CreatedAt time.Time
	Tags      []string
	Latest    *Build
	Sparkline []SparklinePoint
}

// SparklinePoint is a single pass-rate data point for the sparkline.
type SparklinePoint struct {
	BuildOrder int
	PassRate   float64 // 0–100
	CreatedAt  time.Time
}

// BuildStats holds the statistics for a completed build.
type BuildStats struct {
	Passed         int
	Failed         int
	Broken         int
	Skipped        int
	Unknown        int
	Total          int
	DurationMs     int64
	FlakyCount     int
	RetriedCount   int
	NewFailedCount int
	NewPassedCount int
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
		    duration_ms=?,
		    flaky_count=?, retried_count=?, new_failed_count=?, new_passed_count=?
		WHERE project_id=? AND build_order=?`,
		stats.Passed, stats.Failed, stats.Broken,
		stats.Skipped, stats.Unknown, stats.Total,
		stats.DurationMs,
		stats.FlakyCount, stats.RetriedCount, stats.NewFailedCount, stats.NewPassedCount,
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

// UpdateBuildCIMetadata stores CI context for the given build.
// Only non-empty fields in ciMeta are written; empty strings are stored as NULL.
func (bs *BuildStore) UpdateBuildCIMetadata(ctx context.Context, projectID string, buildOrder int, ciMeta CIMetadata) error {
	nullStr := func(s string) sql.NullString {
		return sql.NullString{String: s, Valid: s != ""}
	}
	res, err := bs.db.ExecContext(ctx, `
		UPDATE builds
		SET ci_provider=?, ci_build_url=?, ci_branch=?, ci_commit_sha=?
		WHERE project_id=? AND build_order=?`,
		nullStr(ciMeta.Provider), nullStr(ciMeta.BuildURL),
		nullStr(ciMeta.Branch), nullStr(ciMeta.CommitSHA),
		projectID, buildOrder)
	if err != nil {
		return fmt.Errorf("update build ci metadata: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: project=%s build=%d", ErrBuildNotFound, projectID, buildOrder)
	}
	return nil
}

// scanBuild scans a single build row from the standard column set.
func (bs *BuildStore) scanBuild(row *sql.Row) (Build, error) {
	var b Build
	var createdAt string
	var passed, failed, broken, skipped, unknown, total sql.NullInt64
	var durationMs sql.NullInt64
	var isLatest int
	var flakyCount, retriedCount, newFailedCount, newPassedCount int
	var ciProvider, ciBuildURL, ciBranch, ciCommitSHA sql.NullString

	if err := row.Scan(
		&b.ID, &b.ProjectID, &b.BuildOrder, &createdAt,
		&passed, &failed, &broken, &skipped, &unknown, &total,
		&durationMs, &isLatest,
		&flakyCount, &retriedCount, &newFailedCount, &newPassedCount,
		&ciProvider, &ciBuildURL, &ciBranch, &ciCommitSHA,
	); err != nil {
		return Build{}, err
	}

	if t, err := time.Parse("2006-01-02T15:04:05Z", createdAt); err == nil {
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
	b.FlakyCount = &flakyCount
	b.RetriedCount = &retriedCount
	b.NewFailedCount = &newFailedCount
	b.NewPassedCount = &newPassedCount
	if ciProvider.Valid {
		b.CIProvider = &ciProvider.String
	}
	if ciBuildURL.Valid {
		b.CIBuildURL = &ciBuildURL.String
	}
	if ciBranch.Valid {
		b.CIBranch = &ciBranch.String
	}
	if ciCommitSHA.Valid {
		b.CICommitSHA = &ciCommitSHA.String
	}
	return b, nil
}

const buildSelectColumns = `
	SELECT id, project_id, build_order, created_at,
	       stat_passed, stat_failed, stat_broken, stat_skipped, stat_unknown, stat_total,
	       duration_ms, is_latest,
	       flaky_count, retried_count, new_failed_count, new_passed_count,
	       ci_provider, ci_build_url, ci_branch, ci_commit_sha
	FROM builds`

// GetBuildByOrder returns a single build for project_id + build_order.
func (bs *BuildStore) GetBuildByOrder(ctx context.Context, projectID string, buildOrder int) (Build, error) {
	row := bs.db.QueryRowContext(ctx, buildSelectColumns+`
		WHERE project_id = ? AND build_order = ?`, projectID, buildOrder)
	b, err := bs.scanBuild(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Build{}, fmt.Errorf("%w: project=%s build=%d", ErrBuildNotFound, projectID, buildOrder)
		}
		return Build{}, fmt.Errorf("get build by order: %w", err)
	}
	return b, nil
}

// GetPreviousBuild returns the build immediately before the given build_order.
func (bs *BuildStore) GetPreviousBuild(ctx context.Context, projectID string, buildOrder int) (Build, error) {
	row := bs.db.QueryRowContext(ctx, buildSelectColumns+`
		WHERE project_id = ? AND build_order < ?
		ORDER BY build_order DESC
		LIMIT 1`, projectID, buildOrder)
	b, err := bs.scanBuild(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Build{}, fmt.Errorf("%w: no previous build for project=%s build=%d", ErrBuildNotFound, projectID, buildOrder)
		}
		return Build{}, fmt.Errorf("get previous build: %w", err)
	}
	return b, nil
}

// GetLatestBuild returns the build marked is_latest=1 for a project.
func (bs *BuildStore) GetLatestBuild(ctx context.Context, projectID string) (Build, error) {
	row := bs.db.QueryRowContext(ctx, buildSelectColumns+`
		WHERE project_id = ? AND is_latest = 1`, projectID)
	b, err := bs.scanBuild(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Build{}, fmt.Errorf("%w: no latest build for project=%s", ErrBuildNotFound, projectID)
		}
		return Build{}, fmt.Errorf("get latest build: %w", err)
	}
	return b, nil
}

// ListBuilds returns all builds for a project in descending build_order.
func (bs *BuildStore) ListBuilds(ctx context.Context, projectID string) ([]Build, error) {
	rows, err := bs.db.QueryContext(ctx, `
		SELECT id, project_id, build_order, created_at,
		       stat_passed, stat_failed, stat_broken, stat_skipped, stat_unknown, stat_total,
		       duration_ms, is_latest,
		       flaky_count, retried_count, new_failed_count, new_passed_count,
		       ci_provider, ci_build_url, ci_branch, ci_commit_sha
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
		var flakyCount, retriedCount, newFailedCount, newPassedCount int
		var ciProvider, ciBuildURL, ciBranch, ciCommitSHA sql.NullString
		if err := rows.Scan(
			&b.ID, &b.ProjectID, &b.BuildOrder, &createdAt,
			&passed, &failed, &broken, &skipped, &unknown, &total,
			&durationMs, &isLatest,
			&flakyCount, &retriedCount, &newFailedCount, &newPassedCount,
			&ciProvider, &ciBuildURL, &ciBranch, &ciCommitSHA,
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
		b.FlakyCount = &flakyCount
		b.RetriedCount = &retriedCount
		b.NewFailedCount = &newFailedCount
		b.NewPassedCount = &newPassedCount
		if ciProvider.Valid {
			b.CIProvider = &ciProvider.String
		}
		if ciBuildURL.Valid {
			b.CIBuildURL = &ciBuildURL.String
		}
		if ciBranch.Valid {
			b.CIBranch = &ciBranch.String
		}
		if ciCommitSHA.Valid {
			b.CICommitSHA = &ciCommitSHA.String
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
		       duration_ms, is_latest,
		       flaky_count, retried_count, new_failed_count, new_passed_count,
		       ci_provider, ci_build_url, ci_branch, ci_commit_sha
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
		var flakyCount, retriedCount, newFailedCount, newPassedCount int
		var ciProvider, ciBuildURL, ciBranch, ciCommitSHA sql.NullString
		if err := rows.Scan(
			&b.ID, &b.ProjectID, &b.BuildOrder, &createdAt,
			&passed, &failed, &broken, &skipped, &unknown, &total,
			&durationMs, &isLatest,
			&flakyCount, &retriedCount, &newFailedCount, &newPassedCount,
			&ciProvider, &ciBuildURL, &ciBranch, &ciCommitSHA,
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
		b.FlakyCount = &flakyCount
		b.RetriedCount = &retriedCount
		b.NewFailedCount = &newFailedCount
		b.NewPassedCount = &newPassedCount
		if ciProvider.Valid {
			b.CIProvider = &ciProvider.String
		}
		if ciBuildURL.Valid {
			b.CIBuildURL = &ciBuildURL.String
		}
		if ciBranch.Valid {
			b.CIBranch = &ciBranch.String
		}
		if ciCommitSHA.Valid {
			b.CICommitSHA = &ciCommitSHA.String
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
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx,
		"UPDATE builds SET is_latest=0 WHERE project_id=? AND is_latest=1", projectID); err != nil {
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

// DeleteAllBuilds removes all builds for a project from the database.
func (bs *BuildStore) DeleteAllBuilds(ctx context.Context, projectID string) error {
	_, err := bs.db.ExecContext(ctx,
		"DELETE FROM builds WHERE project_id=?", projectID)
	if err != nil {
		return fmt.Errorf("delete all builds for project %q: %w", projectID, err)
	}
	return nil
}

// GetDashboardData returns all projects with their latest build and sparkline data.
// sparklineDepth controls how many recent builds to include per project (e.g. 10).
// When tag is non-empty, only projects containing that tag are returned.
func (bs *BuildStore) GetDashboardData(ctx context.Context, sparklineDepth int, tag string) ([]DashboardProject, error) {
	// Query 1: all projects LEFT JOIN their latest build.
	var query string
	var args []any
	if tag != "" {
		query = `
		SELECT p.id, p.created_at, p.tags,
		       b.id, b.project_id, b.build_order, b.created_at,
		       b.stat_passed, b.stat_failed, b.stat_broken, b.stat_skipped, b.stat_unknown, b.stat_total,
		       b.duration_ms, b.is_latest,
		       b.flaky_count, b.retried_count, b.new_failed_count, b.new_passed_count,
		       b.ci_provider, b.ci_build_url, b.ci_branch, b.ci_commit_sha
		FROM projects p
		LEFT JOIN builds b ON b.project_id = p.id AND b.is_latest = 1
		WHERE EXISTS (SELECT 1 FROM json_each(p.tags) WHERE json_each.value = ?)
		ORDER BY p.id`
		args = []any{tag}
	} else {
		query = `
		SELECT p.id, p.created_at, p.tags,
		       b.id, b.project_id, b.build_order, b.created_at,
		       b.stat_passed, b.stat_failed, b.stat_broken, b.stat_skipped, b.stat_unknown, b.stat_total,
		       b.duration_ms, b.is_latest,
		       b.flaky_count, b.retried_count, b.new_failed_count, b.new_passed_count,
		       b.ci_provider, b.ci_build_url, b.ci_branch, b.ci_commit_sha
		FROM projects p
		LEFT JOIN builds b ON b.project_id = p.id AND b.is_latest = 1
		ORDER BY p.id`
	}
	rows, err := bs.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("dashboard projects query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	projectMap := make(map[string]*DashboardProject)
	var orderedIDs []string

	for rows.Next() {
		var projID, projCreatedAt, projTags string
		// Build fields — all nullable since LEFT JOIN may produce NULLs.
		var buildID, buildOrder sql.NullInt64
		var buildProjID, buildCreatedAt sql.NullString
		var passed, failed, broken, skipped, unknown, total sql.NullInt64
		var durationMs sql.NullInt64
		var isLatest sql.NullInt64
		var flakyCount, retriedCount, newFailedCount, newPassedCount sql.NullInt64
		var ciProvider, ciBuildURL, ciBranch, ciCommitSHA sql.NullString

		if err := rows.Scan(
			&projID, &projCreatedAt, &projTags,
			&buildID, &buildProjID, &buildOrder, &buildCreatedAt,
			&passed, &failed, &broken, &skipped, &unknown, &total,
			&durationMs, &isLatest,
			&flakyCount, &retriedCount, &newFailedCount, &newPassedCount,
			&ciProvider, &ciBuildURL, &ciBranch, &ciCommitSHA,
		); err != nil {
			return nil, fmt.Errorf("scan dashboard row: %w", err)
		}

		dp := &DashboardProject{ProjectID: projID}
		if t, err := time.Parse("2006-01-02T15:04:05Z", projCreatedAt); err == nil {
			dp.CreatedAt = t
		}
		dp.Tags = parseTags(projTags)

		if buildID.Valid {
			b := &Build{
				ID:         buildID.Int64,
				ProjectID:  buildProjID.String,
				BuildOrder: int(buildOrder.Int64),
				IsLatest:   isLatest.Int64 == 1,
			}
			if buildCreatedAt.Valid {
				if t, err := time.Parse("2006-01-02T15:04:05Z", buildCreatedAt.String); err == nil {
					b.CreatedAt = t
				}
			}
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
			if flakyCount.Valid {
				v := int(flakyCount.Int64)
				b.FlakyCount = &v
			}
			if retriedCount.Valid {
				v := int(retriedCount.Int64)
				b.RetriedCount = &v
			}
			if newFailedCount.Valid {
				v := int(newFailedCount.Int64)
				b.NewFailedCount = &v
			}
			if newPassedCount.Valid {
				v := int(newPassedCount.Int64)
				b.NewPassedCount = &v
			}
			if ciProvider.Valid {
				b.CIProvider = &ciProvider.String
			}
			if ciBuildURL.Valid {
				b.CIBuildURL = &ciBuildURL.String
			}
			if ciBranch.Valid {
				b.CIBranch = &ciBranch.String
			}
			if ciCommitSHA.Valid {
				b.CICommitSHA = &ciCommitSHA.String
			}
			dp.Latest = b
		}

		projectMap[projID] = dp
		orderedIDs = append(orderedIDs, projID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dashboard rows: %w", err)
	}

	if len(orderedIDs) == 0 {
		return []DashboardProject{}, nil
	}

	// Query 2: recent N builds per project for sparklines (window function).
	spRows, err := bs.db.QueryContext(ctx, `
		SELECT project_id, build_order, created_at, stat_passed, stat_total
		FROM (
		    SELECT project_id, build_order, created_at, stat_passed, stat_total,
		           ROW_NUMBER() OVER (PARTITION BY project_id ORDER BY build_order DESC) AS rn
		    FROM builds
		    WHERE stat_total IS NOT NULL AND stat_total > 0
		)
		WHERE rn <= ?
		ORDER BY project_id, build_order ASC`, sparklineDepth)
	if err != nil {
		return nil, fmt.Errorf("dashboard sparkline query: %w", err)
	}
	defer func() { _ = spRows.Close() }()

	for spRows.Next() {
		var spProjID, spCreatedAt string
		var spBuildOrder int
		var spPassed, spTotal sql.NullInt64
		if err := spRows.Scan(&spProjID, &spBuildOrder, &spCreatedAt, &spPassed, &spTotal); err != nil {
			return nil, fmt.Errorf("scan sparkline row: %w", err)
		}

		dp, ok := projectMap[spProjID]
		if !ok {
			continue
		}

		var passRate float64
		if spTotal.Valid && spTotal.Int64 > 0 && spPassed.Valid {
			passRate = float64(spPassed.Int64) / float64(spTotal.Int64) * 100
		}

		pt := SparklinePoint{BuildOrder: spBuildOrder, PassRate: passRate}
		if t, err := time.Parse("2006-01-02T15:04:05Z", spCreatedAt); err == nil {
			pt.CreatedAt = t
		}
		dp.Sparkline = append(dp.Sparkline, pt)
	}
	if err := spRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sparkline rows: %w", err)
	}

	result := make([]DashboardProject, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		result = append(result, *projectMap[id])
	}
	return result, nil
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
