-- +goose Up
-- failure_summaries caches the opt-in, LLM-generated failure hypothesis for a
-- single test in a single build, keyed on (build_id, history_id). The FK to
-- builds(id) with ON DELETE CASCADE keeps the cache clean when retention prunes
-- a build. Category is a display-only label and is never written back to a
-- defect record.
CREATE TABLE IF NOT EXISTS failure_summaries (
    build_id       BIGINT NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
    history_id     TEXT   NOT NULL,
    project_id     BIGINT NOT NULL,
    input_hash     TEXT   NOT NULL,
    hypothesis     TEXT   NOT NULL,
    category       TEXT   NOT NULL,
    confidence     TEXT   NOT NULL DEFAULT '',
    evidence       JSONB  NOT NULL DEFAULT '[]',
    model          TEXT   NOT NULL,
    prompt_version INT    NOT NULL DEFAULT 1,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (build_id, history_id)
);

-- +goose Down
DROP TABLE IF EXISTS failure_summaries;
