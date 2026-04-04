-- +goose Up
ALTER TABLE builds ADD COLUMN IF NOT EXISTS has_playwright_report BOOLEAN NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE builds DROP COLUMN IF EXISTS has_playwright_report;
