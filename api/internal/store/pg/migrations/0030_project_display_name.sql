-- +goose Up
ALTER TABLE projects ADD COLUMN IF NOT EXISTS display_name TEXT NOT NULL DEFAULT '';
UPDATE projects SET display_name = id WHERE display_name = '';

-- +goose Down
ALTER TABLE projects DROP COLUMN IF EXISTS display_name;
