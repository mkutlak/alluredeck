-- branches table
CREATE TABLE branches (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    is_default INTEGER NOT NULL DEFAULT 0,
    created_at TEXT    NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ','now')),
    UNIQUE(project_id, name)
);
CREATE INDEX idx_branches_project ON branches(project_id);

-- Add branch_id to builds (nullable for backward compat with existing data)
ALTER TABLE builds ADD COLUMN branch_id INTEGER REFERENCES branches(id) ON DELETE SET NULL;
CREATE INDEX idx_builds_branch ON builds(branch_id);

-- Add branch_id to test_results (nullable for backward compat)
ALTER TABLE test_results ADD COLUMN branch_id INTEGER REFERENCES branches(id) ON DELETE SET NULL;
CREATE INDEX idx_test_results_branch ON test_results(branch_id, history_id);
