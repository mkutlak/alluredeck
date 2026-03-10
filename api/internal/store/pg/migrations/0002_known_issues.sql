-- +goose Up
CREATE TABLE IF NOT EXISTS known_issues (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id  TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    test_name   TEXT    NOT NULL DEFAULT '',
    pattern     TEXT    NOT NULL DEFAULT '',
    ticket_url  TEXT    NOT NULL DEFAULT '',
    description TEXT    NOT NULL DEFAULT '',
    is_active   BOOLEAN NOT NULL DEFAULT TRUE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_known_issues_project ON known_issues(project_id);

-- +goose Down
DROP INDEX IF EXISTS idx_known_issues_project;
DROP TABLE IF EXISTS known_issues;
