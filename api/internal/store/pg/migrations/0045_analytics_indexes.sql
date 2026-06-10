-- +goose Up
-- Support analytics GROUP BY full_name queries on test_results.
CREATE INDEX IF NOT EXISTS idx_test_results_project_fullname ON test_results(project_id, full_name);
-- Support test_labels joins keyed by test_result_id with an equality filter on name.
CREATE INDEX IF NOT EXISTS idx_test_labels_result_name ON test_labels(test_result_id, name);

-- +goose Down
DROP INDEX IF EXISTS idx_test_results_project_fullname;
DROP INDEX IF EXISTS idx_test_labels_result_name;
