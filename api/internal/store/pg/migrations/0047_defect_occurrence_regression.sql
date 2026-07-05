-- +goose Up
ALTER TABLE defect_occurrences ADD COLUMN is_regression BOOLEAN NOT NULL DEFAULT FALSE;
CREATE INDEX IF NOT EXISTS idx_defect_occurrences_regression ON defect_occurrences (build_id) WHERE is_regression;

-- +goose Down
DROP INDEX IF EXISTS idx_defect_occurrences_regression;
ALTER TABLE defect_occurrences DROP COLUMN IF EXISTS is_regression;
