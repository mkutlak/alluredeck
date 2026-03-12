-- +goose Up
CREATE TABLE IF NOT EXISTS branches (
    id         BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    project_id TEXT    NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, name)
);

CREATE INDEX IF NOT EXISTS idx_branches_project ON branches(project_id);

ALTER TABLE builds ADD COLUMN IF NOT EXISTS branch_id BIGINT REFERENCES branches(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_builds_branch ON builds(branch_id);

ALTER TABLE test_results ADD COLUMN IF NOT EXISTS branch_id BIGINT REFERENCES branches(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_test_results_branch ON test_results(branch_id, history_id);

-- +goose Down
DROP INDEX IF EXISTS idx_test_results_branch;
ALTER TABLE test_results DROP COLUMN IF EXISTS branch_id;
DROP INDEX IF EXISTS idx_builds_branch;
ALTER TABLE builds DROP COLUMN IF EXISTS branch_id;
DROP INDEX IF EXISTS idx_branches_project;
DROP TABLE IF EXISTS branches;
