-- +goose Up
ALTER TABLE projects ADD COLUMN IF NOT EXISTS report_type TEXT NOT NULL DEFAULT 'allure';

-- +goose Down
ALTER TABLE projects DROP COLUMN IF EXISTS report_type;
