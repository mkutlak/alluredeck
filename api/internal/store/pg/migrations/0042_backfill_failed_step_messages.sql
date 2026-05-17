-- +goose Up
-- Backfill failed/broken test_results rows whose status_message is empty.
--
-- Allure 3 ("awesome") reports leave the test-level statusDetails empty; the
-- real error message lives on the deepest failed step. The ingestion parser was
-- fixed to derive this at parse time, but rows ingested before that fix have a
-- blank status_message — which breaks failure display, defect fingerprinting,
-- known-issue matching, and full-text search.
--
-- For each affected test_results row we locate the failed (fallback broken)
-- test_steps row that the runtime walk would pick and copy its status_message
-- across. test_steps has no status_trace column, so status_trace is left
-- untouched. When the chosen step's message is empty we fall back to the step
-- name, matching the parser's runtime behaviour.
--
-- Tie-break: this MUST match failedStepPath() in
-- api/internal/store/pg/test_steps.go so the backfilled status_message is
-- byte-identical to what the runtime/parser produces (the defect fingerprint
-- backfill then hashes the same message). The runtime walk is NOT "globally
-- deepest failed step". Instead it descends from the root and, at each level,
-- picks the lowest-step_order child whose status is 'failed' OR 'broken'
-- (the two statuses are treated equally — there is no failed-before-broken
-- ranking), then recurses into that child. The descent stops when the chosen
-- step has no failed/broken child. The message comes from the LAST step on
-- that single chosen branch.
--
-- The recursive CTE below replicates that walk exactly: rather than expanding
-- the whole step tree, it follows ONLY the single chosen failed child at each
-- level. step_chain starts at each root failed/broken step (lowest step_order
-- wins per test_result) and, at every level, joins to the lowest-step_order
-- failed/broken child of the current node. DISTINCT ON keeps the deepest row
-- of each test_result's chain (max depth), which is the runtime's deepest
-- failed step. A CYCLE clause guards against corrupt parent_step_id data that
-- forms a loop, so a cycle terminates safely instead of recursing unbounded.
--
-- test_results.search_vector is a GENERATED ALWAYS column (migration 0018) and
-- is recomputed automatically by PostgreSQL on UPDATE.
--
-- Indexes idx_test_steps_result (test_result_id) and idx_test_steps_parent
-- (parent_step_id) already exist (migration 0013) and support both the root
-- selection and the per-level child lookup; no new index is required.

-- +goose StatementBegin
WITH chosen_step AS (
    WITH RECURSIVE
    -- Root failed/broken step per test_result: lowest step_order among the
    -- top-level (parent_step_id IS NULL) failed/broken steps. This is the first
    -- step the runtime descent selects.
    roots AS (
        SELECT DISTINCT ON (s.test_result_id)
               s.id, s.test_result_id, s.name, s.status,
               s.status_message, s.step_order, 0 AS depth
        FROM test_steps s
        WHERE s.parent_step_id IS NULL
          AND s.status IN ('failed', 'broken')
        ORDER BY s.test_result_id, s.step_order, s.id
    ),
    -- Follow the single chosen failed/broken child at each level. For the
    -- current node, the chosen child is the lowest-step_order failed/broken
    -- direct child (LATERAL ... LIMIT 1). The CYCLE clause aborts a branch if
    -- the same step id is revisited (corrupt parent_step_id loop).
    step_chain AS (
        SELECT r.id, r.test_result_id, r.name, r.status,
               r.status_message, r.step_order, r.depth
        FROM roots r
        UNION ALL
        SELECT child.id, sc.test_result_id, child.name, child.status,
               child.status_message, child.step_order, sc.depth + 1
        FROM step_chain sc
        CROSS JOIN LATERAL (
            SELECT c.id, c.name, c.status, c.status_message, c.step_order
            FROM test_steps c
            WHERE c.parent_step_id = sc.id
              AND c.status IN ('failed', 'broken')
            ORDER BY c.step_order, c.id
            LIMIT 1
        ) AS child
    ) CYCLE id SET is_cycle USING cycle_path
    -- Keep the deepest row of each test_result's chain: that is the last step
    -- the runtime walk visits, whose status_message is the most specific text.
    SELECT DISTINCT ON (test_result_id)
        test_result_id,
        COALESCE(NULLIF(status_message, ''), name) AS derived_message
    FROM step_chain
    ORDER BY test_result_id, depth DESC, step_order ASC, id ASC
)
UPDATE test_results tr
SET status_message = cs.derived_message
FROM chosen_step cs
WHERE tr.id = cs.test_result_id
  AND tr.status IN ('failed', 'broken')
  AND (tr.status_message IS NULL OR tr.status_message = '')
  AND cs.derived_message IS NOT NULL
  AND cs.derived_message <> '';
-- +goose StatementEnd

-- +goose Down
-- Backfill is a data-only correction; there is no meaningful rollback because
-- the original (empty) status_message values carried no information. This Down
-- step is intentionally a no-op.
SELECT 1;
