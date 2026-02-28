CREATE TABLE IF NOT EXISTS test_results (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    build_id    INTEGER NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
    project_id  TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    test_name   TEXT    NOT NULL,
    full_name   TEXT    NOT NULL DEFAULT '',
    status      TEXT    NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    history_id  TEXT    NOT NULL DEFAULT '',
    flaky       INTEGER NOT NULL DEFAULT 0,
    retries     INTEGER NOT NULL DEFAULT 0,
    new_failed  INTEGER NOT NULL DEFAULT 0,
    new_passed  INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_test_results_project_build ON test_results(project_id, build_id);
CREATE INDEX IF NOT EXISTS idx_test_results_history       ON test_results(project_id, history_id);
CREATE INDEX IF NOT EXISTS idx_test_results_duration      ON test_results(project_id, duration_ms DESC);
