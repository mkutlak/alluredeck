-- +goose Up
CREATE TABLE IF NOT EXISTS test_parameters (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    test_result_id BIGINT NOT NULL REFERENCES test_results(id) ON DELETE CASCADE,
    name           TEXT   NOT NULL,
    value          TEXT   NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_test_params_result ON test_parameters(test_result_id);

-- +goose Down
DROP INDEX IF EXISTS idx_test_params_result;
DROP TABLE IF EXISTS test_parameters;
