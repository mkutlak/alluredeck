package pg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"

	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// PGBuildStore provides operations on the builds table backed by PostgreSQL.
type PGBuildStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewBuildStore creates a PGBuildStore backed by the given PGStore.
func NewBuildStore(s *PGStore, logger *zap.Logger) *PGBuildStore {
	return &PGBuildStore{pool: s.pool, logger: logger}
}

// NextBuildOrder atomically determines the next build order for a project (MAX+1, min 1).
func (bs *PGBuildStore) NextBuildOrder(ctx context.Context, projectID string) (int, error) {
	var maxOrder *int64
	err := bs.pool.QueryRow(ctx,
		"SELECT MAX(build_order) FROM builds WHERE project_id = $1", projectID,
	).Scan(&maxOrder)
	if err != nil {
		return 0, fmt.Errorf("next build order: %w", err)
	}
	if maxOrder != nil {
		return int(*maxOrder) + 1, nil
	}
	return 1, nil
}

// InsertBuild records a new build with the given order.
func (bs *PGBuildStore) InsertBuild(ctx context.Context, projectID string, buildOrder int) error {
	_, err := bs.pool.Exec(ctx,
		"INSERT INTO builds(project_id, build_order) VALUES($1,$2)", projectID, buildOrder)
	if err != nil {
		return fmt.Errorf("insert build: %w", err)
	}
	return nil
}

// UpdateBuildStats updates statistics for the given build.
func (bs *PGBuildStore) UpdateBuildStats(ctx context.Context, projectID string, buildOrder int, stats store.BuildStats) error {
	tag, err := bs.pool.Exec(ctx, `
		UPDATE builds
		SET stat_passed=$1, stat_failed=$2, stat_broken=$3,
		    stat_skipped=$4, stat_unknown=$5, stat_total=$6,
		    duration_ms=$7,
		    flaky_count=$8, retried_count=$9, new_failed_count=$10, new_passed_count=$11
		WHERE project_id=$12 AND build_order=$13`,
		stats.Passed, stats.Failed, stats.Broken,
		stats.Skipped, stats.Unknown, stats.Total,
		stats.DurationMs,
		stats.FlakyCount, stats.RetriedCount, stats.NewFailedCount, stats.NewPassedCount,
		projectID, buildOrder)
	if err != nil {
		return fmt.Errorf("update build stats: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: project=%s build=%d", store.ErrBuildNotFound, projectID, buildOrder)
	}
	return nil
}

// UpdateBuildCIMetadata stores CI context for the given build.
func (bs *PGBuildStore) UpdateBuildCIMetadata(ctx context.Context, projectID string, buildOrder int, ciMeta store.CIMetadata) error {
	nullStr := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	tag, err := bs.pool.Exec(ctx, `
		UPDATE builds
		SET ci_provider=$1, ci_build_url=$2, ci_branch=$3, ci_commit_sha=$4
		WHERE project_id=$5 AND build_order=$6`,
		nullStr(ciMeta.Provider), nullStr(ciMeta.BuildURL),
		nullStr(ciMeta.Branch), nullStr(ciMeta.CommitSHA),
		projectID, buildOrder)
	if err != nil {
		return fmt.Errorf("update build ci metadata: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: project=%s build=%d", store.ErrBuildNotFound, projectID, buildOrder)
	}
	return nil
}

const buildSelectCols = `
	SELECT id, project_id, build_order, created_at,
	       stat_passed, stat_failed, stat_broken, stat_skipped, stat_unknown, stat_total,
	       duration_ms, is_latest,
	       flaky_count, retried_count, new_failed_count, new_passed_count,
	       ci_provider, ci_build_url, ci_branch, ci_commit_sha
	FROM builds`

// buildRowScanner is satisfied by both pgx.Row and pgx.Rows.
type buildRowScanner interface {
	Scan(dest ...any) error
}

// scanBuild scans a single build row from any buildRowScanner (pgx.Row or pgx.Rows).
func scanBuild(row buildRowScanner) (store.Build, error) {
	var b store.Build
	var statPassed, statFailed, statBroken, statSkipped, statUnknown, statTotal *int32
	var durationMs *int64
	var flakyCount, retriedCount, newFailedCount, newPassedCount int
	var ciProvider, ciBuildURL, ciBranch, ciCommitSHA *string

	if err := row.Scan(
		&b.ID, &b.ProjectID, &b.BuildOrder, &b.CreatedAt,
		&statPassed, &statFailed, &statBroken, &statSkipped, &statUnknown, &statTotal,
		&durationMs, &b.IsLatest,
		&flakyCount, &retriedCount, &newFailedCount, &newPassedCount,
		&ciProvider, &ciBuildURL, &ciBranch, &ciCommitSHA,
	); err != nil {
		return store.Build{}, err
	}
	assignBuildStats(&b, statPassed, statFailed, statBroken, statSkipped, statUnknown, statTotal, durationMs,
		flakyCount, retriedCount, newFailedCount, newPassedCount,
		ciProvider, ciBuildURL, ciBranch, ciCommitSHA)
	return b, nil
}

// scanBuildRow scans a single build from a pgx.Row.
func scanBuildRow(row pgx.Row) (store.Build, error) { return scanBuild(row) }

// scanBuildRowsEntry scans a single build from a pgx.Rows cursor.
func scanBuildRowsEntry(rows pgx.Rows) (store.Build, error) { return scanBuild(rows) }

func assignBuildStats(b *store.Build,
	passed, failed, broken, skipped, unknown, total *int32,
	durationMs *int64,
	flakyCount, retriedCount, newFailedCount, newPassedCount int,
	ciProvider, ciBuildURL, ciBranch, ciCommitSHA *string,
) {
	if passed != nil {
		v := int(*passed)
		b.StatPassed = &v
	}
	if failed != nil {
		v := int(*failed)
		b.StatFailed = &v
	}
	if broken != nil {
		v := int(*broken)
		b.StatBroken = &v
	}
	if skipped != nil {
		v := int(*skipped)
		b.StatSkipped = &v
	}
	if unknown != nil {
		v := int(*unknown)
		b.StatUnknown = &v
	}
	if total != nil {
		v := int(*total)
		b.StatTotal = &v
	}
	b.DurationMs = durationMs
	b.FlakyCount = &flakyCount
	b.RetriedCount = &retriedCount
	b.NewFailedCount = &newFailedCount
	b.NewPassedCount = &newPassedCount
	b.CIProvider = ciProvider
	b.CIBuildURL = ciBuildURL
	b.CIBranch = ciBranch
	b.CICommitSHA = ciCommitSHA
}

// GetBuildByOrder returns a single build for project_id + build_order.
func (bs *PGBuildStore) GetBuildByOrder(ctx context.Context, projectID string, buildOrder int) (store.Build, error) {
	row := bs.pool.QueryRow(ctx, buildSelectCols+`
		WHERE project_id=$1 AND build_order=$2`, projectID, buildOrder)
	b, err := scanBuildRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return store.Build{}, fmt.Errorf("%w: project=%s build=%d", store.ErrBuildNotFound, projectID, buildOrder)
		}
		return store.Build{}, fmt.Errorf("get build by order: %w", err)
	}
	return b, nil
}

// GetPreviousBuild returns the build immediately before the given build_order.
func (bs *PGBuildStore) GetPreviousBuild(ctx context.Context, projectID string, buildOrder int) (store.Build, error) {
	row := bs.pool.QueryRow(ctx, buildSelectCols+`
		WHERE project_id=$1 AND build_order<$2
		ORDER BY build_order DESC LIMIT 1`, projectID, buildOrder)
	b, err := scanBuildRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return store.Build{}, fmt.Errorf("%w: no previous build for project=%s build=%d", store.ErrBuildNotFound, projectID, buildOrder)
		}
		return store.Build{}, fmt.Errorf("get previous build: %w", err)
	}
	return b, nil
}

// GetLatestBuild returns the build marked is_latest=TRUE for a project.
func (bs *PGBuildStore) GetLatestBuild(ctx context.Context, projectID string) (store.Build, error) {
	row := bs.pool.QueryRow(ctx, buildSelectCols+`
		WHERE project_id=$1 AND is_latest=TRUE`, projectID)
	b, err := scanBuildRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return store.Build{}, fmt.Errorf("%w: no latest build for project=%s", store.ErrBuildNotFound, projectID)
		}
		return store.Build{}, fmt.Errorf("get latest build: %w", err)
	}
	return b, nil
}

// ListBuilds returns all builds for a project in descending build_order.
func (bs *PGBuildStore) ListBuilds(ctx context.Context, projectID string) ([]store.Build, error) {
	rows, err := bs.pool.Query(ctx, buildSelectCols+`
		WHERE project_id=$1 ORDER BY build_order DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list builds: %w", err)
	}
	defer rows.Close()
	return scanBuildRowsAll(rows)
}

// ListBuildsPaginated returns a page of builds for a project in descending build_order, plus the total count.
func (bs *PGBuildStore) ListBuildsPaginated(ctx context.Context, projectID string, page, perPage int) ([]store.Build, int, error) {
	var totalCount int
	if err := bs.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM builds WHERE project_id=$1", projectID,
	).Scan(&totalCount); err != nil {
		return nil, 0, fmt.Errorf("count builds: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := bs.pool.Query(ctx, buildSelectCols+`
		WHERE project_id=$1 ORDER BY build_order DESC LIMIT $2 OFFSET $3`,
		projectID, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list builds paginated: %w", err)
	}
	defer rows.Close()

	builds, err := scanBuildRowsAll(rows)
	if err != nil {
		return nil, 0, err
	}
	return builds, totalCount, nil
}

// PruneBuilds removes the oldest builds exceeding `keep` count.
// Returns the build_orders of removed builds.
func (bs *PGBuildStore) PruneBuilds(ctx context.Context, projectID string, keep int) ([]int, error) {
	if keep <= 0 {
		return nil, nil
	}
	rows, err := bs.pool.Query(ctx, `
		SELECT build_order FROM builds
		WHERE project_id=$1
		ORDER BY build_order DESC
		LIMIT ALL OFFSET $2`, projectID, keep)
	if err != nil {
		return nil, fmt.Errorf("prune builds query: %w", err)
	}
	defer rows.Close()

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
		if _, err := bs.pool.Exec(ctx,
			"DELETE FROM builds WHERE project_id=$1 AND build_order=$2", projectID, bo); err != nil {
			return nil, fmt.Errorf("delete build %d: %w", bo, err)
		}
	}
	return toRemove, nil
}

// SetLatest marks the given build_order as latest and clears the flag on all others.
func (bs *PGBuildStore) SetLatest(ctx context.Context, projectID string, buildOrder int) error {
	tx, err := bs.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		"UPDATE builds SET is_latest=FALSE WHERE project_id=$1 AND is_latest=TRUE", projectID); err != nil {
		return fmt.Errorf("clear is_latest: %w", err)
	}
	if _, err := tx.Exec(ctx,
		"UPDATE builds SET is_latest=TRUE WHERE project_id=$1 AND build_order=$2",
		projectID, buildOrder); err != nil {
		return fmt.Errorf("set is_latest: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit build prune transaction: %w", err)
	}
	return nil
}

// DeleteAllBuilds removes all builds for a project from the database.
func (bs *PGBuildStore) DeleteAllBuilds(ctx context.Context, projectID string) error {
	_, err := bs.pool.Exec(ctx, "DELETE FROM builds WHERE project_id=$1", projectID)
	if err != nil {
		return fmt.Errorf("delete all builds for project %q: %w", projectID, err)
	}
	return nil
}

// GetDashboardData returns all projects with their latest build and sparkline data.
func (bs *PGBuildStore) GetDashboardData(ctx context.Context, sparklineDepth int, tag string) ([]store.DashboardProject, error) {
	var qArgs []any
	var query string
	if tag != "" {
		tagJSON, _ := json.Marshal([]string{tag})
		query = `
		SELECT DISTINCT ON (p.id)
		    p.id, p.created_at, p.tags,
		    b.id, b.project_id, b.build_order, b.created_at,
		    b.stat_passed, b.stat_failed, b.stat_broken, b.stat_skipped, b.stat_unknown, b.stat_total,
		    b.duration_ms, b.is_latest,
		    b.flaky_count, b.retried_count, b.new_failed_count, b.new_passed_count,
		    b.ci_provider, b.ci_build_url, b.ci_branch, b.ci_commit_sha
		FROM projects p
		LEFT JOIN builds b ON b.project_id=p.id AND b.is_latest=TRUE
		WHERE p.tags @> $1::jsonb
		ORDER BY p.id, b.build_order DESC NULLS LAST`
		qArgs = []any{string(tagJSON)}
	} else {
		query = `
		SELECT DISTINCT ON (p.id)
		    p.id, p.created_at, p.tags,
		    b.id, b.project_id, b.build_order, b.created_at,
		    b.stat_passed, b.stat_failed, b.stat_broken, b.stat_skipped, b.stat_unknown, b.stat_total,
		    b.duration_ms, b.is_latest,
		    b.flaky_count, b.retried_count, b.new_failed_count, b.new_passed_count,
		    b.ci_provider, b.ci_build_url, b.ci_branch, b.ci_commit_sha
		FROM projects p
		LEFT JOIN builds b ON b.project_id=p.id AND b.is_latest=TRUE
		ORDER BY p.id, b.build_order DESC NULLS LAST`
	}

	rows, err := bs.pool.Query(ctx, query, qArgs...)
	if err != nil {
		return nil, fmt.Errorf("dashboard projects query: %w", err)
	}
	defer rows.Close()

	projectMap := make(map[string]*store.DashboardProject)
	var orderedIDs []string

	for rows.Next() {
		var projID string
		var projCreatedAt time.Time
		var projTags []byte
		// Nullable build fields (LEFT JOIN may produce NULLs).
		var buildID *int64
		var buildProjID *string
		var buildOrder *int32
		var buildCreatedAt *time.Time
		var statPassed, statFailed, statBroken, statSkipped, statUnknown, statTotal *int32
		var durationMs *int64
		var isLatest *bool
		var flakyCount, retriedCount, newFailedCount, newPassedCount *int32
		var ciProvider, ciBuildURL, ciBranch, ciCommitSHA *string

		if err := rows.Scan(
			&projID, &projCreatedAt, &projTags,
			&buildID, &buildProjID, &buildOrder, &buildCreatedAt,
			&statPassed, &statFailed, &statBroken, &statSkipped, &statUnknown, &statTotal,
			&durationMs, &isLatest,
			&flakyCount, &retriedCount, &newFailedCount, &newPassedCount,
			&ciProvider, &ciBuildURL, &ciBranch, &ciCommitSHA,
		); err != nil {
			return nil, fmt.Errorf("scan dashboard row: %w", err)
		}

		dp := &store.DashboardProject{
			ProjectID: projID,
			CreatedAt: projCreatedAt,
			Tags:      parseTags(projTags),
		}

		if buildID != nil {
			b := &store.Build{
				ID:        *buildID,
				ProjectID: *buildProjID,
			}
			if buildOrder != nil {
				b.BuildOrder = int(*buildOrder)
			}
			if buildCreatedAt != nil {
				b.CreatedAt = *buildCreatedAt
			}
			if isLatest != nil {
				b.IsLatest = *isLatest
			}
			fc, rc, nfc, npc := 0, 0, 0, 0
			if flakyCount != nil {
				fc = int(*flakyCount)
			}
			if retriedCount != nil {
				rc = int(*retriedCount)
			}
			if newFailedCount != nil {
				nfc = int(*newFailedCount)
			}
			if newPassedCount != nil {
				npc = int(*newPassedCount)
			}
			assignBuildStats(b, statPassed, statFailed, statBroken, statSkipped, statUnknown, statTotal, durationMs,
				fc, rc, nfc, npc, ciProvider, ciBuildURL, ciBranch, ciCommitSHA)
			dp.Latest = b
		}

		if _, seen := projectMap[projID]; !seen {
			orderedIDs = append(orderedIDs, projID)
		}
		projectMap[projID] = dp
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dashboard rows: %w", err)
	}

	if len(orderedIDs) == 0 {
		return []store.DashboardProject{}, nil
	}

	// Sparkline: recent N builds per project using window function.
	spRows, err := bs.pool.Query(ctx, `
		SELECT project_id, build_order, created_at, stat_passed, stat_total
		FROM (
		    SELECT project_id, build_order, created_at, stat_passed, stat_total,
		           ROW_NUMBER() OVER (PARTITION BY project_id ORDER BY build_order DESC) AS rn
		    FROM builds
		    WHERE stat_total IS NOT NULL AND stat_total > 0
		)  ranked
		WHERE rn <= $1
		ORDER BY project_id, build_order ASC`, sparklineDepth)
	if err != nil {
		return nil, fmt.Errorf("dashboard sparkline query: %w", err)
	}
	defer spRows.Close()

	for spRows.Next() {
		var spProjID string
		var spBuildOrder int
		var spCreatedAt time.Time
		var spPassed, spTotal *int32
		if err := spRows.Scan(&spProjID, &spBuildOrder, &spCreatedAt, &spPassed, &spTotal); err != nil {
			return nil, fmt.Errorf("scan sparkline row: %w", err)
		}

		dp, ok := projectMap[spProjID]
		if !ok {
			continue
		}

		var passRate float64
		if spTotal != nil && *spTotal > 0 && spPassed != nil {
			passRate = float64(*spPassed) / float64(*spTotal) * 100
		}
		dp.Sparkline = append(dp.Sparkline, store.SparklinePoint{
			BuildOrder: spBuildOrder,
			PassRate:   passRate,
			CreatedAt:  spCreatedAt,
		})
	}
	if err := spRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate sparkline rows: %w", err)
	}

	result := make([]store.DashboardProject, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		result = append(result, *projectMap[id])
	}
	return result, nil
}

// DeleteBuild removes a single build by build_order from the database.
func (bs *PGBuildStore) DeleteBuild(ctx context.Context, projectID string, buildOrder int) error {
	_, err := bs.pool.Exec(ctx,
		"DELETE FROM builds WHERE project_id=$1 AND build_order=$2", projectID, buildOrder)
	if err != nil {
		return fmt.Errorf("delete build: %w", err)
	}
	return nil
}

// UpdateBuildBranchID sets the branch_id on a build row.
func (bs *PGBuildStore) UpdateBuildBranchID(ctx context.Context, projectID string, buildOrder int, branchID int64) error {
	tag, err := bs.pool.Exec(ctx,
		"UPDATE builds SET branch_id=$1 WHERE project_id=$2 AND build_order=$3",
		branchID, projectID, buildOrder)
	if err != nil {
		return fmt.Errorf("update build branch_id: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: project=%s build=%d", store.ErrBuildNotFound, projectID, buildOrder)
	}
	return nil
}

// SetLatestBranch marks the given build_order as latest, optionally scoped to a branch.
func (bs *PGBuildStore) SetLatestBranch(ctx context.Context, projectID string, buildOrder int, branchID *int64) error {
	if branchID == nil {
		return bs.SetLatest(ctx, projectID, buildOrder)
	}
	tx, err := bs.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		"UPDATE builds SET is_latest=FALSE WHERE project_id=$1 AND branch_id=$2 AND is_latest=TRUE",
		projectID, *branchID); err != nil {
		return fmt.Errorf("clear is_latest for branch: %w", err)
	}
	if _, err := tx.Exec(ctx,
		"UPDATE builds SET is_latest=TRUE WHERE project_id=$1 AND build_order=$2",
		projectID, buildOrder); err != nil {
		return fmt.Errorf("set is_latest: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit set latest branch: %w", err)
	}
	return nil
}

// PruneBuildsBranch removes oldest builds exceeding `keep` count, optionally scoped to a branch.
func (bs *PGBuildStore) PruneBuildsBranch(ctx context.Context, projectID string, keep int, branchID *int64) ([]int, error) {
	if keep <= 0 {
		return nil, nil
	}

	var rows pgx.Rows
	var err error
	if branchID != nil {
		rows, err = bs.pool.Query(ctx, `
			SELECT build_order FROM builds
			WHERE project_id=$1 AND branch_id=$2
			ORDER BY build_order DESC
			LIMIT ALL OFFSET $3`, projectID, *branchID, keep)
	} else {
		rows, err = bs.pool.Query(ctx, `
			SELECT build_order FROM builds
			WHERE project_id=$1
			ORDER BY build_order DESC
			LIMIT ALL OFFSET $2`, projectID, keep)
	}
	if err != nil {
		return nil, fmt.Errorf("prune builds branch query: %w", err)
	}
	defer rows.Close()

	var toRemove []int
	for rows.Next() {
		var bo int
		if err := rows.Scan(&bo); err != nil {
			return nil, fmt.Errorf("scan build_order: %w", err)
		}
		toRemove = append(toRemove, bo)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate prune branch rows: %w", err)
	}

	for _, bo := range toRemove {
		if _, err := bs.pool.Exec(ctx,
			"DELETE FROM builds WHERE project_id=$1 AND build_order=$2", projectID, bo); err != nil {
			return nil, fmt.Errorf("delete build %d: %w", bo, err)
		}
	}
	return toRemove, nil
}

// ListBuildsPaginatedBranch returns a page of builds, optionally filtered by branch.
func (bs *PGBuildStore) ListBuildsPaginatedBranch(ctx context.Context, projectID string, page, perPage int, branchID *int64) ([]store.Build, int, error) {
	var totalCount int
	var err error
	if branchID != nil {
		err = bs.pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM builds WHERE project_id=$1 AND branch_id=$2", projectID, *branchID,
		).Scan(&totalCount)
	} else {
		err = bs.pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM builds WHERE project_id=$1", projectID,
		).Scan(&totalCount)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("count builds branch: %w", err)
	}

	offset := (page - 1) * perPage
	var rows pgx.Rows
	if branchID != nil {
		rows, err = bs.pool.Query(ctx, buildSelectCols+`
			WHERE project_id=$1 AND branch_id=$2
			ORDER BY build_order DESC LIMIT $3 OFFSET $4`,
			projectID, *branchID, perPage, offset)
	} else {
		rows, err = bs.pool.Query(ctx, buildSelectCols+`
			WHERE project_id=$1
			ORDER BY build_order DESC LIMIT $2 OFFSET $3`,
			projectID, perPage, offset)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("list builds paginated branch: %w", err)
	}
	defer rows.Close()

	builds, err := scanBuildRowsAll(rows)
	if err != nil {
		return nil, 0, err
	}
	return builds, totalCount, nil
}

// scanBuildRowsAll scans all builds from pgx.Rows.
func scanBuildRowsAll(rows pgx.Rows) ([]store.Build, error) {
	var builds []store.Build
	for rows.Next() {
		b, err := scanBuildRowsEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan build: %w", err)
		}
		builds = append(builds, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate build rows: %w", err)
	}
	return builds, nil
}

// ExistingBuildOrders returns the set of build orders already in the DB for a project.
func (bs *PGBuildStore) ExistingBuildOrders(ctx context.Context, projectID string) (map[int]struct{}, error) {
	rows, err := bs.pool.Query(ctx,
		"SELECT build_order FROM builds WHERE project_id=$1", projectID)
	if err != nil {
		return nil, fmt.Errorf("existing build orders for %q: %w", projectID, err)
	}
	defer rows.Close()

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

// InsertMissingBuilds inserts builds using ON CONFLICT DO NOTHING for idempotency.
func (bs *PGBuildStore) InsertMissingBuilds(ctx context.Context, projectID string, missing []int) error {
	if len(missing) == 0 {
		return nil
	}
	tx, err := bs.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, bo := range missing {
		if _, err := tx.Exec(ctx,
			"INSERT INTO builds(project_id, build_order) VALUES($1,$2) ON CONFLICT (project_id, build_order) DO NOTHING",
			projectID, bo); err != nil {
			return fmt.Errorf("insert build %d for project %q: %w", bo, projectID, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit insert builds: %w", err)
	}
	return nil
}

// BuildsWithMissingStats returns build orders where stat_total is NULL.
func (bs *PGBuildStore) BuildsWithMissingStats(ctx context.Context, projectID string) ([]int, error) {
	rows, err := bs.pool.Query(ctx,
		"SELECT build_order FROM builds WHERE project_id=$1 AND stat_total IS NULL", projectID)
	if err != nil {
		return nil, fmt.Errorf("builds with missing stats for %q: %w", projectID, err)
	}
	defer rows.Close()

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

type statsResult struct {
	buildOrder int
	stats      storage.BuildStats
}

// BatchSyncStats reads stats from storage concurrently and writes them in one transaction.
func (bs *PGBuildStore) BatchSyncStats(ctx context.Context, projectID string, buildOrders []int, st storage.Store) error {
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
				bs.logger.Info("BatchSyncStats: stats unavailable (will retry next startup)",
					zap.String("project_id", projectID), zap.Int("build_order", bo), zap.Error(err))
				return nil
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

	tx, err := bs.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for i := range results {
		r := &results[i]
		if _, err := tx.Exec(ctx, `
			UPDATE builds
			SET stat_passed=$1, stat_failed=$2, stat_broken=$3,
			    stat_skipped=$4, stat_unknown=$5, stat_total=$6, duration_ms=$7
			WHERE project_id=$8 AND build_order=$9`,
			r.stats.Passed, r.stats.Failed, r.stats.Broken,
			r.stats.Skipped, r.stats.Unknown, r.stats.Total, r.stats.DurationMs,
			projectID, r.buildOrder,
		); err != nil {
			return fmt.Errorf("update stats for build %d: %w", r.buildOrder, err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit stats update: %w", err)
	}
	return nil
}

var _ store.BuildStorer = (*PGBuildStore)(nil)
