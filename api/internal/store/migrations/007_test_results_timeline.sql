-- Add timeline columns for serving timeline data from SQLite instead of N+1 S3 reads.
-- start_ms/stop_ms are nullable: old rows (pre-migration) won't have values.
-- thread/host default to empty string (matches codebase convention).
ALTER TABLE test_results ADD COLUMN start_ms INTEGER;
ALTER TABLE test_results ADD COLUMN stop_ms  INTEGER;
ALTER TABLE test_results ADD COLUMN thread   TEXT NOT NULL DEFAULT '';
ALTER TABLE test_results ADD COLUMN host     TEXT NOT NULL DEFAULT '';
