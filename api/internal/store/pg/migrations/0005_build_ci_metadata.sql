-- +goose Up
ALTER TABLE builds ADD COLUMN IF NOT EXISTS ci_provider   TEXT;
ALTER TABLE builds ADD COLUMN IF NOT EXISTS ci_build_url  TEXT;
ALTER TABLE builds ADD COLUMN IF NOT EXISTS ci_branch     TEXT;
ALTER TABLE builds ADD COLUMN IF NOT EXISTS ci_commit_sha TEXT;

-- +goose Down
ALTER TABLE builds DROP COLUMN IF EXISTS ci_commit_sha;
ALTER TABLE builds DROP COLUMN IF EXISTS ci_branch;
ALTER TABLE builds DROP COLUMN IF EXISTS ci_build_url;
ALTER TABLE builds DROP COLUMN IF EXISTS ci_provider;
