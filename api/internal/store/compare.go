package store

import (
	"context"
	"fmt"
)

// DiffCategory classifies how a test's status changed between two builds.
type DiffCategory string

const (
	DiffRegressed DiffCategory = "regressed"
	DiffFixed     DiffCategory = "fixed"
	DiffAdded     DiffCategory = "added"
	DiffRemoved   DiffCategory = "removed"
)

// DiffEntry represents one test's status change between build A and build B.
type DiffEntry struct {
	TestName  string
	FullName  string
	HistoryID string
	StatusA   string // empty when test is added (only in B)
	StatusB   string // empty when test is removed (only in A)
	DurationA int64  // 0 when added
	DurationB int64  // 0 when removed
	Category  DiffCategory
}

// isFailure reports whether a test status counts as a failure.
func isFailure(status string) bool {
	return status == "failed" || status == "broken"
}

// classifyDiff determines the diff category for a pair of statuses.
// Returns ("", false) when the test is unchanged and should be excluded.
func classifyDiff(statusA, statusB string) (DiffCategory, bool) {
	switch {
	case statusA == "":
		return DiffAdded, true
	case statusB == "":
		return DiffRemoved, true
	case statusA == "passed" && isFailure(statusB):
		return DiffRegressed, true
	case isFailure(statusA) && statusB == "passed":
		return DiffFixed, true
	default:
		return "", false
	}
}

// CompareBuildsByHistoryID diffs two builds within a project, matching tests by
// history_id. Only tests whose status changed are returned. Tests without a
// history_id are excluded.
func (ts *TestResultStore) CompareBuildsByHistoryID(
	ctx context.Context,
	projectID string,
	buildIDA, buildIDB int64,
) ([]DiffEntry, error) {
	// SQLite has no FULL OUTER JOIN, so we use UNION ALL:
	// 1. LEFT JOIN from A to B  — covers all tests in A (matched or unmatched)
	// 2. Anti-join from B to A  — covers tests only in B
	const query = `
		SELECT
			COALESCE(a.test_name, b.test_name),
			COALESCE(a.full_name,  b.full_name),
			COALESCE(a.history_id, b.history_id),
			COALESCE(a.status, ''),
			COALESCE(b.status, ''),
			COALESCE(a.duration_ms, 0),
			COALESCE(b.duration_ms, 0)
		FROM test_results a
		LEFT JOIN test_results b
			ON b.build_id = ? AND b.project_id = ? AND b.history_id = a.history_id
		WHERE a.build_id = ? AND a.project_id = ? AND a.history_id != ''

		UNION ALL

		SELECT
			b.test_name, b.full_name, b.history_id,
			'', b.status,
			0,  b.duration_ms
		FROM test_results b
		LEFT JOIN test_results a
			ON a.build_id = ? AND a.project_id = ? AND a.history_id = b.history_id
		WHERE b.build_id = ? AND b.project_id = ? AND b.history_id != '' AND a.id IS NULL

		ORDER BY 1
	`

	rows, err := ts.db.QueryContext(ctx, query,
		// First SELECT: LEFT JOIN condition
		buildIDB, projectID,
		// First SELECT: WHERE clause
		buildIDA, projectID,
		// Second SELECT (anti-join): LEFT JOIN condition
		buildIDA, projectID,
		// Second SELECT: WHERE clause
		buildIDB, projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("compare builds: %w", err)
	}
	defer func() { _ = rows.Close() }()

	result := []DiffEntry{}
	for rows.Next() {
		var e DiffEntry
		if err := rows.Scan(
			&e.TestName, &e.FullName, &e.HistoryID,
			&e.StatusA, &e.StatusB,
			&e.DurationA, &e.DurationB,
		); err != nil {
			return nil, fmt.Errorf("scan diff row: %w", err)
		}
		cat, include := classifyDiff(e.StatusA, e.StatusB)
		if !include {
			continue
		}
		e.Category = cat
		result = append(result, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate diff rows: %w", err)
	}
	return result, nil
}
