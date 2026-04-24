-- +goose Up
CREATE INDEX IF NOT EXISTS idx_builds_project_branch ON builds(project_id, branch_id, build_order DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_builds_project_branch;
