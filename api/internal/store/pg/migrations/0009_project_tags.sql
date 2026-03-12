-- +goose Up
ALTER TABLE projects ADD COLUMN IF NOT EXISTS tags JSONB NOT NULL DEFAULT '[]'::jsonb;

CREATE INDEX IF NOT EXISTS idx_projects_tags ON projects USING GIN (tags);

-- +goose Down
DROP INDEX IF EXISTS idx_projects_tags;
ALTER TABLE projects DROP COLUMN IF EXISTS tags;
