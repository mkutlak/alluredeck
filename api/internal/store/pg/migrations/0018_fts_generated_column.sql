-- +goose Up
-- Drop the PL/pgSQL trigger and function
DROP TRIGGER IF EXISTS trg_test_results_search ON test_results;
DROP FUNCTION IF EXISTS test_results_search_trigger();

-- Drop the GIN index before altering the column
DROP INDEX IF EXISTS idx_test_results_fts;

-- Replace the manually-maintained tsvector with a generated column.
-- PostgreSQL 12+ supports GENERATED ALWAYS AS ... STORED for tsvector.
ALTER TABLE test_results DROP COLUMN IF EXISTS search_vector;
ALTER TABLE test_results ADD COLUMN search_vector tsvector
  GENERATED ALWAYS AS (
    to_tsvector('english',
      COALESCE(test_name, '') || ' ' ||
      COALESCE(full_name, '') || ' ' ||
      COALESCE(status_message, ''))
  ) STORED;

CREATE INDEX idx_test_results_fts ON test_results USING GIN (search_vector);

-- +goose Down
-- Revert to trigger-based approach
DROP INDEX IF EXISTS idx_test_results_fts;
ALTER TABLE test_results DROP COLUMN IF EXISTS search_vector;
ALTER TABLE test_results ADD COLUMN search_vector tsvector;

CREATE INDEX idx_test_results_fts ON test_results USING GIN (search_vector);

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
