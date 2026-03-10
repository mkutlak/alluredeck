package pg

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	parser "github.com/mkutlak/alluredeck/api/internal/parser"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// PGTestResultStore provides operations on the test_results table using PostgreSQL.
type PGTestResultStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewTestResultStore creates a PGTestResultStore backed by the given PGStore.
func NewTestResultStore(s *PGStore, logger *zap.Logger) *PGTestResultStore {
	return &PGTestResultStore{pool: s.pool, logger: logger}
}

// InsertBatch inserts all results in a single transaction. Returns nil for empty slice.
func (ts *PGTestResultStore) InsertBatch(ctx context.Context, results []store.TestResult) error {
	if len(results) == 0 {
		return nil
	}
	tx, err := ts.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for i := range results {
		r := &results[i]
		if _, err := tx.Exec(ctx, `
			INSERT INTO test_results
				(build_id, project_id, test_name, full_name, status, duration_ms,
				 history_id, flaky, retries, new_failed, new_passed,
				 start_ms, stop_ms, thread, host)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
			r.BuildID, r.ProjectID, r.TestName, r.FullName, r.Status, r.DurationMs,
			r.HistoryID, r.Flaky, r.Retries, r.NewFailed, r.NewPassed,
			r.StartMs, r.StopMs, r.Thread, r.Host,
		); err != nil {
			return fmt.Errorf("insert test result: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// GetBuildID returns the database ID for a build given its project and order.
func (ts *PGTestResultStore) GetBuildID(ctx context.Context, projectID string, buildOrder int) (int64, error) {
	var id int64
	err := ts.pool.QueryRow(ctx,
		"SELECT id FROM builds WHERE project_id=$1 AND build_order=$2", projectID, buildOrder,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("get build id: %w", err)
	}
	return id, nil
}

// ListSlowest returns tests ranked by average duration across the last N builds.
func (ts *PGTestResultStore) ListSlowest(ctx context.Context, projectID string, builds, limit int) ([]store.LowPerformingTest, error) {
	query := `
		WITH recent_builds AS (
			SELECT id FROM builds
			WHERE project_id=$1
			ORDER BY build_order DESC
			LIMIT $2
		)
		SELECT history_id,
		       MAX(test_name)  AS test_name,
		       MAX(full_name)  AS full_name,
		       AVG(duration_ms::float8) AS avg_duration,
		       COUNT(DISTINCT build_id) AS build_count
		FROM test_results
		WHERE project_id=$3
		  AND build_id IN (SELECT id FROM recent_builds)
		  AND history_id != ''
		GROUP BY history_id
		ORDER BY avg_duration DESC
		LIMIT $4`

	rows, err := ts.pool.Query(ctx, query, projectID, builds, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("list slowest: %w", err)
	}

	var tests []store.LowPerformingTest
	for rows.Next() {
		var lt store.LowPerformingTest
		if err := rows.Scan(&lt.HistoryID, &lt.TestName, &lt.FullName, &lt.Metric, &lt.BuildCount); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan slowest: %w", err)
		}
		tests = append(tests, lt)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate slowest: %w", err)
	}
	rows.Close()

	historyIDs := make([]string, len(tests))
	for i := range tests {
		historyIDs[i] = tests[i].HistoryID
	}
	trends, _ := ts.batchTrendDuration(ctx, projectID, historyIDs, builds)
	for i := range tests {
		tests[i].Trend = trends[tests[i].HistoryID]
	}
	return tests, nil
}

// ListLeastReliable returns tests ranked by failure rate across the last N builds.
func (ts *PGTestResultStore) ListLeastReliable(ctx context.Context, projectID string, builds, limit int) ([]store.LowPerformingTest, error) {
	query := `
		WITH recent_builds AS (
			SELECT id FROM builds
			WHERE project_id=$1
			ORDER BY build_order DESC
			LIMIT $2
		)
		SELECT history_id,
		       MAX(test_name)  AS test_name,
		       MAX(full_name)  AS full_name,
		       SUM(CASE WHEN status IN ('failed','broken') THEN 1 ELSE 0 END)::float8
		           / COUNT(*)::float8 AS failure_rate,
		       COUNT(DISTINCT build_id) AS build_count
		FROM test_results
		WHERE project_id=$3
		  AND build_id IN (SELECT id FROM recent_builds)
		  AND history_id != ''
		GROUP BY history_id
		HAVING SUM(CASE WHEN status IN ('failed','broken') THEN 1 ELSE 0 END)::float8
		           / COUNT(*)::float8 > 0
		ORDER BY failure_rate DESC
		LIMIT $4`

	rows, err := ts.pool.Query(ctx, query, projectID, builds, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("list least reliable: %w", err)
	}

	var tests []store.LowPerformingTest
	for rows.Next() {
		var lt store.LowPerformingTest
		if err := rows.Scan(&lt.HistoryID, &lt.TestName, &lt.FullName, &lt.Metric, &lt.BuildCount); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan least reliable: %w", err)
		}
		tests = append(tests, lt)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("iterate least reliable: %w", err)
	}
	rows.Close()

	historyIDs := make([]string, len(tests))
	for i := range tests {
		historyIDs[i] = tests[i].HistoryID
	}
	trends, _ := ts.batchTrendFailureRate(ctx, projectID, historyIDs, builds)
	for i := range tests {
		tests[i].Trend = trends[tests[i].HistoryID]
	}
	return tests, nil
}

// buildPGPlaceholders builds "$start,$start+1,..." for count items.
func buildPGPlaceholders(start, count int) string {
	parts := make([]string, count)
	for i := range parts {
		parts[i] = fmt.Sprintf("$%d", start+i)
	}
	return strings.Join(parts, ",")
}

// batchTrendDuration returns per-build average duration for multiple tests, keyed by history_id.
func (ts *PGTestResultStore) batchTrendDuration(ctx context.Context, projectID string, historyIDs []string, builds int) (map[string][]float64, error) {
	if len(historyIDs) == 0 {
		return nil, nil
	}

	// Fixed args: $1=projectID, $2=builds, $3=projectID; historyIDs start at $4.
	args := make([]any, 0, 3+len(historyIDs))
	args = append(args, projectID, builds, projectID)
	for _, hid := range historyIDs {
		args = append(args, hid)
	}

	query := fmt.Sprintf(`
		WITH recent_builds AS (
			SELECT id FROM builds
			WHERE project_id=$1
			ORDER BY build_order DESC
			LIMIT $2
		)
		SELECT history_id, AVG(duration_ms::float8)
		FROM test_results
		WHERE project_id=$3
		  AND history_id IN (%s)
		  AND build_id IN (SELECT id FROM recent_builds)
		GROUP BY history_id, build_id
		ORDER BY history_id, build_id ASC`, buildPGPlaceholders(4, len(historyIDs)))

	rows, err := ts.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]float64, len(historyIDs))
	for rows.Next() {
		var hid string
		var v float64
		if err := rows.Scan(&hid, &v); err != nil {
			return nil, err
		}
		result[hid] = append(result[hid], v)
	}
	return result, rows.Err()
}

// batchTrendFailureRate returns per-build failure rate for multiple tests, keyed by history_id.
func (ts *PGTestResultStore) batchTrendFailureRate(ctx context.Context, projectID string, historyIDs []string, builds int) (map[string][]float64, error) {
	if len(historyIDs) == 0 {
		return nil, nil
	}

	args := make([]any, 0, 3+len(historyIDs))
	args = append(args, projectID, builds, projectID)
	for _, hid := range historyIDs {
		args = append(args, hid)
	}

	query := fmt.Sprintf(`
		WITH recent_builds AS (
			SELECT id FROM builds
			WHERE project_id=$1
			ORDER BY build_order DESC
			LIMIT $2
		)
		SELECT history_id,
		       SUM(CASE WHEN status IN ('failed','broken') THEN 1 ELSE 0 END)::float8
		           / COUNT(*)::float8
		FROM test_results
		WHERE project_id=$3
		  AND history_id IN (%s)
		  AND build_id IN (SELECT id FROM recent_builds)
		GROUP BY history_id, build_id
		ORDER BY history_id, build_id ASC`, buildPGPlaceholders(4, len(historyIDs)))

	rows, err := ts.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]float64, len(historyIDs))
	for rows.Next() {
		var hid string
		var v float64
		if err := rows.Scan(&hid, &v); err != nil {
			return nil, err
		}
		result[hid] = append(result[hid], v)
	}
	return result, rows.Err()
}

// ListTimeline returns timeline data for a specific build, ordered by start time.
func (ts *PGTestResultStore) ListTimeline(ctx context.Context, projectID string, buildID int64, limit int) ([]store.TimelineRow, error) {
	rows, err := ts.pool.Query(ctx, `
		SELECT test_name, full_name, status, start_ms, stop_ms, thread, host
		FROM test_results
		WHERE build_id=$1 AND project_id=$2 AND start_ms IS NOT NULL
		ORDER BY start_ms ASC
		LIMIT $3`, buildID, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("list timeline: %w", err)
	}
	defer rows.Close()

	var result []store.TimelineRow
	for rows.Next() {
		var r store.TimelineRow
		if err := rows.Scan(&r.TestName, &r.FullName, &r.Status, &r.StartMs, &r.StopMs, &r.Thread, &r.Host); err != nil {
			return nil, fmt.Errorf("scan timeline row: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate timeline: %w", err)
	}
	if result == nil {
		result = []store.TimelineRow{}
	}
	return result, nil
}

// ListFailedByBuild returns failed+broken tests for a build, ordered by duration DESC.
func (ts *PGTestResultStore) ListFailedByBuild(ctx context.Context, projectID string, buildID int64, limit int) ([]store.TestResult, error) {
	rows, err := ts.pool.Query(ctx, `
		SELECT build_id, project_id, test_name, full_name, status, duration_ms,
		       history_id, flaky, retries, new_failed, new_passed
		FROM test_results
		WHERE build_id=$1 AND project_id=$2 AND status IN ('failed','broken')
		ORDER BY duration_ms DESC
		LIMIT $3`, buildID, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("list failed by build: %w", err)
	}
	defer rows.Close()

	var results []store.TestResult
	for rows.Next() {
		var r store.TestResult
		if err := rows.Scan(
			&r.BuildID, &r.ProjectID, &r.TestName, &r.FullName, &r.Status, &r.DurationMs,
			&r.HistoryID, &r.Flaky, &r.Retries, &r.NewFailed, &r.NewPassed,
		); err != nil {
			return nil, fmt.Errorf("scan failed test result: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate failed results: %w", err)
	}
	if results == nil {
		results = []store.TestResult{}
	}
	return results, nil
}

// GetTestHistory returns the run history for a test identified by historyID.
func (ts *PGTestResultStore) GetTestHistory(ctx context.Context, projectID, historyID string, branchID *int64, limit int) ([]store.TestHistoryEntry, error) {
	var rows pgx.Rows
	var err error

	if branchID != nil {
		rows, err = ts.pool.Query(ctx, `
			SELECT b.build_order, b.id, tr.status, tr.duration_ms, b.created_at, b.ci_commit_sha
			FROM test_results tr
			JOIN builds b ON tr.build_id=b.id
			WHERE tr.project_id=$1 AND tr.history_id=$2 AND b.branch_id=$3
			ORDER BY b.build_order DESC
			LIMIT $4`, projectID, historyID, *branchID, limit)
	} else {
		rows, err = ts.pool.Query(ctx, `
			SELECT b.build_order, b.id, tr.status, tr.duration_ms, b.created_at, b.ci_commit_sha
			FROM test_results tr
			JOIN builds b ON tr.build_id=b.id
			WHERE tr.project_id=$1 AND tr.history_id=$2
			ORDER BY b.build_order DESC
			LIMIT $3`, projectID, historyID, limit)
	}
	if err != nil {
		return nil, fmt.Errorf("get test history: %w", err)
	}
	defer rows.Close()

	var entries []store.TestHistoryEntry
	for rows.Next() {
		var e store.TestHistoryEntry
		var createdAt time.Time
		var ciCommitSHA *string
		if err := rows.Scan(&e.BuildOrder, &e.BuildID, &e.Status, &e.DurationMs, &createdAt, &ciCommitSHA); err != nil {
			return nil, fmt.Errorf("scan test history row: %w", err)
		}
		e.CreatedAt = createdAt
		e.CICommitSHA = ciCommitSHA
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate test history: %w", err)
	}
	if entries == nil {
		entries = []store.TestHistoryEntry{}
	}
	return entries, nil
}

// DeleteByBuild removes all test results for a specific build.
func (ts *PGTestResultStore) DeleteByBuild(ctx context.Context, buildID int64) error {
	_, err := ts.pool.Exec(ctx, "DELETE FROM test_results WHERE build_id=$1", buildID)
	if err != nil {
		return fmt.Errorf("delete test results for build %d: %w", buildID, err)
	}
	return nil
}

// DeleteByProject removes all test results for the given project.
func (ts *PGTestResultStore) DeleteByProject(ctx context.Context, projectID string) error {
	_, err := ts.pool.Exec(ctx, "DELETE FROM test_results WHERE project_id=$1", projectID)
	if err != nil {
		return fmt.Errorf("delete test results for project %q: %w", projectID, err)
	}
	return nil
}

// InsertBatchFull stores fully-parsed Allure results in a single transaction.
// For each result it inserts into test_results (returning the new id), then
// inserts labels, parameters, steps (recursive), and attachments.
func (ts *PGTestResultStore) InsertBatchFull(ctx context.Context, buildID int64, projectID string, results []*parser.Result) error {
	if len(results) == 0 {
		return nil
	}
	tx, err := ts.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	for _, r := range results {
		var testResultID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO test_results
				(build_id, project_id, test_name, full_name, status, duration_ms,
				 history_id, start_ms, stop_ms, status_message, description)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)
			ON CONFLICT (build_id, history_id) WHERE history_id != ''
			DO UPDATE SET
				status_message = EXCLUDED.status_message,
				description    = EXCLUDED.description
			RETURNING id`,
			buildID, projectID, r.Name, r.FullName, r.Status, r.StopMs-r.StartMs,
			r.HistoryID, r.StartMs, r.StopMs, r.StatusMessage, r.Description,
		).Scan(&testResultID)
		if err != nil {
			return fmt.Errorf("insert test result %q: %w", r.Name, err)
		}

		for _, lbl := range r.Labels {
			if _, err := tx.Exec(ctx,
				`INSERT INTO test_labels(test_result_id, name, value) VALUES ($1,$2,$3)`,
				testResultID, lbl.Name, lbl.Value,
			); err != nil {
				return fmt.Errorf("insert label: %w", err)
			}
		}

		for _, param := range r.Parameters {
			if _, err := tx.Exec(ctx,
				`INSERT INTO test_parameters(test_result_id, name, value) VALUES ($1,$2,$3)`,
				testResultID, param.Name, param.Value,
			); err != nil {
				return fmt.Errorf("insert parameter: %w", err)
			}
		}

		if err := insertSteps(ctx, tx, testResultID, nil, r.Steps); err != nil {
			return fmt.Errorf("insert steps for %q: %w", r.Name, err)
		}

		for _, att := range r.Attachments {
			if _, err := tx.Exec(ctx,
				`INSERT INTO test_attachments(test_result_id, name, source, mime_type, size_bytes)
				 VALUES ($1,$2,$3,$4,$5)`,
				testResultID, att.Name, att.Source, att.MimeType, 0,
			); err != nil {
				return fmt.Errorf("insert attachment: %w", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// insertSteps recursively inserts steps and their children into test_steps.
func insertSteps(ctx context.Context, tx pgx.Tx, testResultID int64, parentStepID *int64, steps []parser.Step) error {
	for i, step := range steps {
		var stepID int64
		err := tx.QueryRow(ctx,
			`INSERT INTO test_steps(test_result_id, parent_step_id, name, status, status_message, duration_ms, step_order)
			 VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id`,
			testResultID, parentStepID, step.Name, step.Status, step.StatusMessage, step.DurationMs, i,
		).Scan(&stepID)
		if err != nil {
			return fmt.Errorf("insert step %q: %w", step.Name, err)
		}

		for _, att := range step.Attachments {
			if _, err := tx.Exec(ctx,
				`INSERT INTO test_attachments(test_result_id, test_step_id, name, source, mime_type, size_bytes)
				 VALUES ($1,$2,$3,$4,$5,$6)`,
				testResultID, stepID, att.Name, att.Source, att.MimeType, 0,
			); err != nil {
				return fmt.Errorf("insert step attachment: %w", err)
			}
		}

		if err := insertSteps(ctx, tx, testResultID, &stepID, step.Steps); err != nil {
			return err
		}
	}
	return nil
}

var _ store.TestResultStorer = (*PGTestResultStore)(nil)
