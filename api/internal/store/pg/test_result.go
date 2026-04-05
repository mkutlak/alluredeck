package pg

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/parser"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// TestResultStore provides operations on the test_results table using PostgreSQL.
type TestResultStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewTestResultStore creates a TestResultStore backed by the given PGStore.
func NewTestResultStore(s *PGStore, logger *zap.Logger) *TestResultStore {
	return &TestResultStore{pool: s.pool, logger: logger}
}

// InsertBatch inserts all results in a single transaction. Returns nil for empty slice.
func (ts *TestResultStore) InsertBatch(ctx context.Context, results []store.TestResult) error {
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
func (ts *TestResultStore) GetBuildID(ctx context.Context, projectID string, buildNumber int) (int64, error) {
	var id int64
	err := ts.pool.QueryRow(ctx,
		"SELECT id FROM builds WHERE project_id=$1 AND build_order=$2", projectID, buildNumber,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("get build id: %w", err)
	}
	return id, nil
}

// ListSlowest returns tests ranked by average duration across the last N builds.
func (ts *TestResultStore) ListSlowest(ctx context.Context, projectID string, builds, limit int, branchID *int64) ([]store.LowPerformingTest, error) {
	recentCTE := "SELECT id FROM builds WHERE project_id=$1 ORDER BY build_order DESC LIMIT $2"
	args := []any{projectID, builds, projectID, limit}
	if branchID != nil {
		recentCTE = "SELECT id FROM builds WHERE project_id=$1 AND branch_id=$5 ORDER BY build_order DESC LIMIT $2"
		args = append(args, *branchID)
	}
	query := fmt.Sprintf(`
		WITH recent_builds AS (%s)
		SELECT MAX(history_id) AS history_id,
		       MAX(test_name)  AS test_name,
		       full_name,
		       AVG(duration_ms::float8) AS avg_duration,
		       COUNT(DISTINCT build_id) AS build_count
		FROM test_results
		WHERE project_id=$3
		  AND build_id IN (SELECT id FROM recent_builds)
		  AND full_name != ''
		GROUP BY full_name
		ORDER BY avg_duration DESC
		LIMIT $4`, recentCTE)

	rows, err := ts.pool.Query(ctx, query, args...)
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

	fullNames := make([]string, len(tests))
	for i := range tests {
		fullNames[i] = tests[i].FullName
	}
	trends, _ := ts.batchTrendDuration(ctx, projectID, fullNames, builds, branchID)
	for i := range tests {
		tests[i].Trend = trends[tests[i].FullName]
	}
	return tests, nil
}

// ListLeastReliable returns tests ranked by failure rate across the last N builds.
func (ts *TestResultStore) ListLeastReliable(ctx context.Context, projectID string, builds, limit int, branchID *int64) ([]store.LowPerformingTest, error) {
	recentCTE := "SELECT id FROM builds WHERE project_id=$1 ORDER BY build_order DESC LIMIT $2"
	args := []any{projectID, builds, projectID, limit}
	if branchID != nil {
		recentCTE = "SELECT id FROM builds WHERE project_id=$1 AND branch_id=$5 ORDER BY build_order DESC LIMIT $2"
		args = append(args, *branchID)
	}
	query := fmt.Sprintf(`
		WITH recent_builds AS (%s)
		SELECT MAX(history_id) AS history_id,
		       MAX(test_name)  AS test_name,
		       full_name,
		       SUM(CASE WHEN status IN ('failed','broken') THEN 1 ELSE 0 END)::float8
		           / COUNT(*)::float8 AS failure_rate,
		       COUNT(DISTINCT build_id) AS build_count
		FROM test_results
		WHERE project_id=$3
		  AND build_id IN (SELECT id FROM recent_builds)
		  AND full_name != ''
		GROUP BY full_name
		HAVING SUM(CASE WHEN status IN ('failed','broken') THEN 1 ELSE 0 END)::float8
		           / COUNT(*)::float8 > 0
		ORDER BY failure_rate DESC
		LIMIT $4`, recentCTE)

	rows, err := ts.pool.Query(ctx, query, args...)
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

	fullNames := make([]string, len(tests))
	for i := range tests {
		fullNames[i] = tests[i].FullName
	}
	trends, _ := ts.batchTrendFailureRate(ctx, projectID, fullNames, builds, branchID)
	for i := range tests {
		tests[i].Trend = trends[tests[i].FullName]
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

// batchTrendDuration returns per-build average duration for multiple tests, keyed by full_name.
func (ts *TestResultStore) batchTrendDuration(ctx context.Context, projectID string, fullNames []string, builds int, branchID *int64) (map[string][]float64, error) {
	if len(fullNames) == 0 {
		return nil, nil
	}

	// Fixed args: $1=projectID, $2=builds, $3=projectID; branchID at $4 if set; fullNames start at $4 or $5.
	var recentCTE string
	args := make([]any, 0, 3+len(fullNames)+1)
	args = append(args, projectID, builds, projectID)
	var paramStart int
	if branchID != nil {
		recentCTE = "SELECT id FROM builds WHERE project_id=$1 AND branch_id=$4 ORDER BY build_order DESC LIMIT $2"
		args = append(args, *branchID)
		paramStart = 5
	} else {
		recentCTE = "SELECT id FROM builds WHERE project_id=$1 ORDER BY build_order DESC LIMIT $2"
		paramStart = 4
	}
	for _, fn := range fullNames {
		args = append(args, fn)
	}

	query := fmt.Sprintf(`
		WITH recent_builds AS (%s)
		SELECT full_name, AVG(duration_ms::float8)
		FROM test_results
		WHERE project_id=$3
		  AND full_name IN (%s)
		  AND build_id IN (SELECT id FROM recent_builds)
		GROUP BY full_name, build_id
		ORDER BY full_name, build_id ASC`, recentCTE, buildPGPlaceholders(paramStart, len(fullNames)))

	rows, err := ts.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]float64, len(fullNames))
	for rows.Next() {
		var fn string
		var v float64
		if err := rows.Scan(&fn, &v); err != nil {
			return nil, err
		}
		result[fn] = append(result[fn], v)
	}
	return result, rows.Err()
}

// batchTrendFailureRate returns per-build failure rate for multiple tests, keyed by full_name.
func (ts *TestResultStore) batchTrendFailureRate(ctx context.Context, projectID string, fullNames []string, builds int, branchID *int64) (map[string][]float64, error) {
	if len(fullNames) == 0 {
		return nil, nil
	}

	var recentCTE string
	args := make([]any, 0, 3+len(fullNames)+1)
	args = append(args, projectID, builds, projectID)
	var paramStart int
	if branchID != nil {
		recentCTE = "SELECT id FROM builds WHERE project_id=$1 AND branch_id=$4 ORDER BY build_order DESC LIMIT $2"
		args = append(args, *branchID)
		paramStart = 5
	} else {
		recentCTE = "SELECT id FROM builds WHERE project_id=$1 ORDER BY build_order DESC LIMIT $2"
		paramStart = 4
	}
	for _, fn := range fullNames {
		args = append(args, fn)
	}

	query := fmt.Sprintf(`
		WITH recent_builds AS (%s)
		SELECT full_name,
		       SUM(CASE WHEN status IN ('failed','broken') THEN 1 ELSE 0 END)::float8
		           / COUNT(*)::float8
		FROM test_results
		WHERE project_id=$3
		  AND full_name IN (%s)
		  AND build_id IN (SELECT id FROM recent_builds)
		GROUP BY full_name, build_id
		ORDER BY full_name, build_id ASC`, recentCTE, buildPGPlaceholders(paramStart, len(fullNames)))

	rows, err := ts.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string][]float64, len(fullNames))
	for rows.Next() {
		var fn string
		var v float64
		if err := rows.Scan(&fn, &v); err != nil {
			return nil, err
		}
		result[fn] = append(result[fn], v)
	}
	return result, rows.Err()
}

// ListTimeline returns timeline data for a specific build, ordered by start time.
func (ts *TestResultStore) ListTimeline(ctx context.Context, projectID string, buildID int64, limit int) ([]store.TimelineRow, error) {
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

// ListTimelineMulti returns timeline data across multiple builds, ordered by build_order ASC then start_ms ASC.
func (ts *TestResultStore) ListTimelineMulti(ctx context.Context, projectID string, buildIDs []int64, limit int) ([]store.MultiTimelineRow, error) {
	rows, err := ts.pool.Query(ctx, `
		SELECT tr.build_id, b.build_order, tr.test_name, tr.full_name, tr.status,
		       tr.start_ms, tr.stop_ms, tr.thread, tr.host
		FROM test_results tr
		JOIN builds b ON b.id = tr.build_id
		WHERE tr.build_id = ANY($1) AND tr.project_id = $2 AND tr.start_ms IS NOT NULL
		ORDER BY b.build_order ASC, tr.start_ms ASC
		LIMIT $3`, buildIDs, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("list timeline multi: %w", err)
	}
	defer rows.Close()

	var result []store.MultiTimelineRow
	for rows.Next() {
		var r store.MultiTimelineRow
		if err := rows.Scan(&r.BuildID, &r.BuildNumber, &r.TestName, &r.FullName, &r.Status, &r.StartMs, &r.StopMs, &r.Thread, &r.Host); err != nil {
			return nil, fmt.Errorf("scan multi timeline row: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate multi timeline: %w", err)
	}
	return result, nil
}

// ListFailedByBuild returns failed+broken tests for a build, ordered by duration DESC.
func (ts *TestResultStore) ListFailedByBuild(ctx context.Context, projectID string, buildID int64, limit int) ([]store.TestResult, error) {
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
func (ts *TestResultStore) GetTestHistory(ctx context.Context, projectID, historyID string, branchID *int64, limit int) ([]store.TestHistoryEntry, error) {
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
		if err := rows.Scan(&e.BuildNumber, &e.BuildID, &e.Status, &e.DurationMs, &createdAt, &ciCommitSHA); err != nil {
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
func (ts *TestResultStore) DeleteByBuild(ctx context.Context, buildID int64) error {
	_, err := ts.pool.Exec(ctx, "DELETE FROM test_results WHERE build_id=$1", buildID)
	if err != nil {
		return fmt.Errorf("delete test results for build %d: %w", buildID, err)
	}
	return nil
}

// DeleteByProject removes all test results for the given project.
func (ts *TestResultStore) DeleteByProject(ctx context.Context, projectID string) error {
	_, err := ts.pool.Exec(ctx, "DELETE FROM test_results WHERE project_id=$1", projectID)
	if err != nil {
		return fmt.Errorf("delete test results for project %q: %w", projectID, err)
	}
	return nil
}

// InsertBatchFull stores fully-parsed Allure results in a single transaction.
// For each result it inserts into test_results (returning the new id), then
// inserts labels, parameters, steps (recursive), and attachments.
func (ts *TestResultStore) InsertBatchFull(ctx context.Context, buildID int64, projectID string, results []*parser.Result) error {
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
				 history_id, start_ms, stop_ms, status_message, status_trace, description)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
			ON CONFLICT (build_id, history_id) WHERE history_id != ''
			DO UPDATE SET
				status_message = EXCLUDED.status_message,
				status_trace   = EXCLUDED.status_trace,
				description    = EXCLUDED.description
			RETURNING id`,
			buildID, projectID, r.Name, r.FullName, r.Status, r.StopMs-r.StartMs,
			r.HistoryID, r.StartMs, r.StopMs, r.StatusMessage, r.StatusTrace, r.Description,
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
				testResultID, att.Name, att.Source, att.MimeType, att.Size,
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
				testResultID, stepID, att.Name, att.Source, att.MimeType, att.Size,
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

// ListFailedForFingerprinting returns failed test results for a build, providing
// the minimal fields needed for fingerprint heuristics (ID, status message, trace).
func (ts *TestResultStore) ListFailedForFingerprinting(ctx context.Context, projectID string, buildID int64) ([]store.FailedTestResult, error) {
	rows, err := ts.pool.Query(ctx, `
		SELECT tr.id, tr.status_message, tr.status_trace
		FROM test_results tr
		WHERE tr.build_id = $1
		  AND tr.project_id = $2
		  AND tr.status IN ('failed', 'broken')
		ORDER BY tr.id
	`, buildID, projectID)
	if err != nil {
		return nil, fmt.Errorf("list failed for fingerprinting: %w", err)
	}
	defer rows.Close()

	var results []store.FailedTestResult
	for rows.Next() {
		var r store.FailedTestResult
		if err := rows.Scan(&r.ID, &r.StatusMessage, &r.StatusTrace); err != nil {
			return nil, fmt.Errorf("scan failed test result: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

var _ store.TestResultStorer = (*TestResultStore)(nil)
