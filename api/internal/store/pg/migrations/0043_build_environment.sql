-- +goose Up
ALTER TABLE builds ADD COLUMN IF NOT EXISTS environment JSONB;

-- +goose Down
ALTER TABLE builds DROP COLUMN IF EXISTS environment;
