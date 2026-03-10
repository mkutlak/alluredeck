-- +goose Up
CREATE INDEX IF NOT EXISTS idx_test_results_analytics ON test_results(project_id, history_id, status, duration_ms);

-- +goose Down
DROP INDEX IF EXISTS idx_test_results_analytics;
