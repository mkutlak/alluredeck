package pg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// PipelineStore provides cross-project pipeline run queries.
type PipelineStore struct {
	pool *pgxpool.Pool
}

var _ store.PipelineStorer = (*PipelineStore)(nil)

// NewPipelineStore creates a PipelineStore backed by the given PGStore's connection pool.
func NewPipelineStore(s *PGStore) *PipelineStore {
	return &PipelineStore{pool: s.pool}
}

const pipelineRunsQuery = `
WITH child_builds AS (
    SELECT COALESCE(b.ci_pipeline_id, '') AS ci_pipeline_id,
           COALESCE(b.ci_pipeline_url, '') AS ci_pipeline_url,
           COALESCE(b.ci_commit_sha, '') AS ci_commit_sha,
           COALESCE(b.ci_branch, '') AS ci_branch,
           COALESCE(b.ci_build_url, '') AS ci_build_url,
           b.created_at,
           b.project_id,
           p.slug AS project_slug,
           b.build_order,
           b.id AS build_id,
           b.stat_passed, b.stat_failed, b.stat_broken, b.stat_skipped, b.stat_total,
           b.duration_ms,
           COALESCE(b.ci_pipeline_id, b.ci_commit_sha) AS group_key
    FROM builds b
    JOIN projects p ON p.id = b.project_id
    WHERE p.parent_id = $1
      AND (b.ci_pipeline_id IS NOT NULL OR b.ci_commit_sha IS NOT NULL)
      AND ($2::text = '' OR b.ci_branch = $2)
),
distinct_groups AS (
    SELECT group_key,
           MAX(created_at) AS max_ts
    FROM child_builds
    GROUP BY group_key
    ORDER BY max_ts DESC
),
total_count AS (
    SELECT COUNT(*) AS cnt FROM distinct_groups
),
paginated_groups AS (
    SELECT group_key FROM distinct_groups
    ORDER BY max_ts DESC
    LIMIT $3 OFFSET $4
)
SELECT cb.ci_pipeline_id, cb.ci_pipeline_url,
       cb.ci_commit_sha, cb.ci_branch, cb.ci_build_url, cb.created_at,
       cb.project_id, cb.project_slug, cb.build_order, cb.build_id,
       cb.stat_passed, cb.stat_failed, cb.stat_broken, cb.stat_skipped, cb.stat_total,
       cb.duration_ms,
       tc.cnt
FROM child_builds cb
JOIN paginated_groups pg ON cb.group_key = pg.group_key
CROSS JOIN total_count tc
ORDER BY cb.created_at DESC, cb.project_id ASC`

// ListPipelineRuns returns builds from child projects of the given parent,
// paginated by distinct commit SHA. Returns flat rows that the caller groups.
func (s *PipelineStore) ListPipelineRuns(ctx context.Context, parentID int64, branch string, page, perPage int) ([]store.PipelineRunRow, int, error) {
	offset := (page - 1) * perPage

	rows, err := s.pool.Query(ctx, pipelineRunsQuery, parentID, branch, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("pipeline runs query: %w", err)
	}
	defer rows.Close()

	var (
		result []store.PipelineRunRow
		total  int
	)
	for rows.Next() {
		var r store.PipelineRunRow
		if err := rows.Scan(
			&r.PipelineID, &r.PipelineURL,
			&r.CommitSHA, &r.Branch, &r.CIBuildURL, &r.CreatedAt,
			&r.ProjectID, &r.Slug, &r.BuildNumber, &r.BuildID,
			&r.StatPassed, &r.StatFailed, &r.StatBroken, &r.StatSkipped, &r.StatTotal,
			&r.DurationMs,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan pipeline row: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("pipeline rows iteration: %w", err)
	}

	return result, total, nil
}

// allPipelineRunsQuery is the cross-parent generalization of pipelineRunsQuery.
// group_key is prefixed with the parent project ID so that the same commit SHA
// appearing under two different parent groups remains two separate runs.
const allPipelineRunsQuery = `
WITH child_builds AS (
    SELECT COALESCE(b.ci_pipeline_id, '') AS ci_pipeline_id,
           COALESCE(b.ci_pipeline_url, '') AS ci_pipeline_url,
           COALESCE(b.ci_commit_sha, '') AS ci_commit_sha,
           COALESCE(b.ci_branch, '') AS ci_branch,
           COALESCE(b.ci_build_url, '') AS ci_build_url,
           b.created_at,
           b.project_id,
           p.slug AS project_slug,
           b.build_order,
           b.id AS build_id,
           gp.id AS group_project_id,
           gp.slug AS group_slug,
           b.stat_passed, b.stat_failed, b.stat_broken, b.stat_skipped, b.stat_total,
           b.duration_ms,
           p.parent_id::text || ':' || COALESCE(b.ci_pipeline_id, b.ci_commit_sha) AS group_key
    FROM builds b
    JOIN projects p ON p.id = b.project_id
    JOIN projects gp ON gp.id = p.parent_id
    WHERE p.parent_id IS NOT NULL
      AND (b.ci_pipeline_id IS NOT NULL OR b.ci_commit_sha IS NOT NULL)
      AND ($1::text = '' OR b.ci_branch = $1)
      AND (cardinality($2::bigint[]) = 0 OR p.parent_id = ANY($2))
),
distinct_groups AS (
    SELECT group_key,
           MAX(created_at) AS max_ts
    FROM child_builds
    GROUP BY group_key
    ORDER BY max_ts DESC
),
total_count AS (
    SELECT COUNT(*) AS cnt FROM distinct_groups
),
paginated_groups AS (
    SELECT group_key FROM distinct_groups
    ORDER BY max_ts DESC
    LIMIT $3 OFFSET $4
)
SELECT cb.ci_pipeline_id, cb.ci_pipeline_url,
       cb.ci_commit_sha, cb.ci_branch, cb.ci_build_url, cb.created_at,
       cb.project_id, cb.project_slug, cb.build_order, cb.build_id,
       cb.group_project_id, cb.group_slug,
       cb.stat_passed, cb.stat_failed, cb.stat_broken, cb.stat_skipped, cb.stat_total,
       cb.duration_ms,
       tc.cnt
FROM child_builds cb
JOIN paginated_groups pg ON cb.group_key = pg.group_key
CROSS JOIN total_count tc
ORDER BY cb.created_at DESC, cb.project_id ASC`

// ListAllPipelineRuns returns builds across every parent/child project group,
// paginated by distinct (parent, commit-SHA-or-pipeline-ID) group. Returns
// flat rows that the caller groups. groupIDs must be non-nil (an empty slice
// disables the group filter); nil would break the cardinality() check pgx
// sends for a bigint[] parameter.
func (s *PipelineStore) ListAllPipelineRuns(ctx context.Context, branch string, groupIDs []int64, page, perPage int) ([]store.PipelineRunRow, int, error) {
	offset := (page - 1) * perPage
	if groupIDs == nil {
		groupIDs = []int64{}
	}

	rows, err := s.pool.Query(ctx, allPipelineRunsQuery, branch, groupIDs, perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("all pipeline runs query: %w", err)
	}
	defer rows.Close()

	var (
		result []store.PipelineRunRow
		total  int
	)
	for rows.Next() {
		var r store.PipelineRunRow
		if err := rows.Scan(
			&r.PipelineID, &r.PipelineURL,
			&r.CommitSHA, &r.Branch, &r.CIBuildURL, &r.CreatedAt,
			&r.ProjectID, &r.Slug, &r.BuildNumber, &r.BuildID,
			&r.GroupProjectID, &r.GroupSlug,
			&r.StatPassed, &r.StatFailed, &r.StatBroken, &r.StatSkipped, &r.StatTotal,
			&r.DurationMs,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan all pipeline row: %w", err)
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("all pipeline rows iteration: %w", err)
	}

	return result, total, nil
}
