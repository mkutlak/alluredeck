CREATE INDEX IF NOT EXISTS idx_test_results_analytics
    ON test_results(project_id, build_id, history_id, status, duration_ms);
