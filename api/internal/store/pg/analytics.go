package pg

import (
	"context"
	"fmt"
	"math"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// AnalyticsStore provides analytics queries over the expanded PG schema.
type AnalyticsStore struct {
	pool *pgxpool.Pool
}

// NewAnalyticsStore creates a AnalyticsStore backed by the given PGStore.
func NewAnalyticsStore(s *PGStore) *AnalyticsStore {
	return &AnalyticsStore{pool: s.pool}
}

// ListTopErrors returns the most common failure messages across the last N builds for one or more projects.
func (a *AnalyticsStore) ListTopErrors(ctx context.Context, projectIDs []int64, builds, limit int, branchID *int64) ([]store.ErrorCluster, error) {
	recentCTE := "SELECT id FROM builds WHERE project_id = ANY($1) ORDER BY build_order DESC LIMIT $2"
	args := []any{projectIDs, builds, projectIDs, limit}
	if branchID != nil {
		recentCTE = "SELECT id FROM builds WHERE project_id = ANY($1) AND branch_id=$5 ORDER BY build_order DESC LIMIT $2"
		args = append(args, *branchID)
	}
	query := fmt.Sprintf(`
		WITH recent AS (%s)
		SELECT status_message, COUNT(*) AS cnt
		FROM test_results
		WHERE project_id = ANY($3)
		  AND build_id IN (SELECT id FROM recent)
		  AND status IN ('failed','broken')
		  AND status_message IS NOT NULL AND status_message != ''
		GROUP BY status_message
		ORDER BY cnt DESC
		LIMIT $4`, recentCTE)

	rows, err := a.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list top errors: %w", err)
	}
	defer rows.Close()

	var result []store.ErrorCluster
	for rows.Next() {
		var ec store.ErrorCluster
		if err := rows.Scan(&ec.Message, &ec.Count); err != nil {
			return nil, fmt.Errorf("scan error cluster: %w", err)
		}
		result = append(result, ec)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate error clusters: %w", err)
	}
	if result == nil {
		result = []store.ErrorCluster{}
	}
	return result, nil
}

// ListSuitePassRates returns per-suite pass rates across the last N builds for one or more projects.
func (a *AnalyticsStore) ListSuitePassRates(ctx context.Context, projectIDs []int64, builds int, branchID *int64) ([]store.SuitePassRate, error) {
	recentCTE := "SELECT id FROM builds WHERE project_id = ANY($1) ORDER BY build_order DESC LIMIT $2"
	args := []any{projectIDs, builds, projectIDs}
	if branchID != nil {
		recentCTE = "SELECT id FROM builds WHERE project_id = ANY($1) AND branch_id=$4 ORDER BY build_order DESC LIMIT $2"
		args = append(args, *branchID)
	}
	query := fmt.Sprintf(`
		WITH recent AS (%s)
		SELECT tl.value AS suite,
		       COUNT(*) AS total,
		       SUM(CASE WHEN tr.status='passed' THEN 1 ELSE 0 END) AS passed,
		       SUM(CASE WHEN tr.status='skipped' THEN 1 ELSE 0 END) AS skipped
		FROM test_results tr
		JOIN test_labels tl ON tl.test_result_id = tr.id
		WHERE tr.project_id = ANY($3)
		  AND tr.build_id IN (SELECT id FROM recent)
		  AND tl.name = 'suite'
		GROUP BY tl.value
		ORDER BY tl.value`, recentCTE)

	rows, err := a.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list suite pass rates: %w", err)
	}
	defer rows.Close()

	var result []store.SuitePassRate
	for rows.Next() {
		var spr store.SuitePassRate
		var skipped int
		if err := rows.Scan(&spr.Suite, &spr.Total, &spr.Passed, &skipped); err != nil {
			return nil, fmt.Errorf("scan suite pass rate: %w", err)
		}
		denom := spr.Total - skipped
		if denom > 0 {
			spr.PassRate = math.Round(float64(spr.Passed)/float64(denom)*10000) / 100
		}
		result = append(result, spr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate suite pass rates: %w", err)
	}
	if result == nil {
		result = []store.SuitePassRate{}
	}
	return result, nil
}

// ListLabelBreakdown returns counts grouped by label value for a given label name across one or more projects.
func (a *AnalyticsStore) ListLabelBreakdown(ctx context.Context, projectIDs []int64, labelName string, builds int, branchID *int64) ([]store.LabelCount, error) {
	recentCTE := "SELECT id FROM builds WHERE project_id = ANY($1) ORDER BY build_order DESC LIMIT $2"
	args := []any{projectIDs, builds, projectIDs, labelName}
	if branchID != nil {
		recentCTE = "SELECT id FROM builds WHERE project_id = ANY($1) AND branch_id=$5 ORDER BY build_order DESC LIMIT $2"
		args = append(args, *branchID)
	}
	query := fmt.Sprintf(`
		WITH recent AS (%s)
		SELECT tl.value, COUNT(DISTINCT tr.id) AS cnt
		FROM test_results tr
		JOIN test_labels tl ON tl.test_result_id = tr.id
		WHERE tr.project_id = ANY($3)
		  AND tr.build_id IN (SELECT id FROM recent)
		  AND tl.name = $4
		GROUP BY tl.value
		ORDER BY cnt DESC`, recentCTE)

	rows, err := a.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list label breakdown: %w", err)
	}
	defer rows.Close()

	var result []store.LabelCount
	for rows.Next() {
		var lc store.LabelCount
		if err := rows.Scan(&lc.Value, &lc.Count); err != nil {
			return nil, fmt.Errorf("scan label count: %w", err)
		}
		result = append(result, lc)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate label counts: %w", err)
	}
	if result == nil {
		result = []store.LabelCount{}
	}
	return result, nil
}

// ListTrendPoints returns per-build statistics for the last N builds across one or more projects, ordered chronologically (oldest first).
func (a *AnalyticsStore) ListTrendPoints(ctx context.Context, projectIDs []int64, builds int, branchID *int64) ([]store.TrendPoint, error) {
	query := `SELECT build_order,
       COALESCE(stat_passed, 0),
       COALESCE(stat_failed, 0),
       COALESCE(stat_broken, 0),
       COALESCE(stat_skipped, 0),
       COALESCE(stat_total, 0),
       COALESCE(duration_ms, 0)
FROM builds
WHERE project_id = ANY($1)`
	args := []any{projectIDs, builds}
	if branchID != nil {
		query += " AND branch_id = $3"
		args = append(args, *branchID)
	}
	query += " ORDER BY build_order DESC LIMIT $2"

	rows, err := a.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list trend points: %w", err)
	}
	defer rows.Close()

	var result []store.TrendPoint
	for rows.Next() {
		var tp store.TrendPoint
		if err := rows.Scan(&tp.BuildNumber, &tp.Passed, &tp.Failed, &tp.Broken, &tp.Skipped, &tp.Total, &tp.DurationMs); err != nil {
			return nil, fmt.Errorf("scan trend point: %w", err)
		}
		denom := tp.Total - tp.Skipped
		if denom > 0 {
			tp.PassRate = math.Round(float64(tp.Passed)/float64(denom)*10000) / 100
		}
		result = append(result, tp)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate trend points: %w", err)
	}
	if result == nil {
		result = []store.TrendPoint{}
	}
	// Reverse to chronological (oldest first) order.
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result, nil
}

// ListFlakyImpact returns tests with flaky/retry impact metrics across the
// last N builds for a project, optionally scoped to a branch. Only tests with
// at least one flaky occurrence in the window are included. Ordered by wasted
// time (retries * duration_ms) descending, then flaky count descending.
func (a *AnalyticsStore) ListFlakyImpact(ctx context.Context, projectID int64, branchID *int64, builds, limit int) ([]store.FlakyImpact, error) {
	recentCTE := "SELECT id FROM builds WHERE project_id=$1 ORDER BY build_order DESC LIMIT $2"
	args := []any{projectID, builds, projectID, limit}
	if branchID != nil {
		recentCTE = "SELECT id FROM builds WHERE project_id=$1 AND branch_id=$5 ORDER BY build_order DESC LIMIT $2"
		args = append(args, *branchID)
	}
	query := fmt.Sprintf(`
		WITH recent AS (%s),
		agg AS (
			SELECT
				tr.full_name,
				SUM(CASE WHEN tr.flaky THEN 1 ELSE 0 END) AS flaky_count,
				SUM(tr.retries) AS retry_sum,
				SUM(tr.retries * tr.duration_ms) AS wasted_ms,
				SUM(CASE WHEN tr.status IN ('failed','broken') THEN 1 ELSE 0 END)::float8
					/ COUNT(*)::float8 AS failure_rate,
				COUNT(DISTINCT tr.build_id) AS runs,
				COUNT(DISTINCT tr.build_id) FILTER (WHERE tr.flaky) AS builds_affected,
				(array_agg(b.build_order ORDER BY b.build_order))[1] AS first_seen_build_order,
				(array_agg(b.id ORDER BY b.build_order))[1] AS first_seen_build_id,
				(array_agg(b.build_order ORDER BY b.build_order DESC))[1] AS last_seen_build_order,
				(array_agg(b.id ORDER BY b.build_order DESC))[1] AS last_seen_build_id,
				(array_agg(b.created_at ORDER BY b.build_order DESC))[1] AS last_seen_at,
				(array_agg(COALESCE(b.ci_build_url, '') ORDER BY b.build_order DESC))[1] AS ci_build_url
			FROM test_results tr
			JOIN builds b ON b.id = tr.build_id
			WHERE tr.project_id=$3
			  AND tr.build_id IN (SELECT id FROM recent)
			  AND tr.full_name != ''
			GROUP BY tr.full_name
			HAVING SUM(CASE WHEN tr.flaky THEN 1 ELSE 0 END) > 0
		)
		SELECT agg.full_name, agg.flaky_count, agg.retry_sum, agg.wasted_ms, agg.failure_rate,
		       agg.runs, agg.builds_affected,
		       agg.first_seen_build_order, agg.first_seen_build_id,
		       agg.last_seen_build_order, agg.last_seen_build_id, agg.last_seen_at,
		       agg.ci_build_url
		FROM agg
		ORDER BY agg.wasted_ms DESC, agg.flaky_count DESC
		LIMIT $4`, recentCTE)

	rows, err := a.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list flaky impact: %w", err)
	}
	defer rows.Close()

	var result []store.FlakyImpact
	for rows.Next() {
		var fi store.FlakyImpact
		if err := rows.Scan(
			&fi.FullName, &fi.FlakyCount, &fi.RetrySum, &fi.WastedMs, &fi.FailureRate,
			&fi.Runs, &fi.BuildsAffected,
			&fi.FirstSeenBuildOrder, &fi.FirstSeenBuildID,
			&fi.LastSeenBuildOrder, &fi.LastSeenBuildID, &fi.LastSeenAt,
			&fi.CIBuildURL,
		); err != nil {
			return nil, fmt.Errorf("scan flaky impact: %w", err)
		}
		result = append(result, fi)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate flaky impact: %w", err)
	}
	if result == nil {
		result = []store.FlakyImpact{}
	}
	return result, nil
}

// Compile-time interface compliance check.
var _ store.AnalyticsStorer = (*AnalyticsStore)(nil)
