-- +goose Up
CREATE UNIQUE INDEX IF NOT EXISTS idx_test_results_build_history
ON test_results(build_id, history_id) WHERE history_id != '';

-- +goose Down
DROP INDEX IF EXISTS idx_test_results_build_history;
