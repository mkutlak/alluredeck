-- +goose Up
-- Delete the bare "shell" test_results twins left behind by double ingestion.
--
-- Both writes for an Allure upload upsert on (build_id, history_id):
-- InsertBatch writes stability rows read from the GENERATED report
-- (reports/latest/data/test-results), InsertBatchFull writes enriched rows
-- parsed from the RAW uploaded *-result.json files. The Allure generator does
-- not always preserve the reporter's historyId, so the two writes landed on two
-- different keys and every test was stored twice in the same build: a shell row
-- (status/duration/retries only, no status_message and no child rows) plus an
-- enriched row (status_message and/or steps, labels, parameters, attachments).
--
-- The ingestion fix (historyIDsByFullName in api/internal/runner/allure.go)
-- makes the stability rows adopt the raw result's historyId, so both writes
-- collapse onto one row going forward. This migration removes the twins that
-- are already in the table.
--
-- Predicate. Identification follows the repo rule: group on full_name, never on
-- history_id. A row r1 is deleted when ALL of the following hold:
--
--   0. r1.full_name <> ''. full_name is NOT NULL DEFAULT '' and fullName is
--      OPTIONAL in an Allure raw result, so an empty value is a MISSING
--      identity rather than a shared one. Without this guard every
--      empty-full_name row in a build falls into a single '' group and any
--      message-less, childless row there is deleted because some unrelated row
--      in the same build happens to carry a message — up to and including
--      wiping the group out entirely. Empty-full_name rows are therefore
--      neither grouped nor deleted, and never count as anyone's sibling: the
--      whole statement filters them out before grouping (WHERE r.full_name <>
--      '' in the scored CTE below), which applies the guard to r1 and r2 alike.
--   1. r1 is a shell: COALESCE(status_message,'') = '' and it has ZERO rows in
--      every child table (test_attachments, test_steps, test_parameters,
--      test_labels). Below, that is spelled "NOT rich".
--   2. A sibling r2 exists in the same (project_id, build_id, full_name) group
--      with a different history_id.
--   3. That sibling is strictly richer: it has a non-empty status_message OR at
--      least one child row.
--
-- Condition 3 is what extends the cleanup to green builds, where neither twin
-- carries a status_message but only the enriched twin has labels/parameters.
-- Condition 1 keeps the relation antisymmetric: a row can only be deleted if it
-- holds nothing at all, so two twins can never delete each other. Combined with
-- condition 3 every group keeps at least one row — the rich sibling that
-- authorized the delete is itself never deletable — so no test can lose its
-- last surviving copy. Two childless twins with empty messages are ambiguous
-- and both are left alone; likewise a childless row that does carry a message
-- is never deleted, even next to a richer sibling.
--
-- Parameterized tests share a full_name and differ only by parameter hash, but
-- each variant is written by InsertBatchFull with its own test_parameters rows,
-- so condition 1 excludes them and they are never collapsed.
--
-- Because condition 1 requires zero child rows, there is nothing to delete from
-- the child tables first. They are declared REFERENCES test_results(id) ON
-- DELETE CASCADE anyway (migrations 0011-0014, 0050), so any row that somehow
-- slipped through would still be removed with its parent rather than orphaned.
--
-- test_results.defect_fingerprint_id is a plain column, not an inbound FK, so
-- deleting shell rows breaks no reference. Occurrence counts cached in
-- defect_occurrences may be left slightly high for historical builds; they are
-- recomputed on the next ingestion of the affected fingerprint.
--
-- Shape. This runs on startup, before the HTTP listener binds, so it is written
-- as ONE set-based pass rather than a correlated per-row probe. The earlier
-- correlated form re-derived each candidate's sibling group from
-- idx_test_results_project_fullname (project_id, full_name) — an index that
-- does NOT include build_id, so every candidate row bitmap-ANDed a whole
-- project's copies of a full_name against a whole build. On a 400k-row fixture
-- (200 builds x 1000 twinned tests) that measured 40.7s; the form below
-- measures 5.3s on the same data, of which 4.1s is the unavoidable ON DELETE
-- CASCADE trigger work for the 200k deleted rows. That extrapolates to well
-- under a minute at 1M rows.
--
-- The rewrite computes richness once per row (one hash join against the union
-- of child-table owners) and derives the sibling condition with window
-- aggregates over a single sort of (project_id, build_id, full_name,
-- history_id) instead of re-querying test_results per candidate:
--
--   rich_in_group     — rich rows in r1's (project_id, build_id, full_name) group
--   rich_same_history — rich rows in that group sharing r1's history_id
--
-- so "a RICHER sibling with a DIFFERENT history_id exists" is exactly
-- rich_in_group > rich_same_history. rich_same_history is only ever non-zero
-- for history_id = '': idx_test_results_build_history is UNIQUE on (build_id,
-- history_id) WHERE history_id <> '', so no two rows of one build can share a
-- non-empty id. It is what keeps a shell from being deleted by a rich sibling
-- that has the SAME (empty) history_id, matching condition 2.
--
-- Idempotent: once the shell rows are gone every surviving row in a group is
-- rich, "NOT rich" selects nothing, and a re-run deletes zero rows.

-- +goose StatementBegin
WITH child_owners AS (
    SELECT test_result_id AS id FROM test_labels
    UNION
    SELECT test_result_id FROM test_parameters
    UNION
    SELECT test_result_id FROM test_steps
    UNION
    SELECT test_result_id FROM test_attachments
),
scored AS (
    SELECT r.id, r.project_id, r.build_id, r.full_name, r.history_id,
           (COALESCE(r.status_message, '') <> '' OR c.id IS NOT NULL) AS rich
    FROM test_results r
    LEFT JOIN child_owners c ON c.id = r.id
    WHERE r.full_name <> ''
),
ranked AS (
    SELECT s.id, s.rich,
           count(*) FILTER (WHERE s.rich)
               OVER (PARTITION BY s.project_id, s.build_id, s.full_name)                AS rich_in_group,
           count(*) FILTER (WHERE s.rich)
               OVER (PARTITION BY s.project_id, s.build_id, s.full_name, s.history_id)  AS rich_same_history
    FROM scored s
)
DELETE FROM test_results t
USING ranked k
WHERE t.id = k.id
  AND NOT k.rich
  AND k.rich_in_group > k.rich_same_history;
-- +goose StatementEnd

-- +goose Down
-- Deleting duplicated ingestion artifacts is a one-way data correction: the
-- removed rows carried no information their surviving richer twin does not
-- already hold, and their identity (id, history_id) cannot be reconstructed.
-- This Down step is intentionally a no-op.
SELECT 1;
