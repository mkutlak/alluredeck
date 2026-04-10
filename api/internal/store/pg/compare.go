package pg

import (
	"context"
	"fmt"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// pgIsFailure reports whether a test status counts as a failure.
func pgIsFailure(status string) bool {
	return status == "failed" || status == "broken"
}

// pgClassifyDiff determines the diff category for a pair of statuses.
// Returns ("", false) when the test is unchanged and should be excluded.
func pgClassifyDiff(statusA, statusB string) (store.DiffCategory, bool) {
	switch {
	case statusA == "":
		return store.DiffAdded, true
	case statusB == "":
		return store.DiffRemoved, true
	case statusA == "passed" && pgIsFailure(statusB):
		return store.DiffRegressed, true
	case pgIsFailure(statusA) && statusB == "passed":
		return store.DiffFixed, true
	default:
		return "", false
	}
}

// CompareBuildsByHistoryID diffs two builds within a project, matching tests by
// history_id. Only tests whose status changed are returned. Tests without a
// history_id are excluded.
// Uses PostgreSQL's native FULL OUTER JOIN.
func (ts *TestResultStore) CompareBuildsByHistoryID(
	ctx context.Context,
	projectID int64,
	buildIDA, buildIDB int64,
) ([]store.DiffEntry, error) {
	const query = `
		SELECT
			COALESCE(a.test_name,  b.test_name),
			COALESCE(a.full_name,  b.full_name),
			COALESCE(a.history_id, b.history_id),
			COALESCE(a.status,     ''),
			COALESCE(b.status,     ''),
			COALESCE(a.duration_ms, 0),
			COALESCE(b.duration_ms, 0)
		FROM (SELECT * FROM test_results WHERE build_id=$1 AND project_id=$2 AND history_id != '') a
		FULL OUTER JOIN (SELECT * FROM test_results WHERE build_id=$3 AND project_id=$4 AND history_id != '') b
			ON a.history_id = b.history_id
		ORDER BY COALESCE(a.test_name, b.test_name)
	`

	rows, err := ts.pool.Query(ctx, query, buildIDA, projectID, buildIDB, projectID)
	if err != nil {
		return nil, fmt.Errorf("compare builds: %w", err)
	}
	defer rows.Close()

	result := []store.DiffEntry{}
	for rows.Next() {
		var e store.DiffEntry
		if err := rows.Scan(
			&e.TestName, &e.FullName, &e.HistoryID,
			&e.StatusA, &e.StatusB,
			&e.DurationA, &e.DurationB,
		); err != nil {
			return nil, fmt.Errorf("scan diff row: %w", err)
		}
		cat, include := pgClassifyDiff(e.StatusA, e.StatusB)
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
