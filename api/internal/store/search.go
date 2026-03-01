package store

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"
)

// ProjectMatch holds a project returned by a search query.
type ProjectMatch struct {
	ID        string
	CreatedAt time.Time
}

// TestMatch holds a test result returned by a search query.
type TestMatch struct {
	ProjectID string
	TestName  string
	FullName  string
	Status    string
}

// SearchStore provides cross-entity search using LIKE queries.
type SearchStore struct {
	db     *sql.DB
	logger *zap.Logger
}

// NewSearchStore creates a SearchStore backed by the given SQLiteStore.
func NewSearchStore(s *SQLiteStore, logger *zap.Logger) *SearchStore {
	return &SearchStore{db: s.db, logger: logger}
}

// escapeLike escapes SQL LIKE wildcards so user input is treated literally.
func escapeLike(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// SearchProjects returns projects whose ID contains the query substring (case-insensitive).
func (ss *SearchStore) SearchProjects(ctx context.Context, query string, limit int) ([]ProjectMatch, error) {
	pattern := "%" + escapeLike(query) + "%"
	rows, err := ss.db.QueryContext(ctx,
		`SELECT id, created_at FROM projects
		 WHERE id LIKE ? ESCAPE '\'
		 ORDER BY id
		 LIMIT ?`,
		pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []ProjectMatch
	for rows.Next() {
		var m ProjectMatch
		var createdAt string
		if err := rows.Scan(&m.ID, &createdAt); err != nil {
			return nil, fmt.Errorf("scan project match: %w", err)
		}
		if t, err := time.Parse("2006-01-02T15:04:05Z", createdAt); err != nil {
			ss.logger.Warn("invalid created_at in search result",
				zap.String("created_at", createdAt), zap.String("project_id", m.ID), zap.Error(err))
		} else {
			m.CreatedAt = t
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project matches: %w", err)
	}
	if results == nil {
		results = []ProjectMatch{}
	}
	return results, nil
}

// SearchTests returns test results from latest builds whose test_name or
// full_name contains the query substring (case-insensitive).
func (ss *SearchStore) SearchTests(ctx context.Context, query string, limit int) ([]TestMatch, error) {
	pattern := "%" + escapeLike(query) + "%"
	rows, err := ss.db.QueryContext(ctx,
		`SELECT DISTINCT tr.project_id, tr.test_name, tr.full_name, tr.status
		 FROM test_results tr
		 INNER JOIN builds b ON b.id = tr.build_id
		 WHERE b.is_latest = 1
		   AND (tr.test_name LIKE ? ESCAPE '\' OR tr.full_name LIKE ? ESCAPE '\')
		 ORDER BY tr.project_id, tr.test_name
		 LIMIT ?`,
		pattern, pattern, limit)
	if err != nil {
		return nil, fmt.Errorf("search tests: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var results []TestMatch
	for rows.Next() {
		var m TestMatch
		if err := rows.Scan(&m.ProjectID, &m.TestName, &m.FullName, &m.Status); err != nil {
			return nil, fmt.Errorf("scan test match: %w", err)
		}
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate test matches: %w", err)
	}
	if results == nil {
		results = []TestMatch{}
	}
	return results, nil
}
