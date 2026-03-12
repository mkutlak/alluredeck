-- +goose Up
CREATE TABLE IF NOT EXISTS test_steps (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    test_result_id BIGINT  NOT NULL REFERENCES test_results(id) ON DELETE CASCADE,
    parent_step_id BIGINT  REFERENCES test_steps(id) ON DELETE CASCADE,
    name           TEXT    NOT NULL,
    status         TEXT    NOT NULL,
    status_message TEXT,
    duration_ms    BIGINT  NOT NULL DEFAULT 0,
    step_order     INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_test_steps_result ON test_steps(test_result_id);
CREATE INDEX IF NOT EXISTS idx_test_steps_parent ON test_steps(parent_step_id);

-- +goose Down
DROP INDEX IF EXISTS idx_test_steps_parent;
DROP INDEX IF EXISTS idx_test_steps_result;
DROP TABLE IF EXISTS test_steps;
