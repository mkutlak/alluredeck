-- +goose Up
-- test_attempts records one row per execution attempt of a test within a build.
--
-- test_results.retries already stores how MANY times a test was re-run, but a
-- count cannot answer the question a triage tool actually asks: did every
-- attempt fail the same way (a deterministic break) or differently (a flaky or
-- environmental one)? triage.computeRetryConsistency has always been able to
-- classify that, but nothing persisted the per-attempt outcomes it compares, so
-- it reported "single" for every test regardless of how often it was retried.
--
-- status is stored verbatim from the source report rather than normalized to
-- the Allure vocabulary: a Playwright attempt reports "timedOut" or
-- "interrupted", and folding those into "broken" would erase exactly the
-- distinction this table exists to record.
--
-- status_message is NOT NULL DEFAULT '' — a passing retry legitimately has no
-- error text, and an empty string says that without forcing every reader
-- through a NULL check. Writers cap it at 2000 characters; the full text of the
-- final attempt already lives in test_results.status_message/status_trace, so
-- this column only needs to hold enough for attempts to be compared.
--
-- The FK cascades from test_results so retention pruning a build takes its
-- attempts with it. UNIQUE (test_result_id, attempt_index) makes ingestion
-- idempotent: a re-upload deletes and reinserts a test's attempts, and the
-- constraint turns any double-write bug into an immediate error rather than a
-- silently duplicated attempt sequence.
--
-- That unique constraint is also the ONLY index this table needs. Its btree is
-- keyed (test_result_id, attempt_index), so a leading-column lookup by
-- test_result_id alone — the FK cascade probe and every read in
-- store/pg/test_result.go — is served by it. A separate index on
-- (test_result_id) would only add write cost and bloat (migration 0017 dropped
-- a redundant index for the same reason).
CREATE TABLE IF NOT EXISTS test_attempts (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    test_result_id BIGINT  NOT NULL REFERENCES test_results(id) ON DELETE CASCADE,
    attempt_index  INTEGER NOT NULL,
    status         TEXT    NOT NULL,
    status_message TEXT    NOT NULL DEFAULT '',
    UNIQUE (test_result_id, attempt_index)
);

-- +goose Down
DROP TABLE IF EXISTS test_attempts;
