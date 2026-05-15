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

	// Collapse duplicate-encoding entries: the same logical test ingested under
	// two different history_id values (e.g. "." vs ":" separator variants in the
	// MD5 hash) will produce twin "added"/"removed" rows that never join in the
	// FULL OUTER JOIN. Prefer the row where both status_a and status_b are
	// non-empty (a real diff) over a row where one side is empty (an unmatched
	// add/remove). Stable order by first occurrence.
	//
	// NOTE: the proper fix is consistent history_id derivation in the parser —
	// that is a separate change, out of scope here.
	result = collapseByFullName(result)

	return result, nil
}

// diffKey is the deduplication key for collapseByFullName.
type diffKey struct {
	fullName string
	testName string
}

// collapseByFullName deduplicates DiffEntry slices keyed by (full_name,
// test_name). When two entries share the same key, the one with both StatusA
// and StatusB non-empty wins (real diff > unmatched add/remove).
func collapseByFullName(entries []store.DiffEntry) []store.DiffEntry {
	if len(entries) == 0 {
		return entries
	}

	type slot struct {
		idx   int
		entry store.DiffEntry
	}

	seen := make(map[diffKey]slot, len(entries))
	order := make([]diffKey, 0, len(entries))

	for _, e := range entries {
		k := diffKey{fullName: e.FullName, testName: e.TestName}
		existing, ok := seen[k]
		if !ok {
			seen[k] = slot{idx: len(order), entry: e}
			order = append(order, k)
			continue
		}
		// Prefer entries where both sides are populated over one-sided entries.
		bothNew := e.StatusA != "" && e.StatusB != ""
		bothOld := existing.entry.StatusA != "" && existing.entry.StatusB != ""
		if bothNew && !bothOld {
			seen[k] = slot{idx: existing.idx, entry: e}
		}
	}

	out := make([]store.DiffEntry, 0, len(order))
	for _, k := range order {
		out = append(out, seen[k].entry)
	}
	return out
}
