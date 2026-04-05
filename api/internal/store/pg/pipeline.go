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

// NewPipelineStore creates a PipelineStore backed by the given PGStore's connection pool.
func NewPipelineStore(s *PGStore) *PipelineStore {
	return &PipelineStore{pool: s.pool}
}

const pipelineRunsQuery = `
WITH child_builds AS (
    SELECT b.ci_commit_sha,
           COALESCE(b.ci_branch, '') AS ci_branch,
           COALESCE(b.ci_build_url, '') AS ci_build_url,
           b.created_at,
           b.project_id,
           b.build_order,
           b.stat_passed, b.stat_failed, b.stat_broken, b.stat_total,
           b.duration_ms
    FROM builds b
    JOIN projects p ON p.id = b.project_id
    WHERE p.parent_id = $1
      AND b.ci_commit_sha IS NOT NULL
      AND ($2::text = '' OR b.ci_branch = $2)
),
distinct_shas AS (
    SELECT ci_commit_sha,
           MAX(created_at) AS max_ts
    FROM child_builds
    GROUP BY ci_commit_sha
    ORDER BY max_ts DESC
),
total_count AS (
    SELECT COUNT(*) AS cnt FROM distinct_shas
),
paginated_shas AS (
    SELECT ci_commit_sha FROM distinct_shas
    LIMIT $3 OFFSET $4
)
SELECT cb.ci_commit_sha, cb.ci_branch, cb.ci_build_url, cb.created_at,
       cb.project_id, cb.build_order,
       cb.stat_passed, cb.stat_failed, cb.stat_broken, cb.stat_total,
       cb.duration_ms,
       tc.cnt
FROM child_builds cb
JOIN paginated_shas ps ON ps.ci_commit_sha = cb.ci_commit_sha
CROSS JOIN total_count tc
ORDER BY cb.created_at DESC, cb.project_id ASC`

// ListPipelineRuns returns builds from child projects of the given parent,
// paginated by distinct commit SHA. Returns flat rows that the caller groups.
func (s *PipelineStore) ListPipelineRuns(ctx context.Context, parentID string, branch string, page, perPage int) ([]store.PipelineRunRow, int, error) {
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
			&r.CommitSHA, &r.Branch, &r.CIBuildURL, &r.CreatedAt,
			&r.ProjectID, &r.BuildNumber,
			&r.StatPassed, &r.StatFailed, &r.StatBroken, &r.StatTotal,
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
