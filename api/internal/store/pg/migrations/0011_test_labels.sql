-- +goose Up
CREATE TABLE IF NOT EXISTS test_labels (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    test_result_id BIGINT NOT NULL REFERENCES test_results(id) ON DELETE CASCADE,
    name           TEXT   NOT NULL,
    value          TEXT   NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_test_labels_result      ON test_labels(test_result_id);
CREATE INDEX IF NOT EXISTS idx_test_labels_name_value  ON test_labels(name, value);
CREATE INDEX IF NOT EXISTS idx_test_labels_project_name ON test_labels(name, value) INCLUDE (test_result_id);

-- +goose Down
DROP INDEX IF EXISTS idx_test_labels_project_name;
DROP INDEX IF EXISTS idx_test_labels_name_value;
DROP INDEX IF EXISTS idx_test_labels_result;
DROP TABLE IF EXISTS test_labels;
