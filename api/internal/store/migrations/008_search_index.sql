-- Search index: speeds up JOIN builds.is_latest → test_results.build_id
-- with LIKE filters on test_name / full_name.
CREATE INDEX IF NOT EXISTS idx_test_results_search
    ON test_results(build_id, test_name, full_name);
