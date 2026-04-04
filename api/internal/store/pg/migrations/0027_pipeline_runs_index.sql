-- +goose Up
CREATE INDEX IF NOT EXISTS idx_builds_commit_sha ON builds(ci_commit_sha) WHERE ci_commit_sha IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_builds_commit_sha;
