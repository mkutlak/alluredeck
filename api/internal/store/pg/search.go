package pg

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"go.uber.org/zap"
)

// SearchStore provides cross-entity search using LIKE queries backed by PostgreSQL.
type SearchStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewSearchStore creates a SearchStore backed by the given PGStore.
func NewSearchStore(s *PGStore, logger *zap.Logger) *SearchStore {
	return &SearchStore{pool: s.pool, logger: logger}
}

// escapeLike escapes SQL LIKE wildcards so user input is treated literally.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// SearchProjects returns projects whose slug contains the query substring (case-insensitive).
func (ss *SearchStore) SearchProjects(ctx context.Context, query string, limit int) ([]store.ProjectMatch, error) {
	pattern := "%" + escapeLike(query) + "%"
	rows, err := ss.pool.Query(ctx,
		`SELECT id, slug, created_at FROM projects
		 WHERE slug LIKE $1 ESCAPE '\'
		 ORDER BY slug
		 LIMIT $2`,
		pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search projects: %w", err)
	}
	defer rows.Close()

	var results []store.ProjectMatch
	for rows.Next() {
		var m store.ProjectMatch
		if err := rows.Scan(&m.ID, &m.Slug, &m.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan project match: %w", err)
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project matches: %w", err)
	}
	if results == nil {
		results = []store.ProjectMatch{}
	}
	return results, nil
}

// SearchTests returns test results from latest builds whose search_vector matches
// the query using PostgreSQL full-text search (plainto_tsquery).
func (ss *SearchStore) SearchTests(ctx context.Context, query string, limit int) ([]store.TestMatch, error) {
	rows, err := ss.pool.Query(ctx,
		`SELECT DISTINCT tr.project_id, p.slug, tr.test_name, tr.full_name, tr.status
		 FROM test_results tr
		 INNER JOIN builds b ON b.id = tr.build_id
		 INNER JOIN projects p ON p.id = tr.project_id
		 WHERE b.is_latest = TRUE
		   AND tr.search_vector @@ plainto_tsquery('english', $1)
		 ORDER BY tr.project_id, tr.test_name
		 LIMIT $2`,
		query, limit)
	if err != nil {
		return nil, fmt.Errorf("search tests: %w", err)
	}
	defer rows.Close()

	var results []store.TestMatch
	for rows.Next() {
		var m store.TestMatch
		if err := rows.Scan(&m.ProjectID, &m.Slug, &m.TestName, &m.FullName, &m.Status); err != nil {
			return nil, fmt.Errorf("scan test match: %w", err)
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate test matches: %w", err)
	}
	if results == nil {
		results = []store.TestMatch{}
	}
	return results, nil
}

// SearchPipelineRuns returns the pipeline runs whose pipeline id equals the
// query exactly, newest first. A run is one group project plus one pipeline id,
// the feed's group_key, so a run that uploaded several suites or shards is one
// row. Only builds under a group project are feed runs. The branch is the
// newest build's, as in the feed, and empty when that build has none.
func (ss *SearchStore) SearchPipelineRuns(ctx context.Context, query string, limit int) ([]store.PipelineRunMatch, error) {
	rows, err := ss.pool.Query(ctx,
		`SELECT p.parent_id, gp.slug, b.ci_pipeline_id,
		        COALESCE((ARRAY_AGG(COALESCE(b.ci_branch, '') ORDER BY b.created_at DESC, b.id DESC))[1], ''),
		        MAX(b.created_at)
		 FROM builds b
		 JOIN projects p ON p.id = b.project_id
		 JOIN projects gp ON gp.id = p.parent_id
		 WHERE b.ci_pipeline_id = $1 AND p.parent_id IS NOT NULL
		 GROUP BY p.parent_id, gp.slug, b.ci_pipeline_id
		 ORDER BY MAX(b.created_at) DESC, p.parent_id
		 LIMIT $2`,
		query, limit)
	if err != nil {
		return nil, fmt.Errorf("search pipeline runs: %w", err)
	}
	defer rows.Close()

	results := []store.PipelineRunMatch{}
	for rows.Next() {
		var m store.PipelineRunMatch
		if err := rows.Scan(&m.GroupProjectID, &m.GroupSlug, &m.PipelineID, &m.Branch, &m.Timestamp); err != nil {
			return nil, fmt.Errorf("scan pipeline run match: %w", err)
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pipeline run matches: %w", err)
	}
	return results, nil
}

// SearchBranches returns the distinct branch names on builds under a group
// project whose name contains the query substring (case-insensitive). Builds
// with no run key (neither a pipeline id nor a commit SHA) are skipped like the
// feed skips them, so a branch found here always has runs to open.
// It reads builds.ci_branch, the value the feed filters on; the branches table
// is lazily populated and project-scoped, so it would miss branches.
func (ss *SearchStore) SearchBranches(ctx context.Context, query string, limit int) ([]store.BranchMatch, error) {
	// ponytail: ILIKE '%q%' cannot use an index, so this scans builds with a
	// branch (150-226 ms measured at 500k builds, hot cache); fine to roughly
	// 100k builds. Past that, add a pg_trgm GIN index on builds(ci_branch).
	rows, err := ss.pool.Query(ctx,
		`SELECT DISTINCT b.ci_branch
		 FROM builds b
		 JOIN projects p ON p.id = b.project_id
		 WHERE p.parent_id IS NOT NULL
		   AND b.ci_branch IS NOT NULL AND b.ci_branch <> ''
		   AND (b.ci_pipeline_id IS NOT NULL OR b.ci_commit_sha IS NOT NULL)
		   AND b.ci_branch ILIKE $1 ESCAPE '\'
		 ORDER BY b.ci_branch
		 LIMIT $2`,
		"%"+escapeLike(query)+"%", limit)
	if err != nil {
		return nil, fmt.Errorf("search branches: %w", err)
	}
	defer rows.Close()

	results := []store.BranchMatch{}
	for rows.Next() {
		var m store.BranchMatch
		if err := rows.Scan(&m.Name); err != nil {
			return nil, fmt.Errorf("scan branch match: %w", err)
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate branch matches: %w", err)
	}
	return results, nil
}

var _ store.SearchStorer = (*SearchStore)(nil)
