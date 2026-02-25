PRAGMA foreign_keys=ON;

CREATE TABLE IF NOT EXISTS projects (
    id          TEXT PRIMARY KEY,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    config_json TEXT NOT NULL DEFAULT '{}'
);

CREATE TABLE IF NOT EXISTS builds (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id   TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    build_order  INTEGER NOT NULL,
    created_at   TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    stat_passed  INTEGER,
    stat_failed  INTEGER,
    stat_broken  INTEGER,
    stat_skipped INTEGER,
    stat_unknown INTEGER,
    stat_total   INTEGER,
    duration_ms  INTEGER,
    is_latest    INTEGER NOT NULL DEFAULT 0,
    UNIQUE(project_id, build_order)
);

CREATE INDEX IF NOT EXISTS idx_builds_project ON builds(project_id, build_order DESC);

CREATE TABLE IF NOT EXISTS jwt_blacklist (
    jti        TEXT PRIMARY KEY,
    expires_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_jwt_expires ON jwt_blacklist(expires_at);
