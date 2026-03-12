-- +goose Up
ALTER TABLE test_results ADD COLUMN IF NOT EXISTS status_message TEXT;
ALTER TABLE test_results ADD COLUMN IF NOT EXISTS status_trace   TEXT;
ALTER TABLE test_results ADD COLUMN IF NOT EXISTS description    TEXT;
ALTER TABLE test_results ADD COLUMN IF NOT EXISTS search_vector  tsvector;

CREATE INDEX IF NOT EXISTS idx_test_results_fts ON test_results USING GIN (search_vector);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION test_results_search_trigger() RETURNS trigger AS $$
BEGIN
  NEW.search_vector := to_tsvector('english',
    COALESCE(NEW.test_name, '') || ' ' ||
    COALESCE(NEW.full_name, '') || ' ' ||
    COALESCE(NEW.status_message, ''));
  RETURN NEW;
END $$ LANGUAGE plpgsql;
-- +goose StatementEnd

CREATE TRIGGER trg_test_results_search
  BEFORE INSERT OR UPDATE ON test_results
  FOR EACH ROW EXECUTE FUNCTION test_results_search_trigger();

-- +goose Down
DROP TRIGGER IF EXISTS trg_test_results_search ON test_results;
DROP FUNCTION IF EXISTS test_results_search_trigger();
DROP INDEX IF EXISTS idx_test_results_fts;
ALTER TABLE test_results DROP COLUMN IF EXISTS search_vector;
ALTER TABLE test_results DROP COLUMN IF EXISTS description;
ALTER TABLE test_results DROP COLUMN IF EXISTS status_trace;
ALTER TABLE test_results DROP COLUMN IF EXISTS status_message;
