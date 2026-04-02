package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// ProjectStore provides CRUD operations on the projects table using PostgreSQL.
type ProjectStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewProjectStore creates a ProjectStore backed by the given PGStore.
func NewProjectStore(s *PGStore, logger *zap.Logger) *ProjectStore {
	return &ProjectStore{pool: s.pool, logger: logger}
}

// CreateProject inserts a new project. Returns store.ErrProjectExists if the ID is taken.
func (ps *ProjectStore) CreateProject(ctx context.Context, id string) error {
	_, err := ps.pool.Exec(ctx,
		"INSERT INTO projects(id) VALUES($1)", id)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %s", store.ErrProjectExists, id)
		}
		return fmt.Errorf("create project: %w", err)
	}
	return nil
}

// CreateProjectWithParent inserts a new project with the given parent ID.
// Returns store.ErrProjectExists if the ID is taken.
func (ps *ProjectStore) CreateProjectWithParent(ctx context.Context, id string, parentID string) error {
	_, err := ps.pool.Exec(ctx,
		"INSERT INTO projects(id, parent_id) VALUES($1, $2)", id, parentID)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %s", store.ErrProjectExists, id)
		}
		return fmt.Errorf("insert project: %w", err)
	}
	return nil
}

// GetProject returns the project with the given ID or store.ErrProjectNotFound.
func (ps *ProjectStore) GetProject(ctx context.Context, id string) (*store.Project, error) {
	var p store.Project
	err := ps.pool.QueryRow(ctx,
		"SELECT id, parent_id, report_type, created_at FROM projects WHERE id = $1", id,
	).Scan(&p.ID, &p.ParentID, &p.ReportType, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", store.ErrProjectNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	return &p, nil
}

// ListProjects returns all projects ordered by ID.
func (ps *ProjectStore) ListProjects(ctx context.Context) ([]store.Project, error) {
	rows, err := ps.pool.Query(ctx,
		"SELECT id, parent_id, report_type, created_at FROM projects ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []store.Project
	for rows.Next() {
		var p store.Project
		if err := rows.Scan(&p.ID, &p.ParentID, &p.ReportType, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project rows: %w", err)
	}
	return projects, nil
}

// ListProjectsPaginated returns a page of projects, plus the total count.
func (ps *ProjectStore) ListProjectsPaginated(ctx context.Context, page, perPage int) ([]store.Project, int, error) {
	var total int
	if err := ps.pool.QueryRow(ctx, "SELECT COUNT(*) FROM projects").Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count projects: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := ps.pool.Query(ctx,
		"SELECT id, parent_id, report_type, created_at FROM projects ORDER BY id LIMIT $1 OFFSET $2",
		perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list projects paginated: %w", err)
	}
	defer rows.Close()

	var projects []store.Project
	for rows.Next() {
		var p store.Project
		if err := rows.Scan(&p.ID, &p.ParentID, &p.ReportType, &p.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate project rows: %w", err)
	}
	return projects, total, nil
}

// ListProjectsPaginatedTopLevel returns a page of top-level projects (parent_id IS NULL), plus the total count.
func (ps *ProjectStore) ListProjectsPaginatedTopLevel(ctx context.Context, page, perPage int) ([]store.Project, int, error) {
	var total int
	if err := ps.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM projects WHERE parent_id IS NULL",
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count top-level projects: %w", err)
	}

	offset := (page - 1) * perPage
	rows, err := ps.pool.Query(ctx,
		"SELECT id, parent_id, report_type, created_at FROM projects WHERE parent_id IS NULL ORDER BY id LIMIT $1 OFFSET $2",
		perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list top-level projects paginated: %w", err)
	}
	defer rows.Close()

	var projects []store.Project
	for rows.Next() {
		var p store.Project
		if err := rows.Scan(&p.ID, &p.ParentID, &p.ReportType, &p.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate project rows: %w", err)
	}
	return projects, total, nil
}

// ListChildren returns all child projects for a given parent project ID, ordered by ID.
func (ps *ProjectStore) ListChildren(ctx context.Context, parentID string) ([]store.Project, error) {
	rows, err := ps.pool.Query(ctx,
		"SELECT id, parent_id, report_type, created_at FROM projects WHERE parent_id = $1 ORDER BY id", parentID)
	if err != nil {
		return nil, fmt.Errorf("list children: %w", err)
	}
	defer rows.Close()

	var projects []store.Project
	for rows.Next() {
		var p store.Project
		if err := rows.Scan(&p.ID, &p.ParentID, &p.ReportType, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project rows: %w", err)
	}
	return projects, nil
}

// HasChildren returns true if any project has the given projectID as its parent.
func (ps *ProjectStore) HasChildren(ctx context.Context, projectID string) (bool, error) {
	var exists bool
	err := ps.pool.QueryRow(ctx,
		"SELECT EXISTS(SELECT 1 FROM projects WHERE parent_id = $1)", projectID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check children: %w", err)
	}
	return exists, nil
}

// SetParent sets the parent_id for a project. Returns store.ErrProjectNotFound if the project does not exist.
func (ps *ProjectStore) SetParent(ctx context.Context, projectID, parentID string) error {
	tag, err := ps.pool.Exec(ctx,
		"UPDATE projects SET parent_id = $1 WHERE id = $2", parentID, projectID)
	if err != nil {
		return fmt.Errorf("set parent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", store.ErrProjectNotFound, projectID)
	}
	return nil
}

// ClearParent sets parent_id to NULL for a project. Returns store.ErrProjectNotFound if the project does not exist.
func (ps *ProjectStore) ClearParent(ctx context.Context, projectID string) error {
	tag, err := ps.pool.Exec(ctx,
		"UPDATE projects SET parent_id = NULL WHERE id = $1", projectID)
	if err != nil {
		return fmt.Errorf("clear parent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", store.ErrProjectNotFound, projectID)
	}
	return nil
}

// DeleteProject removes a project and all its builds (CASCADE).
func (ps *ProjectStore) DeleteProject(ctx context.Context, id string) error {
	tag, err := ps.pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", store.ErrProjectNotFound, id)
	}
	return nil
}

// RenameProject changes a project's ID. ON UPDATE CASCADE propagates to all child tables.
func (ps *ProjectStore) RenameProject(ctx context.Context, oldID, newID string) error {
	tag, err := ps.pool.Exec(ctx, "UPDATE projects SET id = $1 WHERE id = $2", newID, oldID)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %s", store.ErrProjectExists, newID)
		}
		return fmt.Errorf("rename project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", store.ErrProjectNotFound, oldID)
	}
	return nil
}

// ProjectExists returns true if a project with the given ID exists.
func (ps *ProjectStore) ProjectExists(ctx context.Context, id string) (bool, error) {
	var count int
	err := ps.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM projects WHERE id = $1", id,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("project exists: %w", err)
	}
	return count > 0, nil
}

// InsertOrIgnore inserts a project row, silently ignoring duplicate-key errors.
// Used by SyncMetadata.
func (ps *ProjectStore) InsertOrIgnore(ctx context.Context, id string) error {
	_, err := ps.pool.Exec(ctx,
		"INSERT INTO projects(id) VALUES($1) ON CONFLICT (id) DO NOTHING", id)
	if err != nil {
		return fmt.Errorf("insert project %q: %w", id, err)
	}
	return nil
}

// SetReportType updates the report_type for a project.
func (ps *ProjectStore) SetReportType(ctx context.Context, id, reportType string) error {
	_, err := ps.pool.Exec(ctx,
		"UPDATE projects SET report_type = $1 WHERE id = $2", reportType, id)
	if err != nil {
		return fmt.Errorf("set report type: %w", err)
	}
	return nil
}

// isUniqueViolation returns true if err is a PostgreSQL unique constraint violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Compile-time interface check.
var _ store.ProjectStorer = (*ProjectStore)(nil)
