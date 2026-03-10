package pg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// PGAnalyticsStore provides analytics queries over the expanded PG schema.
type PGAnalyticsStore struct {
	pool *pgxpool.Pool
}

// NewAnalyticsStore creates a PGAnalyticsStore backed by the given PGStore.
func NewAnalyticsStore(s *PGStore) *PGAnalyticsStore {
	return &PGAnalyticsStore{pool: s.pool}
}

// ListTopErrors returns the most common failure messages across the last N builds.
func (a *PGAnalyticsStore) ListTopErrors(ctx context.Context, projectID string, builds, limit int) ([]store.ErrorCluster, error) {
	rows, err := a.pool.Query(ctx, `
		WITH recent AS (SELECT id FROM builds WHERE project_id=$1 ORDER BY build_order DESC LIMIT $2)
		SELECT status_message, COUNT(*) AS cnt
		FROM test_results
		WHERE project_id=$3
		  AND build_id IN (SELECT id FROM recent)
		  AND status IN ('failed','broken')
		  AND status_message IS NOT NULL AND status_message != ''
		GROUP BY status_message
		ORDER BY cnt DESC
		LIMIT $4`,
		projectID, builds, projectID, limit,
	)
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

// ListSuitePassRates returns per-suite pass rates across the last N builds.
func (a *PGAnalyticsStore) ListSuitePassRates(ctx context.Context, projectID string, builds int) ([]store.SuitePassRate, error) {
	rows, err := a.pool.Query(ctx, `
		WITH recent AS (SELECT id FROM builds WHERE project_id=$1 ORDER BY build_order DESC LIMIT $2)
		SELECT tl.value AS suite,
		       COUNT(*) AS total,
		       SUM(CASE WHEN tr.status='passed' THEN 1 ELSE 0 END) AS passed
		FROM test_results tr
		JOIN test_labels tl ON tl.test_result_id = tr.id
		WHERE tr.project_id=$3
		  AND tr.build_id IN (SELECT id FROM recent)
		  AND tl.name = 'suite'
		GROUP BY tl.value
		ORDER BY tl.value`,
		projectID, builds, projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list suite pass rates: %w", err)
	}
	defer rows.Close()

	var result []store.SuitePassRate
	for rows.Next() {
		var spr store.SuitePassRate
		if err := rows.Scan(&spr.Suite, &spr.Total, &spr.Passed); err != nil {
			return nil, fmt.Errorf("scan suite pass rate: %w", err)
		}
		if spr.Total > 0 {
			spr.PassRate = float64(spr.Passed) / float64(spr.Total) * 100
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

// ListLabelBreakdown returns counts grouped by label value for a given label name.
func (a *PGAnalyticsStore) ListLabelBreakdown(ctx context.Context, projectID, labelName string, builds int) ([]store.LabelCount, error) {
	rows, err := a.pool.Query(ctx, `
		WITH recent AS (SELECT id FROM builds WHERE project_id=$1 ORDER BY build_order DESC LIMIT $2)
		SELECT tl.value, COUNT(DISTINCT tr.id) AS cnt
		FROM test_results tr
		JOIN test_labels tl ON tl.test_result_id = tr.id
		WHERE tr.project_id=$3
		  AND tr.build_id IN (SELECT id FROM recent)
		  AND tl.name = $4
		GROUP BY tl.value
		ORDER BY cnt DESC`,
		projectID, builds, projectID, labelName,
	)
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

// Compile-time interface compliance check.
var _ store.AnalyticsStorer = (*PGAnalyticsStore)(nil)
