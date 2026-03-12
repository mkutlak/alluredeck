-- +goose Up
CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    config_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS builds (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id    TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    build_order   INTEGER NOT NULL,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    stat_passed   INTEGER,
    stat_failed   INTEGER,
    stat_broken   INTEGER,
    stat_skipped  INTEGER,
    stat_unknown  INTEGER,
    stat_total    INTEGER,
    duration_ms   BIGINT,
    is_latest     BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE(project_id, build_order)
);

CREATE INDEX IF NOT EXISTS idx_builds_project ON builds(project_id, build_order DESC);

CREATE TABLE IF NOT EXISTS jwt_blacklist (
    jti        TEXT PRIMARY KEY,
    expires_at TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_jwt_expires ON jwt_blacklist(expires_at);

-- +goose Down
DROP INDEX IF EXISTS idx_jwt_expires;
DROP TABLE IF EXISTS jwt_blacklist;
DROP INDEX IF EXISTS idx_builds_project;
DROP TABLE IF EXISTS builds;
DROP TABLE IF EXISTS projects;
