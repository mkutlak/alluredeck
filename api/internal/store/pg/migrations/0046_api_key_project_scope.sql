-- +goose Up
ALTER TABLE api_keys ADD COLUMN project_ids BIGINT[] NOT NULL DEFAULT '{}';

-- +goose Down
ALTER TABLE api_keys DROP COLUMN IF EXISTS project_ids;
