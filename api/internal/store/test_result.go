package store

import (
	"context"
	"database/sql"
	"fmt"

	"go.uber.org/zap"
)

// TestResult holds one parsed test-case row to be inserted.
type TestResult struct {
	BuildID    int64
	ProjectID  string
	TestName   string
	FullName   string
	Status     string
	HistoryID  string
	DurationMs int64
	Flaky      bool
	Retries    int
	NewFailed  bool
	NewPassed  bool
}

// LowPerformingTest holds aggregated performance data for one test across builds.
type LowPerformingTest struct {
	TestName   string
	FullName   string
	HistoryID  string
	Metric     float64 // avg duration_ms or failure rate (0.0–1.0)
	BuildCount int
	Trend      []float64 // per-build values, oldest→newest
}

// TestResultStore provides operations on the test_results table.
type TestResultStore struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewTestResultStore creates a TestResultStore backed by the given SQLiteStore.
func NewTestResultStore(s *SQLiteStore, logger *zap.Logger) *TestResultStore {
	return &TestResultStore{db: s.db, logger: logger}
}

// InsertBatch inserts all results in a single transaction. Returns nil for empty slice.
func (ts *TestResultStore) InsertBatch(ctx context.Context, results []TestResult) error {
	if len(results) == 0 {
		return nil
	}
	tx, err := ts.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO test_results
			(build_id, project_id, test_name, full_name, status, duration_ms,
			 history_id, flaky, retries, new_failed, new_passed)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	for i := range results {
		r := &results[i]
		boolToInt := func(b bool) int {
			if b {
				return 1
			}
			return 0
		}
		if _, err := stmt.ExecContext(ctx,
			r.BuildID, r.ProjectID, r.TestName, r.FullName, r.Status, r.DurationMs,
			r.HistoryID, boolToInt(r.Flaky), r.Retries, boolToInt(r.NewFailed), boolToInt(r.NewPassed),
		); err != nil {
			return fmt.Errorf("insert test result: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// GetBuildID returns the database ID for a build given its project and order.
func (ts *TestResultStore) GetBuildID(ctx context.Context, projectID string, buildOrder int) (int64, error) {
	var id int64
	err := ts.db.QueryRowContext(ctx,
		"SELECT id FROM builds WHERE project_id=? AND build_order=?", projectID, buildOrder,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("get build id: %w", err)
	}
	return id, nil
}

// ListSlowest returns tests ranked by average duration across the last N builds.
func (ts *TestResultStore) ListSlowest(ctx context.Context, projectID string, builds, limit int) ([]LowPerformingTest, error) {
	query := `
		WITH recent_builds AS (
			SELECT id FROM builds
			WHERE project_id = ?
			ORDER BY build_order DESC
			LIMIT ?
		)
		SELECT history_id,
		       MAX(test_name)  AS test_name,
		       MAX(full_name)  AS full_name,
		       AVG(CAST(duration_ms AS REAL)) AS avg_duration,
		       COUNT(DISTINCT build_id)       AS build_count
		FROM test_results
		WHERE project_id = ?
		  AND build_id IN (SELECT id FROM recent_builds)
		  AND history_id != ''
		GROUP BY history_id
		ORDER BY avg_duration DESC
		LIMIT ?`

	rows, err := ts.db.QueryContext(ctx, query, projectID, builds, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("list slowest: %w", err)
	}

	var tests []LowPerformingTest
	for rows.Next() {
		var lt LowPerformingTest
		if err := rows.Scan(&lt.HistoryID, &lt.TestName, &lt.FullName, &lt.Metric, &lt.BuildCount); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan slowest: %w", err)
		}
		tests = append(tests, lt)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate slowest: %w", err)
	}
	_ = rows.Close()

	// Compute trends after closing main cursor (SQLite can't nest queries on same conn).
	for i := range tests {
		tests[i].Trend, _ = ts.trendDuration(ctx, projectID, tests[i].HistoryID, builds)
	}
	return tests, nil
}

// ListLeastReliable returns tests ranked by failure rate (failed+broken / total) across the last N builds.
func (ts *TestResultStore) ListLeastReliable(ctx context.Context, projectID string, builds, limit int) ([]LowPerformingTest, error) {
	query := `
		WITH recent_builds AS (
			SELECT id FROM builds
			WHERE project_id = ?
			ORDER BY build_order DESC
			LIMIT ?
		)
		SELECT history_id,
		       MAX(test_name)  AS test_name,
		       MAX(full_name)  AS full_name,
		       CAST(SUM(CASE WHEN status IN ('failed','broken') THEN 1 ELSE 0 END) AS REAL)
		           / CAST(COUNT(*) AS REAL) AS failure_rate,
		       COUNT(DISTINCT build_id) AS build_count
		FROM test_results
		WHERE project_id = ?
		  AND build_id IN (SELECT id FROM recent_builds)
		  AND history_id != ''
		GROUP BY history_id
		HAVING failure_rate > 0
		ORDER BY failure_rate DESC
		LIMIT ?`

	rows, err := ts.db.QueryContext(ctx, query, projectID, builds, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("list least reliable: %w", err)
	}

	var tests []LowPerformingTest
	for rows.Next() {
		var lt LowPerformingTest
		if err := rows.Scan(&lt.HistoryID, &lt.TestName, &lt.FullName, &lt.Metric, &lt.BuildCount); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("scan least reliable: %w", err)
		}
		tests = append(tests, lt)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("iterate least reliable: %w", err)
	}
	_ = rows.Close()

	// Compute trends after closing main cursor (SQLite can't nest queries on same conn).
	for i := range tests {
		tests[i].Trend, _ = ts.trendFailureRate(ctx, projectID, tests[i].HistoryID, builds)
	}
	return tests, nil
}

// trendDuration returns per-build average duration for a test, oldest→newest.
func (ts *TestResultStore) trendDuration(ctx context.Context, projectID, historyID string, builds int) ([]float64, error) {
	rows, err := ts.db.QueryContext(ctx, `
		WITH recent_builds AS (
			SELECT id FROM builds
			WHERE project_id = ?
			ORDER BY build_order DESC
			LIMIT ?
		)
		SELECT AVG(CAST(duration_ms AS REAL))
		FROM test_results
		WHERE project_id=? AND history_id=?
		  AND build_id IN (SELECT id FROM recent_builds)
		GROUP BY build_id
		ORDER BY build_id ASC`,
		projectID, builds, projectID, historyID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var trend []float64
	for rows.Next() {
		var v float64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		trend = append(trend, v)
	}
	return trend, rows.Err()
}

// trendFailureRate returns per-build failure rate for a test, oldest→newest.
func (ts *TestResultStore) trendFailureRate(ctx context.Context, projectID, historyID string, builds int) ([]float64, error) {
	rows, err := ts.db.QueryContext(ctx, `
		WITH recent_builds AS (
			SELECT id FROM builds
			WHERE project_id = ?
			ORDER BY build_order DESC
			LIMIT ?
		)
		SELECT CAST(SUM(CASE WHEN status IN ('failed','broken') THEN 1 ELSE 0 END) AS REAL)
		       / CAST(COUNT(*) AS REAL)
		FROM test_results
		WHERE project_id=? AND history_id=?
		  AND build_id IN (SELECT id FROM recent_builds)
		GROUP BY build_id
		ORDER BY build_id ASC`,
		projectID, builds, projectID, historyID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var trend []float64
	for rows.Next() {
		var v float64
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		trend = append(trend, v)
	}
	return trend, rows.Err()
}

// DeleteByProject removes all test results for the given project.
func (ts *TestResultStore) DeleteByProject(ctx context.Context, projectID string) error {
	_, err := ts.db.ExecContext(ctx,
		"DELETE FROM test_results WHERE project_id=?", projectID)
	if err != nil {
		return fmt.Errorf("delete test results for project %q: %w", projectID, err)
	}
	return nil
}
