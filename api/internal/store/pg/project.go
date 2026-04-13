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

// CreateProject inserts a new project. Returns the created project with generated ID.
// Returns store.ErrProjectExists if the slug is taken.
func (ps *ProjectStore) CreateProject(ctx context.Context, slug string) (*store.Project, error) {
	var p store.Project
	err := ps.pool.QueryRow(ctx,
		`INSERT INTO projects(slug, display_name) VALUES($1, $1) RETURNING id, slug, parent_id, display_name, report_type, created_at`,
		slug,
	).Scan(&p.ID, &p.Slug, &p.ParentID, &p.DisplayName, &p.ReportType, &p.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w: %s", store.ErrProjectExists, slug)
		}
		return nil, fmt.Errorf("create project: %w", err)
	}
	return &p, nil
}

// CreateProjectWithParent inserts a new project with the given parent ID.
// Returns store.ErrProjectExists if the slug is taken.
func (ps *ProjectStore) CreateProjectWithParent(ctx context.Context, slug string, parentID int64) (*store.Project, error) {
	var p store.Project
	err := ps.pool.QueryRow(ctx,
		`INSERT INTO projects(slug, parent_id, display_name) VALUES($1, $2, $1) RETURNING id, slug, parent_id, display_name, report_type, created_at`,
		slug, parentID,
	).Scan(&p.ID, &p.Slug, &p.ParentID, &p.DisplayName, &p.ReportType, &p.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, fmt.Errorf("%w: %s", store.ErrProjectExists, slug)
		}
		return nil, fmt.Errorf("insert project: %w", err)
	}
	return &p, nil
}

// GetProject returns the project with the given ID or store.ErrProjectNotFound.
func (ps *ProjectStore) GetProject(ctx context.Context, id int64) (*store.Project, error) {
	var p store.Project
	err := ps.pool.QueryRow(ctx,
		"SELECT id, slug, parent_id, display_name, report_type, created_at FROM projects WHERE id = $1", id,
	).Scan(&p.ID, &p.Slug, &p.ParentID, &p.DisplayName, &p.ReportType, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %d", store.ErrProjectNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	return &p, nil
}

// GetProjectBySlug returns the top-level project with the given slug or store.ErrProjectNotFound.
func (ps *ProjectStore) GetProjectBySlug(ctx context.Context, slug string) (*store.Project, error) {
	var p store.Project
	err := ps.pool.QueryRow(ctx,
		"SELECT id, slug, parent_id, display_name, report_type, created_at FROM projects WHERE slug = $1 AND parent_id IS NULL", slug,
	).Scan(&p.ID, &p.Slug, &p.ParentID, &p.DisplayName, &p.ReportType, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", store.ErrProjectNotFound, slug)
	}
	if err != nil {
		return nil, fmt.Errorf("get project by slug: %w", err)
	}
	return &p, nil
}

// GetProjectBySlugAny returns a project matching the slug regardless of parent status.
// If multiple projects share the slug (one top-level, one child), the top-level project is preferred.
func (ps *ProjectStore) GetProjectBySlugAny(ctx context.Context, slug string) (*store.Project, error) {
	var p store.Project
	err := ps.pool.QueryRow(ctx,
		"SELECT id, slug, parent_id, display_name, report_type, created_at FROM projects WHERE slug = $1 ORDER BY parent_id IS NOT NULL, id LIMIT 1", slug,
	).Scan(&p.ID, &p.Slug, &p.ParentID, &p.DisplayName, &p.ReportType, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", store.ErrProjectNotFound, slug)
	}
	if err != nil {
		return nil, fmt.Errorf("get project by slug any: %w", err)
	}
	return &p, nil
}

// ListProjects returns all projects ordered by ID.
func (ps *ProjectStore) ListProjects(ctx context.Context) ([]store.Project, error) {
	rows, err := ps.pool.Query(ctx,
		"SELECT id, slug, parent_id, display_name, report_type, created_at FROM projects ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []store.Project
	for rows.Next() {
		var p store.Project
		if err := rows.Scan(&p.ID, &p.Slug, &p.ParentID, &p.DisplayName, &p.ReportType, &p.CreatedAt); err != nil {
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
		"SELECT id, slug, parent_id, display_name, report_type, created_at FROM projects ORDER BY id LIMIT $1 OFFSET $2",
		perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list projects paginated: %w", err)
	}
	defer rows.Close()

	var projects []store.Project
	for rows.Next() {
		var p store.Project
		if err := rows.Scan(&p.ID, &p.Slug, &p.ParentID, &p.DisplayName, &p.ReportType, &p.CreatedAt); err != nil {
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
		"SELECT id, slug, parent_id, display_name, report_type, created_at FROM projects WHERE parent_id IS NULL ORDER BY id LIMIT $1 OFFSET $2",
		perPage, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list top-level projects paginated: %w", err)
	}
	defer rows.Close()

	var projects []store.Project
	for rows.Next() {
		var p store.Project
		if err := rows.Scan(&p.ID, &p.Slug, &p.ParentID, &p.DisplayName, &p.ReportType, &p.CreatedAt); err != nil {
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
func (ps *ProjectStore) ListChildren(ctx context.Context, parentID int64) ([]store.Project, error) {
	rows, err := ps.pool.Query(ctx,
		"SELECT id, slug, parent_id, display_name, report_type, created_at FROM projects WHERE parent_id = $1 ORDER BY id", parentID)
	if err != nil {
		return nil, fmt.Errorf("list children: %w", err)
	}
	defer rows.Close()

	var projects []store.Project
	for rows.Next() {
		var p store.Project
		if err := rows.Scan(&p.ID, &p.Slug, &p.ParentID, &p.DisplayName, &p.ReportType, &p.CreatedAt); err != nil {
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
func (ps *ProjectStore) HasChildren(ctx context.Context, projectID int64) (bool, error) {
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
func (ps *ProjectStore) SetParent(ctx context.Context, projectID, parentID int64) error {
	tag, err := ps.pool.Exec(ctx,
		"UPDATE projects SET parent_id = $1 WHERE id = $2", parentID, projectID)
	if err != nil {
		return fmt.Errorf("set parent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %d", store.ErrProjectNotFound, projectID)
	}
	return nil
}

// ClearParent sets parent_id to NULL for a project. Returns store.ErrProjectNotFound if the project does not exist.
func (ps *ProjectStore) ClearParent(ctx context.Context, projectID int64) error {
	tag, err := ps.pool.Exec(ctx,
		"UPDATE projects SET parent_id = NULL WHERE id = $1", projectID)
	if err != nil {
		return fmt.Errorf("clear parent: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %d", store.ErrProjectNotFound, projectID)
	}
	return nil
}

// DeleteProject removes a project and all its builds (CASCADE).
func (ps *ProjectStore) DeleteProject(ctx context.Context, id int64) error {
	tag, err := ps.pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %d", store.ErrProjectNotFound, id)
	}
	return nil
}

// RenameProject updates a project's slug and display_name.
func (ps *ProjectStore) RenameProject(ctx context.Context, id int64, newSlug string) error {
	tag, err := ps.pool.Exec(ctx, "UPDATE projects SET slug = $1, display_name = $1 WHERE id = $2", newSlug, id)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("%w: %s", store.ErrProjectExists, newSlug)
		}
		return fmt.Errorf("rename project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %d", store.ErrProjectNotFound, id)
	}
	return nil
}

// ProjectExists returns true if a project with the given ID exists.
func (ps *ProjectStore) ProjectExists(ctx context.Context, id int64) (bool, error) {
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
func (ps *ProjectStore) InsertOrIgnore(ctx context.Context, slug string) error {
	_, err := ps.pool.Exec(ctx, `
		INSERT INTO projects(slug, display_name)
		SELECT $1, $1
		WHERE NOT EXISTS (SELECT 1 FROM projects WHERE slug = $1)
	`, slug)
	if err != nil {
		return fmt.Errorf("insert project %q: %w", slug, err)
	}
	return nil
}

// SetReportType updates the report_type for a project.
func (ps *ProjectStore) SetReportType(ctx context.Context, id int64, reportType string) error {
	_, err := ps.pool.Exec(ctx,
		"UPDATE projects SET report_type = $1 WHERE id = $2", reportType, id)
	if err != nil {
		return fmt.Errorf("set report type: %w", err)
	}
	return nil
}

// ListChildIDs returns the slugs of all projects whose parent_id matches the given ID.
func (ps *ProjectStore) ListChildIDs(ctx context.Context, parentID int64) ([]string, error) {
	rows, err := ps.pool.Query(ctx, "SELECT slug FROM projects WHERE parent_id = $1 ORDER BY slug", parentID)
	if err != nil {
		return nil, fmt.Errorf("list children: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan child id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// isUniqueViolation returns true if err is a PostgreSQL unique constraint violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Compile-time interface check.
var _ store.ProjectStorer = (*ProjectStore)(nil)
