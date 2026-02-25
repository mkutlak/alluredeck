package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrProjectNotFound is returned when a project does not exist.
var ErrProjectNotFound = errors.New("project not found")

// ErrProjectExists is returned when creating a project that already exists.
var ErrProjectExists = errors.New("project already exists")

// Project holds project metadata from the database.
type Project struct {
	ID        string
	CreatedAt time.Time
}

// ProjectStore provides CRUD operations on the projects table.
type ProjectStore struct {
	db *sql.DB
}

// NewProjectStore creates a ProjectStore backed by the given SQLiteStore.
func NewProjectStore(s *SQLiteStore) *ProjectStore {
	return &ProjectStore{db: s.db}
}

// CreateProject inserts a new project. Returns ErrProjectExists if the ID is taken.
func (ps *ProjectStore) CreateProject(ctx context.Context, id string) error {
	_, err := ps.db.ExecContext(ctx,
		"INSERT INTO projects(id) VALUES(?)", id)
	if err != nil {
		if isUniqueConstraintError(err) {
			return fmt.Errorf("%w: %s", ErrProjectExists, id)
		}
		return fmt.Errorf("create project: %w", err)
	}
	return nil
}

// GetProject returns the project with the given ID or ErrProjectNotFound.
func (ps *ProjectStore) GetProject(ctx context.Context, id string) (*Project, error) {
	var p Project
	var createdAt string
	err := ps.db.QueryRowContext(ctx,
		"SELECT id, created_at FROM projects WHERE id = ?", id,
	).Scan(&p.ID, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", ErrProjectNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	p.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAt)
	return &p, nil
}

// ListProjects returns all projects ordered by ID.
func (ps *ProjectStore) ListProjects(ctx context.Context) ([]Project, error) {
	rows, err := ps.db.QueryContext(ctx,
		"SELECT id, created_at FROM projects ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []Project
	for rows.Next() {
		var p Project
		var createdAt string
		if err := rows.Scan(&p.ID, &createdAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		p.CreatedAt, _ = time.Parse("2006-01-02T15:04:05Z", createdAt)
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project rows: %w", err)
	}
	return projects, nil
}

// DeleteProject removes a project and all its builds (CASCADE).
func (ps *ProjectStore) DeleteProject(ctx context.Context, id string) error {
	res, err := ps.db.ExecContext(ctx, "DELETE FROM projects WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: %s", ErrProjectNotFound, id)
	}
	return nil
}

// ProjectExists returns true if a project with the given ID exists.
func (ps *ProjectStore) ProjectExists(ctx context.Context, id string) (bool, error) {
	var count int
	err := ps.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM projects WHERE id = ?", id,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("project exists: %w", err)
	}
	return count > 0, nil
}

// isUniqueConstraintError returns true if err is a SQLite UNIQUE constraint violation.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	return containsAny(err.Error(), "UNIQUE constraint failed", "unique constraint failed")
}

func containsAny(s string, substrings ...string) bool {
	for _, sub := range substrings {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}
