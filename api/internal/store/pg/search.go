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

// SearchProjects returns projects whose ID contains the query substring (case-insensitive).
func (ss *SearchStore) SearchProjects(ctx context.Context, query string, limit int) ([]store.ProjectMatch, error) {
	pattern := "%" + escapeLike(query) + "%"
	rows, err := ss.pool.Query(ctx,
		`SELECT id, created_at FROM projects
		 WHERE id LIKE $1 ESCAPE '\'
		 ORDER BY id
		 LIMIT $2`,
		pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search projects: %w", err)
	}
	defer rows.Close()

	var results []store.ProjectMatch
	for rows.Next() {
		var m store.ProjectMatch
		if err := rows.Scan(&m.ID, &m.CreatedAt); err != nil {
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
		`SELECT DISTINCT tr.project_id, tr.test_name, tr.full_name, tr.status
		 FROM test_results tr
		 INNER JOIN builds b ON b.id = tr.build_id
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
		if err := rows.Scan(&m.ProjectID, &m.TestName, &m.FullName, &m.Status); err != nil {
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

var _ store.SearchStorer = (*SearchStore)(nil)
