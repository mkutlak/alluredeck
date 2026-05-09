-- +goose Up
-- River exposes JobUpdate but only allows updating the job output, not arbitrary
-- metadata, so per-phase progress is persisted in a sibling table keyed by the
-- River job ID. Last-write-wins semantics: a single row per job is upserted as
-- the worker advances. Rows live for the lifetime of the job and are cleaned up
-- when the job is deleted from River.
CREATE TABLE IF NOT EXISTS job_progress (
    job_id        BIGINT      PRIMARY KEY,
    phase         TEXT        NOT NULL,
    progress_done INTEGER     NOT NULL DEFAULT 0,
    progress_total INTEGER    NOT NULL DEFAULT 0,
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE IF EXISTS job_progress;
