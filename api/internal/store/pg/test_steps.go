package pg

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// stepRow is a single row loaded from the test_steps table for path
// reconstruction. It carries only the columns needed to walk the step tree.
type stepRow struct {
	id            int64
	parentID      *int64
	name          string
	status        string
	statusMessage string
	stepOrder     int
}

// failedStepStatuses are the step statuses treated as "failed" when walking the
// step tree to find the failure path.
var failedStepStatuses = map[string]bool{
	"failed": true,
	"broken": true,
}

// GetFailedStepPath reconstructs the failed-step trail for the test_results row
// identified by (projectID, buildID, historyID).
//
// Algorithm: load every step for the test in one query, index them by
// parent_step_id, then descend from the root: at each level pick the
// lowest-step_order failed/broken step and recurse into it. The walk stops when
// no failed child remains; the accumulated names form the path and the deepest
// failed step's status_message is the most specific error text.
//
// Returns empty results (not an error) when the test has no recorded steps or
// no failed step. Returns store.ErrTestResultNotFound when no matching
// test_results row exists.
func (ts *TestResultStore) GetFailedStepPath(ctx context.Context, projectID int64, buildID int64, historyID string) ([]string, string, error) {
	// Resolve the test_results primary key first. This doubles as an existence
	// check so callers get ErrTestResultNotFound rather than a silent empty path
	// when they pass a bad (build_id, history_id) pair.
	var testResultID int64
	err := ts.pool.QueryRow(ctx, `
		SELECT id
		FROM test_results
		WHERE project_id=$1 AND build_id=$2 AND history_id=$3
		LIMIT 1`, projectID, buildID, historyID,
	).Scan(&testResultID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", fmt.Errorf("%w: build_id=%d history_id=%s", store.ErrTestResultNotFound, buildID, historyID)
	}
	if err != nil {
		return nil, "", fmt.Errorf("resolve test result id: %w", err)
	}

	rows, err := ts.pool.Query(ctx, `
		SELECT id, parent_step_id, name, status, COALESCE(status_message, ''), step_order
		FROM test_steps
		WHERE test_result_id=$1
		ORDER BY step_order`, testResultID)
	if err != nil {
		return nil, "", fmt.Errorf("list test steps: %w", err)
	}
	defer rows.Close()

	var steps []stepRow
	for rows.Next() {
		var s stepRow
		if err := rows.Scan(&s.id, &s.parentID, &s.name, &s.status, &s.statusMessage, &s.stepOrder); err != nil {
			return nil, "", fmt.Errorf("scan test step: %w", err)
		}
		steps = append(steps, s)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate test steps: %w", err)
	}

	path, msg := failedStepPath(steps)
	return path, msg, nil
}

// failedStepPath walks the step tree described by steps and returns the ordered
// names from the root failed step down to the deepest failed step, plus the
// status_message of that deepest failed step. It is pure logic (no DB access)
// so it is unit-testable in isolation.
//
// steps may arrive in any order; this function indexes them by parent and sorts
// each sibling group by step_order so the descent is deterministic regardless
// of input order.
func failedStepPath(steps []stepRow) ([]string, string) {
	if len(steps) == 0 {
		return []string{}, ""
	}

	// Group children by parent step id; nil parent → root group (key 0 is safe
	// because test_steps.id is GENERATED ALWAYS AS IDENTITY and starts at 1).
	const rootKey int64 = 0
	children := make(map[int64][]stepRow, len(steps))
	for _, s := range steps {
		key := rootKey
		if s.parentID != nil {
			key = *s.parentID
		}
		children[key] = append(children[key], s)
	}
	// Sort each sibling group by step_order for a deterministic descent.
	for key := range children {
		group := children[key]
		sort.SliceStable(group, func(i, j int) bool {
			return group[i].stepOrder < group[j].stepOrder
		})
	}

	path := make([]string, 0, 4)
	var deepestMessage string
	parent := rootKey
	for {
		failed := firstFailedChild(children[parent])
		if failed == nil {
			break
		}
		path = append(path, failed.name)
		deepestMessage = failed.statusMessage
		parent = failed.id
	}
	if len(path) == 0 {
		return []string{}, ""
	}
	return path, deepestMessage
}

// firstFailedChild returns the lowest-step_order failed/broken step among
// siblings, or nil when none failed. Siblings arrive ordered by step_order from
// the query, so a linear scan suffices.
func firstFailedChild(siblings []stepRow) *stepRow {
	for i := range siblings {
		if failedStepStatuses[siblings[i].status] {
			return &siblings[i]
		}
	}
	return nil
}
