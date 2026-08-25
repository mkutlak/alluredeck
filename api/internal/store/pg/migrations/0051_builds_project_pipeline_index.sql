-- +goose Up
-- Support ListBuildsByPipelineID: WHERE project_id=$1 AND ci_pipeline_id=$2.
--
-- Migration 0035 already indexes builds(ci_pipeline_id), but that index answers
-- the cross-project question ListAllPipelineRuns asks. The single-project shard
-- lookup filters on BOTH columns, and a CI pipeline id is not unique across
-- projects — a monorepo runs one pipeline that uploads to several projects — so
-- the single-column index makes the planner read every project's builds for
-- that pipeline and discard the ones belonging to other projects.
--
-- The partial predicate is only IS NOT NULL, deliberately narrower than
-- "IS NOT NULL AND <> ''". A partial index is usable only when the planner can
-- PROVE the query implies its predicate: ci_pipeline_id = $2 implies IS NOT
-- NULL for any $2, but implies <> '' only when the parameter value is known at
-- plan time. pgx sends these as bound parameters, so once PostgreSQL switches a
-- prepared statement to a generic plan the <> '' form would stop being usable
-- and the query would silently fall back to a scan. Builds with an empty-string
-- ci_pipeline_id are rare enough not to be worth that risk.
CREATE INDEX IF NOT EXISTS idx_builds_project_pipeline
    ON builds(project_id, ci_pipeline_id)
    WHERE ci_pipeline_id IS NOT NULL;

-- +goose Down
DROP INDEX IF EXISTS idx_builds_project_pipeline;
