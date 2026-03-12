-- +goose Up
CREATE TABLE IF NOT EXISTS test_attachments (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    test_result_id BIGINT REFERENCES test_results(id) ON DELETE CASCADE,
    test_step_id   BIGINT REFERENCES test_steps(id) ON DELETE CASCADE,
    name           TEXT   NOT NULL,
    source         TEXT   NOT NULL,
    mime_type      TEXT   NOT NULL DEFAULT '',
    size_bytes     BIGINT NOT NULL DEFAULT 0
);

CREATE INDEX IF NOT EXISTS idx_test_attachments_result ON test_attachments(test_result_id);
CREATE INDEX IF NOT EXISTS idx_test_attachments_step   ON test_attachments(test_step_id);

-- +goose Down
DROP INDEX IF EXISTS idx_test_attachments_step;
DROP INDEX IF EXISTS idx_test_attachments_result;
DROP TABLE IF EXISTS test_attachments;
