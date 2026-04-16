-- +goose Up
ALTER TABLE projects ADD COLUMN storage_key TEXT;

-- Top-level projects: storage_key = slug (preserves existing directory names)
UPDATE projects SET storage_key = slug WHERE parent_id IS NULL;

-- Child projects: storage_key = numeric ID (globally unique, no collision)
UPDATE projects SET storage_key = id::TEXT WHERE parent_id IS NOT NULL;

ALTER TABLE projects ALTER COLUMN storage_key SET NOT NULL;

-- Global uniqueness — slugs are unique among top-level, numeric IDs are
-- unique among children, and slug validation rejects all-numeric names.
CREATE UNIQUE INDEX idx_projects_storage_key ON projects(storage_key);

-- +goose Down
DROP INDEX IF EXISTS idx_projects_storage_key;
ALTER TABLE projects DROP COLUMN IF EXISTS storage_key;
