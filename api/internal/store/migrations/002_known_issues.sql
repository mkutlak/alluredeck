CREATE TABLE IF NOT EXISTS known_issues (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    test_name   TEXT    NOT NULL,
    pattern     TEXT    NOT NULL DEFAULT '',
    ticket_url  TEXT    NOT NULL DEFAULT '',
    description TEXT    NOT NULL DEFAULT '',
    is_active   INTEGER NOT NULL DEFAULT 1,
    created_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    updated_at  TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    UNIQUE(project_id, test_name)
);
CREATE INDEX IF NOT EXISTS idx_known_issues_project ON known_issues(project_id, is_active);
