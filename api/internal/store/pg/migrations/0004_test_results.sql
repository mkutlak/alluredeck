-- +goose Up
CREATE TABLE IF NOT EXISTS test_results (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    build_id    BIGINT NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
    project_id  TEXT   NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    test_name   TEXT   NOT NULL DEFAULT '',
    full_name   TEXT   NOT NULL DEFAULT '',
    status      TEXT   NOT NULL DEFAULT '',
    duration_ms BIGINT NOT NULL DEFAULT 0,
    history_id  TEXT   NOT NULL DEFAULT '',
    flaky       BOOLEAN NOT NULL DEFAULT FALSE,
    retries     INTEGER NOT NULL DEFAULT 0,
    new_failed  BOOLEAN NOT NULL DEFAULT FALSE,
    new_passed  BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS idx_test_results_build   ON test_results(build_id);
CREATE INDEX IF NOT EXISTS idx_test_results_project ON test_results(project_id);
CREATE INDEX IF NOT EXISTS idx_test_results_history ON test_results(project_id, history_id);

-- +goose Down
DROP INDEX IF EXISTS idx_test_results_history;
DROP INDEX IF EXISTS idx_test_results_project;
DROP INDEX IF EXISTS idx_test_results_build;
DROP TABLE IF EXISTS test_results;
