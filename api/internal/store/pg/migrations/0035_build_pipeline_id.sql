-- +goose Up
ALTER TABLE builds ADD COLUMN IF NOT EXISTS ci_pipeline_id TEXT;
ALTER TABLE builds ADD COLUMN IF NOT EXISTS ci_pipeline_url TEXT;
CREATE INDEX idx_builds_pipeline_id ON builds(ci_pipeline_id) WHERE ci_pipeline_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_builds_pipeline_id;
ALTER TABLE builds DROP COLUMN IF EXISTS ci_pipeline_url;
ALTER TABLE builds DROP COLUMN IF EXISTS ci_pipeline_id;
