package pg

import (
	"context"
	"errors"
	"fmt"
	"sort"
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
		// A single build can legitimately contain several results that share
		// the same non-empty historyId: Allure writes one *-result.json per
		// retry/flaky attempt, so parseStabilityEntries emits one entry per
		// attempt. Those collide on the idx_test_results_build_history partial
		// unique index. Collapse them to one row per (build_id, history_id),
		// keeping the latest attempt (greatest stop_ms) so a retried test
		// records its final outcome. The guard makes the winner independent of
		// batch order, and an older attempt arriving second is a no-op rather
		// than aborting the whole transaction. Empty historyIds are excluded by
		// the index predicate and so are never collapsed.
		if _, err := tx.Exec(ctx, `
			INSERT INTO test_results
				(build_id, project_id, test_name, full_name, status, duration_ms,
				 history_id, flaky, retries, new_failed, new_passed,
				 start_ms, stop_ms, thread, host)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			ON CONFLICT (build_id, history_id) WHERE history_id != ''
			DO UPDATE SET
				test_name   = EXCLUDED.test_name,
				full_name   = EXCLUDED.full_name,
				status      = EXCLUDED.status,
				duration_ms = EXCLUDED.duration_ms,
				flaky       = EXCLUDED.flaky,
				retries     = EXCLUDED.retries,
				new_failed  = EXCLUDED.new_failed,
				new_passed  = EXCLUDED.new_passed,
				start_ms    = EXCLUDED.start_ms,
				stop_ms     = EXCLUDED.stop_ms,
				thread      = EXCLUDED.thread,
				host        = EXCLUDED.host
			WHERE COALESCE(EXCLUDED.stop_ms, 0) >= COALESCE(test_results.stop_ms, 0)`,
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
func (ts *TestResultStore) GetBuildID(ctx context.Context, projectID int64, buildNumber int) (int64, error) {
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
func (ts *TestResultStore) ListSlowest(ctx context.Context, projectID int64, builds, limit int, branchID *int64) ([]store.LowPerformingTest, error) {
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
func (ts *TestResultStore) ListLeastReliable(ctx context.Context, projectID int64, builds, limit int, branchID *int64) ([]store.LowPerformingTest, error) {
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
func (ts *TestResultStore) batchTrendDuration(ctx context.Context, projectID int64, fullNames []string, builds int, branchID *int64) (map[string][]float64, error) {
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
func (ts *TestResultStore) batchTrendFailureRate(ctx context.Context, projectID int64, fullNames []string, builds int, branchID *int64) (map[string][]float64, error) {
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
func (ts *TestResultStore) ListTimeline(ctx context.Context, projectID int64, buildID int64, limit int) ([]store.TimelineRow, error) {
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
func (ts *TestResultStore) ListTimelineMulti(ctx context.Context, projectID int64, buildIDs []int64, limit int) ([]store.MultiTimelineRow, error) {
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
// status_message is bounded to 500 characters — callers only need enough of the
// message for a short error preview, not the full text. GetReportKnownFailures
// also calls this with a limit of 10000, so every row it fetches now carries a
// status_message capped at 500 chars rather than the full column value.
//
// id ASC breaks duration ties. Durations collide often (fast tests round to the
// same millisecond, skipped ones are all zero), and without a second sort key
// PostgreSQL is free to return tied rows in any order — so which of them fall
// inside the LIMIT window could change between two runs over identical data.
// Callers that derive a verdict from this list need it to be reproducible.
func (ts *TestResultStore) ListFailedByBuild(ctx context.Context, projectID int64, buildID int64, limit int) ([]store.TestResult, error) {
	rows, err := ts.pool.Query(ctx, `
		SELECT id, build_id, project_id, test_name, full_name, status, duration_ms,
		       history_id, flaky, retries, new_failed, new_passed,
		       COALESCE(LEFT(status_message, 500), '')
		FROM test_results
		WHERE build_id=$1 AND project_id=$2 AND status IN ('failed','broken')
		ORDER BY duration_ms DESC, id ASC
		LIMIT $3`, buildID, projectID, limit)
	if err != nil {
		return nil, fmt.Errorf("list failed by build: %w", err)
	}
	defer rows.Close()

	var results []store.TestResult
	for rows.Next() {
		var r store.TestResult
		if err := rows.Scan(
			&r.ID, &r.BuildID, &r.ProjectID, &r.TestName, &r.FullName, &r.Status, &r.DurationMs,
			&r.HistoryID, &r.Flaky, &r.Retries, &r.NewFailed, &r.NewPassed,
			&r.StatusMessage,
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

// CountFailedByBuild returns the number of distinct failing tests in a build.
// It counts DISTINCT full_name, not rows, because the table can still hold two
// rows per test: an enriched row and an empty shell carrying a second
// history_id scheme, which would make COUNT(*) report twice the failures a
// human sees. Ingestion no longer produces those twins — the Allure path
// reconciles both writes onto one history_id (historyIDsByFullName in
// api/internal/runner/allure.go) and the Playwright path always wrote a single
// scheme — but builds ingested before that fix keep their twins until migration
// 0049 deletes them, and this count has to be right on historical data too.
// full_name is the stable per-test identity across both schemes.
func (ts *TestResultStore) CountFailedByBuild(ctx context.Context, projectID int64, buildID int64) (int, error) {
	var n int
	if err := ts.pool.QueryRow(ctx, `
		SELECT COUNT(DISTINCT full_name)
		FROM test_results
		WHERE build_id=$1 AND project_id=$2 AND status IN ('failed','broken')`,
		buildID, projectID,
	).Scan(&n); err != nil {
		return 0, fmt.Errorf("count failed by build: %w", err)
	}
	return n, nil
}

// GetByHistoryID returns the test_results row identified by (projectID,
// buildID, historyID) regardless of status, so a caller can look up a test
// that passed as readily as one that failed. status_message is bounded to 500
// characters, matching ListFailedByBuild. Returns (nil, nil) when no row
// matches, and immediately when historyID is empty — an empty history_id
// matches every test lacking one and so is never a valid lookup key.
func (ts *TestResultStore) GetByHistoryID(ctx context.Context, projectID int64, buildID int64, historyID string) (*store.TestResult, error) {
	if historyID == "" {
		return nil, nil
	}
	var r store.TestResult
	err := ts.pool.QueryRow(ctx, `
		SELECT id, build_id, project_id, test_name, full_name, status, duration_ms,
		       history_id, flaky, retries, new_failed, new_passed,
		       COALESCE(LEFT(status_message, 500), '')
		FROM test_results
		WHERE project_id=$1 AND build_id=$2 AND history_id=$3
		LIMIT 1`, projectID, buildID, historyID,
	).Scan(
		&r.ID, &r.BuildID, &r.ProjectID, &r.TestName, &r.FullName, &r.Status, &r.DurationMs,
		&r.HistoryID, &r.Flaky, &r.Retries, &r.NewFailed, &r.NewPassed,
		&r.StatusMessage,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get test result by history id: %w", err)
	}
	return &r, nil
}

// ListStabilityByBuild returns tests with stability signals (flaky, retried, new-failed, new-passed) for a build.
func (ts *TestResultStore) ListStabilityByBuild(ctx context.Context, projectID int64, buildID int64) ([]store.TestResult, error) {
	rows, err := ts.pool.Query(ctx, `
		SELECT build_id, project_id, test_name, full_name, status, duration_ms,
		       history_id, flaky, retries, new_failed, new_passed
		FROM test_results
		WHERE build_id=$1 AND project_id=$2
		  AND (flaky = true OR retries > 0 OR new_failed = true OR new_passed = true)
		ORDER BY test_name`, buildID, projectID)
	if err != nil {
		return nil, fmt.Errorf("list stability by build: %w", err)
	}
	defer rows.Close()

	var results []store.TestResult
	for rows.Next() {
		var r store.TestResult
		if err := rows.Scan(
			&r.BuildID, &r.ProjectID, &r.TestName, &r.FullName, &r.Status, &r.DurationMs,
			&r.HistoryID, &r.Flaky, &r.Retries, &r.NewFailed, &r.NewPassed,
		); err != nil {
			return nil, fmt.Errorf("scan stability result: %w", err)
		}
		results = append(results, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate stability results: %w", err)
	}
	if results == nil {
		results = []store.TestResult{}
	}
	return results, nil
}

// GetTestHistory returns the run history for a test identified by historyID.
func (ts *TestResultStore) GetTestHistory(ctx context.Context, projectID int64, historyID string, branchID *int64, limit int) ([]store.TestHistoryEntry, error) {
	var rows pgx.Rows
	var err error

	// br is LEFT-joined: builds predating branch tracking have a NULL branch_id
	// and must still appear in the history, with an empty branch name.
	if branchID != nil {
		rows, err = ts.pool.Query(ctx, `
			SELECT b.build_order, b.id, tr.status, tr.duration_ms, b.created_at, b.ci_commit_sha, tr.flaky, tr.retries,
			       COALESCE(br.name, '')
			FROM test_results tr
			JOIN builds b ON tr.build_id=b.id
			LEFT JOIN branches br ON b.branch_id=br.id
			WHERE tr.project_id=$1 AND tr.history_id=$2 AND b.branch_id=$3
			ORDER BY b.build_order DESC
			LIMIT $4`, projectID, historyID, *branchID, limit)
	} else {
		rows, err = ts.pool.Query(ctx, `
			SELECT b.build_order, b.id, tr.status, tr.duration_ms, b.created_at, b.ci_commit_sha, tr.flaky, tr.retries,
			       COALESCE(br.name, '')
			FROM test_results tr
			JOIN builds b ON tr.build_id=b.id
			LEFT JOIN branches br ON b.branch_id=br.id
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
		if err := rows.Scan(&e.BuildNumber, &e.BuildID, &e.Status, &e.DurationMs, &createdAt, &ciCommitSHA, &e.Flaky, &e.Retries, &e.BranchName); err != nil {
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

// GetLastPassingBuild returns the most recent build strictly before
// beforeBuildOrder in which the test identified by historyID passed, scoped to
// branchID when non-nil. It mirrors GetTestHistory's join and branch-scoping
// clause. beforeBuildOrder is a builds.build_order (NOT builds.id): builds are
// not guaranteed to be ingested in build_order sequence (backfills can insert
// an older build after newer ones already exist, so the IDENTITY-generated
// b.id does not reliably track build_order), so the predicate and ORDER BY
// both key on b.build_order for correctness — matching GetTestHistory's own
// ordering column. Returns (nil, nil) when the test has no prior passing
// build, or immediately when historyID is empty: an empty history_id matches
// every test lacking one, so it must never be used as a lookup key.
func (ts *TestResultStore) GetLastPassingBuild(ctx context.Context, projectID int64, historyID string, branchID *int64, beforeBuildOrder int) (*store.TestHistoryEntry, error) {
	if historyID == "" {
		return nil, nil
	}

	// br is LEFT-joined for the same reason as in GetTestHistory: a pass on a
	// build with no branch_id is still a pass.
	var row pgx.Row
	if branchID != nil {
		row = ts.pool.QueryRow(ctx, `
			SELECT b.build_order, b.id, tr.status, tr.duration_ms, b.created_at, b.ci_commit_sha, tr.flaky, tr.retries,
			       COALESCE(br.name, '')
			FROM test_results tr
			JOIN builds b ON tr.build_id=b.id
			LEFT JOIN branches br ON b.branch_id=br.id
			WHERE tr.project_id=$1 AND tr.history_id=$2 AND tr.status=$3
			  AND b.branch_id=$4 AND b.build_order < $5
			ORDER BY b.build_order DESC
			LIMIT 1`, projectID, historyID, string(store.TestStatusPassed), *branchID, beforeBuildOrder)
	} else {
		row = ts.pool.QueryRow(ctx, `
			SELECT b.build_order, b.id, tr.status, tr.duration_ms, b.created_at, b.ci_commit_sha, tr.flaky, tr.retries,
			       COALESCE(br.name, '')
			FROM test_results tr
			JOIN builds b ON tr.build_id=b.id
			LEFT JOIN branches br ON b.branch_id=br.id
			WHERE tr.project_id=$1 AND tr.history_id=$2 AND tr.status=$3
			  AND b.build_order < $4
			ORDER BY b.build_order DESC
			LIMIT 1`, projectID, historyID, string(store.TestStatusPassed), beforeBuildOrder)
	}

	var e store.TestHistoryEntry
	var createdAt time.Time
	var ciCommitSHA *string
	if err := row.Scan(&e.BuildNumber, &e.BuildID, &e.Status, &e.DurationMs, &createdAt, &ciCommitSHA, &e.Flaky, &e.Retries, &e.BranchName); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get last passing build: %w", err)
	}
	e.CreatedAt = createdAt
	e.CICommitSHA = ciCommitSHA
	return &e, nil
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
func (ts *TestResultStore) DeleteByProject(ctx context.Context, projectID int64) error {
	_, err := ts.pool.Exec(ctx, "DELETE FROM test_results WHERE project_id=$1", projectID)
	if err != nil {
		return fmt.Errorf("delete test results for project %d: %w", projectID, err)
	}
	return nil
}

// InsertBatchFull stores fully-parsed Allure results in a single transaction.
// For each result it inserts into test_results (returning the new id), then
// inserts labels, parameters, steps (recursive), and attachments.
// dedupeResultsByHistoryID collapses results that share the same non-empty
// historyId down to a single entry, keeping the latest attempt (greatest
// StopMs), with ties resolved in favour of the first result read. This mirrors
// the latest-attempt-wins rule InsertBatch applies in SQL, so the stability row
// and its enrichment come from the same Allure attempt.
// Entries with an empty historyId are excluded from the unique index and so are
// never collapsed — each is preserved in its original order.
// The collapsed siblings are not discarded outright: on the Allure path they
// ARE the per-attempt record. An Allure reporter writes one standalone
// *-result.json per attempt with no in-file retry structure, so "the attempts of
// a retried test" is expressed purely as several files sharing one historyId —
// exactly the group this function collapses. Each survivor therefore carries an
// Attempts slice synthesized from its whole group (see attemptsFromSiblings), so
// the retry-consistency signal works for Allure uploads and not only for
// Playwright ones, which get Attempts straight from the parser.
func dedupeResultsByHistoryID(results []*parser.Result) []*parser.Result {
	out := make([]*parser.Result, 0, len(results))
	groups := make([][]*parser.Result, 0, len(results)) // parallel to out
	idx := make(map[string]int, len(results))           // historyId -> index into out
	for _, r := range results {
		if r.HistoryID == "" {
			out = append(out, r)
			groups = append(groups, nil)
			continue
		}
		if i, ok := idx[r.HistoryID]; ok {
			groups[i] = append(groups[i], r)
			// Strictly greater, never equal: the survivor is the attempt with the
			// highest stop timestamp, and ties keep the FIRST result read. A
			// reporter may omit the stop timestamp entirely, which leaves StopMs
			// at zero, so ">=" would have made every untimestamped group resolve
			// on ParseDir's filename order — a different survivor for the same
			// upload depending on how the files happened to be named. Zero also
			// loses to any real timestamp under ">", so a timestamped attempt
			// always outranks an untimestamped one whichever is read first.
			if r.StopMs > out[i].StopMs {
				out[i] = r
			}
			continue
		}
		idx[r.HistoryID] = len(out)
		out = append(out, r)
		groups = append(groups, []*parser.Result{r})
	}

	for i, group := range groups {
		// A parser that supplied its own attempts (Playwright) is authoritative;
		// a lone result is not a retry sequence and is skipped so the common
		// single-attempt case allocates nothing.
		if len(group) < attemptsMinToPersist || len(out[i].Attempts) > 0 {
			continue
		}
		// Copy rather than mutate: the caller's slice elements are shared with
		// the ingestion path that produced them, which never asked for attempts
		// to be attached to its results.
		enriched := *out[i]
		enriched.Attempts = attemptsFromSiblings(group)
		out[i] = &enriched
	}
	return out
}

// attemptsFromSiblings turns the result files of one retried Allure test into an
// ordered attempt list. Attempts are ordered by StopMs ascending — the same
// "later attempt has the greater StopMs" rule the survivor selection above
// relies on — with ties broken by the original file order.
func attemptsFromSiblings(group []*parser.Result) []parser.Attempt {
	ordered := make([]*parser.Result, len(group))
	copy(ordered, group)
	sort.SliceStable(ordered, func(a, b int) bool { return ordered[a].StopMs < ordered[b].StopMs })

	attempts := make([]parser.Attempt, 0, len(ordered))
	for i, r := range ordered {
		attempts = append(attempts, parser.Attempt{
			Index:         i,
			Status:        r.Status,
			StatusMessage: r.StatusMessage,
		})
	}
	return attempts
}

func (ts *TestResultStore) InsertBatchFull(ctx context.Context, buildID int64, projectID int64, results []*parser.Result) error {
	if len(results) == 0 {
		return nil
	}
	// Allure writes one *-result.json per retry attempt, so ParseDir can return
	// several results sharing the same non-empty historyId. They all collapse
	// onto a single test_results row via the idx_test_results_build_history
	// ON CONFLICT below — but the loop also re-inserts each result's labels,
	// parameters, steps and attachments under the RETURNING id, so without
	// de-duplication every extra attempt would duplicate those child rows on the
	// surviving result. Collapse to the latest attempt (greatest StopMs) so the
	// enrichment matches the row InsertBatch keeps and children are written once.
	results = dedupeResultsByHistoryID(results)
	tx, err := ts.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Attempt rows are collected here and written once for the whole batch after
	// the loop, rather than per result inside it — see replaceAttempts.
	resultIDs := make([]int64, 0, len(results))
	var attempts []attemptRow

	for _, r := range results {
		var testResultID int64
		err := tx.QueryRow(ctx, `
			INSERT INTO test_results
				(build_id, project_id, test_name, full_name, status, duration_ms,
				 history_id, start_ms, stop_ms, status_message, status_trace, description,
				 flaky, retries)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
			ON CONFLICT (build_id, history_id) WHERE history_id != ''
			DO UPDATE SET
				status_message = EXCLUDED.status_message,
				status_trace   = EXCLUDED.status_trace,
				description    = EXCLUDED.description,
				-- Merge, don't clobber: InsertBatch already recorded the authoritative
				-- flaky/retries (from Allure stability entries or the Playwright runner).
				-- The Allure parser leaves these zero on parser.Result, so a plain
				-- EXCLUDED assignment here would wipe a real flaky flag back to false.
				flaky          = test_results.flaky OR EXCLUDED.flaky,
				retries        = GREATEST(test_results.retries, EXCLUDED.retries)
			RETURNING id`,
			buildID, projectID, r.Name, r.FullName, r.Status, r.StopMs-r.StartMs,
			r.HistoryID, r.StartMs, r.StopMs, r.StatusMessage, r.StatusTrace, r.Description,
			r.Flaky, r.Retries,
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

		resultIDs = append(resultIDs, testResultID)
		if len(r.Attempts) >= attemptsMinToPersist {
			for _, a := range r.Attempts {
				attempts = append(attempts, attemptRow{testResultID: testResultID, attempt: a})
			}
		}
	}

	if err := replaceAttempts(ctx, tx, resultIDs, attempts); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// attemptMessageMax bounds a stored attempt message. The full text of the final
// attempt already lives in test_results.status_message/status_trace; these rows
// only need enough text for attempts to be compared against each other.
const attemptMessageMax = 2000

// attemptsMinToPersist is the smallest attempt count worth a row. A lone
// attempt supports no comparison — triage classifies anything under two as
// "single" — and its outcome is already on the test_results row, so persisting
// one would add a row per test per build for information nothing reads.
const attemptsMinToPersist = 2

// attemptRow pairs one parsed attempt with the test_results row it belongs to,
// so a whole InsertBatchFull batch's attempts can be queued together after the
// per-result loop instead of a round trip at a time.
type attemptRow struct {
	testResultID int64
	attempt      parser.Attempt
}

// replaceAttempts rewrites the attempt rows of an entire InsertBatchFull batch:
// one DELETE covering every test result the batch touched, then one pgx.Batch
// carrying all the inserts.
//
// Unlike the other child tables it deletes first, because UNIQUE
// (test_result_id, attempt_index) makes a plain insert fail outright on the
// second ingestion of the same report — the parent INSERT is an upsert, so a
// re-upload lands on the SAME test_result_id. Delete-then-insert makes the
// attempt sequence converge on whatever the latest upload says instead.
//
// The DELETE covers resultIDs, every result in the batch, not just the ones
// contributing rows. That is what lets a result below attemptsMinToPersist
// still clear its old rows: dropping to a single attempt is a statement that
// the report no longer claims a retry sequence, and leaving the previous
// upload's rows behind would attribute them to data that has withdrawn them.
// Those results are simply absent from attempts.
//
// Batch-wide rather than per-result because nearly every test result has fewer
// than two attempts and so contributes no rows at all: issued per result, the
// DELETE was an unconditional network round trip per test — ten thousand of
// them for a large build, all inside the ingestion transaction, holding its
// locks open for the duration.
func replaceAttempts(ctx context.Context, tx pgx.Tx, resultIDs []int64, attempts []attemptRow) error {
	if len(resultIDs) == 0 {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`DELETE FROM test_attempts WHERE test_result_id = ANY($1)`, resultIDs,
	); err != nil {
		return fmt.Errorf("delete existing attempts: %w", err)
	}
	if len(attempts) == 0 {
		return nil
	}

	batch := &pgx.Batch{}
	for _, row := range attempts {
		batch.Queue(
			`INSERT INTO test_attempts(test_result_id, attempt_index, status, status_message)
			 VALUES ($1,$2,$3,$4)`,
			row.testResultID, row.attempt.Index, row.attempt.Status,
			truncateRunes(row.attempt.StatusMessage, attemptMessageMax),
		)
	}
	br := tx.SendBatch(ctx, batch)
	defer func() { _ = br.Close() }()
	// Results are read in queue order so a failure names the attempt that caused
	// it rather than the batch as a whole.
	for _, row := range attempts {
		if _, err := br.Exec(); err != nil {
			return fmt.Errorf("insert attempt %d of test result %d: %w", row.attempt.Index, row.testResultID, err)
		}
	}
	return br.Close()
}

// truncateRunes caps s at max runes. It counts runes rather than bytes so a
// multi-byte character is never split into invalid UTF-8 on the way to a TEXT
// column.
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// GetAttempts returns every recorded execution attempt of the test identified
// by (projectID, buildID, historyID), ordered by attempt_index ascending. The
// result is empty rather than an error when the test has no attempt rows —
// only reports carrying per-attempt detail produce any. An empty historyID
// short-circuits: it matches every test lacking one, so it is never a valid
// lookup key (matching GetByHistoryID).
func (ts *TestResultStore) GetAttempts(ctx context.Context, projectID int64, buildID int64, historyID string) ([]store.TestAttemptRow, error) {
	if historyID == "" {
		return nil, nil
	}
	rows, err := ts.pool.Query(ctx, `
		SELECT ta.attempt_index, ta.status, ta.status_message
		FROM test_attempts ta
		JOIN test_results tr ON tr.id = ta.test_result_id
		WHERE tr.project_id=$1 AND tr.build_id=$2 AND tr.history_id=$3
		ORDER BY ta.attempt_index ASC`, projectID, buildID, historyID)
	if err != nil {
		return nil, fmt.Errorf("get test attempts: %w", err)
	}
	defer rows.Close()

	var out []store.TestAttemptRow
	for rows.Next() {
		var a store.TestAttemptRow
		if err := rows.Scan(&a.AttemptIndex, &a.Status, &a.StatusMessage); err != nil {
			return nil, fmt.Errorf("scan test attempt: %w", err)
		}
		out = append(out, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate test attempts: %w", err)
	}
	return out, nil
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
func (ts *TestResultStore) ListFailedForFingerprinting(ctx context.Context, projectID int64, buildID int64) ([]store.FailedTestResult, error) {
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

// MarkFlakyByHistoryID sets flaky=true on the most-recent test_results row
// matching (project_id, history_id, full_name). When no matching row exists the
// call is a no-op (not an error) — the build may have been pruned.
func (ts *TestResultStore) MarkFlakyByHistoryID(ctx context.Context, projectID int64, historyID, fullName string) error {
	tag, err := ts.pool.Exec(ctx, `
		UPDATE test_results
		SET flaky = true
		WHERE id = (
			SELECT id FROM test_results
			WHERE project_id = $1 AND history_id = $2 AND full_name = $3
			ORDER BY id DESC
			LIMIT 1
		)`,
		projectID, historyID, fullName,
	)
	if err != nil {
		return fmt.Errorf("mark flaky by history_id: %w", err)
	}
	_ = tag // no-op when no row found; caller treats 0 rows as success
	return nil
}

// SearchByName returns up to limit test results whose full_name contains
// substring (case-insensitive ILIKE). Results are ordered by full_name.
func (ts *TestResultStore) SearchByName(ctx context.Context, projectID int64, substring string, limit int) ([]*store.TestResult, error) {
	rows, err := ts.pool.Query(ctx, `
		SELECT DISTINCT ON (history_id)
			build_id, project_id, test_name, full_name, status, history_id,
			duration_ms, flaky, retries, new_failed, new_passed,
			start_ms, stop_ms, thread, host
		FROM test_results
		WHERE project_id = $1 AND full_name ILIKE $2
		ORDER BY history_id, build_id DESC
		LIMIT $3`,
		projectID, "%"+substring+"%", limit,
	)
	if err != nil {
		return nil, fmt.Errorf("search by name: %w", err)
	}
	defer rows.Close()

	var out []*store.TestResult
	for rows.Next() {
		var r store.TestResult
		if err := rows.Scan(
			&r.BuildID, &r.ProjectID, &r.TestName, &r.FullName, &r.Status,
			&r.HistoryID, &r.DurationMs, &r.Flaky, &r.Retries, &r.NewFailed, &r.NewPassed,
			&r.StartMs, &r.StopMs, &r.Thread, &r.Host,
		); err != nil {
			return nil, fmt.Errorf("scan search row: %w", err)
		}
		out = append(out, &r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate search rows: %w", err)
	}
	return out, nil
}

// ListRecentMessages returns up to limit distinct non-empty status_message
// values from failed/broken test_results for a project, ordered by recency.
// Used by the propose_known_issue dry-run match-count estimate.
func (ts *TestResultStore) ListRecentMessages(ctx context.Context, projectID int64, limit int) ([]string, error) {
	rows, err := ts.pool.Query(ctx, `
		SELECT DISTINCT status_message
		FROM test_results
		WHERE project_id = $1
		  AND status IN ('failed', 'broken')
		  AND status_message IS NOT NULL
		  AND status_message != ''
		ORDER BY status_message
		LIMIT $2`,
		projectID, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list recent messages: %w", err)
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var msg string
		if err := rows.Scan(&msg); err != nil {
			return nil, fmt.Errorf("scan message: %w", err)
		}
		out = append(out, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate messages: %w", err)
	}
	return out, nil
}

// GetDefectFingerprintID returns the defect_fingerprint_id linked to the
// test_results row identified by (projectID, buildID, historyID). The returned
// pointer is nil when the row exists but has no linked fingerprint (the FK is
// NULL). Returns store.ErrTestResultNotFound when no matching row exists.
func (ts *TestResultStore) GetDefectFingerprintID(ctx context.Context, projectID int64, buildID int64, historyID string) (*string, error) {
	var fingerprintID *string
	err := ts.pool.QueryRow(ctx, `
		SELECT defect_fingerprint_id::text
		FROM test_results
		WHERE project_id=$1 AND build_id=$2 AND history_id=$3
		LIMIT 1`, projectID, buildID, historyID,
	).Scan(&fingerprintID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: build_id=%d history_id=%s", store.ErrTestResultNotFound, buildID, historyID)
	}
	if err != nil {
		return nil, fmt.Errorf("get defect fingerprint id: %w", err)
	}
	return fingerprintID, nil
}

var _ store.TestResultStorer = (*TestResultStore)(nil)
