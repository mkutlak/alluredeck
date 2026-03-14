package pg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// PGProjectStore provides CRUD operations on the projects table using PostgreSQL.
type PGProjectStore struct {
	pool   *pgxpool.Pool
	logger *zap.Logger
}

// NewProjectStore creates a PGProjectStore backed by the given PGStore.
func NewProjectStore(s *PGStore, logger *zap.Logger) *PGProjectStore {
	return &PGProjectStore{pool: s.pool, logger: logger}
}

// CreateProject inserts a new project. Returns store.ErrProjectExists if the ID is taken.
func (ps *PGProjectStore) CreateProject(ctx context.Context, id string) error {
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

// GetProject returns the project with the given ID or store.ErrProjectNotFound.
func (ps *PGProjectStore) GetProject(ctx context.Context, id string) (*store.Project, error) {
	var p store.Project
	var rawTags []byte
	err := ps.pool.QueryRow(ctx,
		"SELECT id, created_at, tags FROM projects WHERE id = $1", id,
	).Scan(&p.ID, &p.CreatedAt, &rawTags)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: %s", store.ErrProjectNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get project: %w", err)
	}
	p.Tags = parseTags(rawTags)
	return &p, nil
}

// ListProjects returns all projects ordered by ID.
func (ps *PGProjectStore) ListProjects(ctx context.Context) ([]store.Project, error) {
	rows, err := ps.pool.Query(ctx,
		"SELECT id, created_at, tags FROM projects ORDER BY id")
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()

	var projects []store.Project
	for rows.Next() {
		var p store.Project
		var rawTags []byte
		if err := rows.Scan(&p.ID, &p.CreatedAt, &rawTags); err != nil {
			return nil, fmt.Errorf("scan project: %w", err)
		}
		p.Tags = parseTags(rawTags)
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project rows: %w", err)
	}
	return projects, nil
}

// ListProjectsPaginated returns a page of projects, plus the total count.
// When tag is non-empty, only projects containing that tag are returned.
func (ps *PGProjectStore) ListProjectsPaginated(ctx context.Context, page, perPage int, tag string) ([]store.Project, int, error) {
	var total int
	if tag != "" {
		tagJSON, _ := json.Marshal([]string{tag})
		if err := ps.pool.QueryRow(ctx,
			"SELECT COUNT(*) FROM projects WHERE tags @> $1::jsonb", string(tagJSON),
		).Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("count projects (tag filter): %w", err)
		}
	} else {
		if err := ps.pool.QueryRow(ctx, "SELECT COUNT(*) FROM projects").Scan(&total); err != nil {
			return nil, 0, fmt.Errorf("count projects: %w", err)
		}
	}

	offset := (page - 1) * perPage
	var rows pgx.Rows
	var err error
	if tag != "" {
		tagJSON, _ := json.Marshal([]string{tag})
		rows, err = ps.pool.Query(ctx,
			"SELECT id, created_at, tags FROM projects WHERE tags @> $1::jsonb ORDER BY id LIMIT $2 OFFSET $3",
			string(tagJSON), perPage, offset)
	} else {
		rows, err = ps.pool.Query(ctx,
			"SELECT id, created_at, tags FROM projects ORDER BY id LIMIT $1 OFFSET $2",
			perPage, offset)
	}
	if err != nil {
		return nil, 0, fmt.Errorf("list projects paginated: %w", err)
	}
	defer rows.Close()

	var projects []store.Project
	for rows.Next() {
		var p store.Project
		var rawTags []byte
		if err := rows.Scan(&p.ID, &p.CreatedAt, &rawTags); err != nil {
			return nil, 0, fmt.Errorf("scan project: %w", err)
		}
		p.Tags = parseTags(rawTags)
		projects = append(projects, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate project rows: %w", err)
	}
	return projects, total, nil
}

// ListAllTags returns all distinct tags across all projects, sorted alphabetically.
func (ps *PGProjectStore) ListAllTags(ctx context.Context) ([]string, error) {
	rows, err := ps.pool.Query(ctx,
		"SELECT DISTINCT jsonb_array_elements_text(tags) AS tag FROM projects ORDER BY tag")
	if err != nil {
		return nil, fmt.Errorf("list all tags: %w", err)
	}
	defer rows.Close()

	var tags []string
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return nil, fmt.Errorf("scan tag: %w", err)
		}
		tags = append(tags, tag)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tag rows: %w", err)
	}
	return tags, nil
}

// SetTags replaces the tags for a project. Returns store.ErrProjectNotFound if the project does not exist.
func (ps *PGProjectStore) SetTags(ctx context.Context, projectID string, tags []string) error {
	if tags == nil {
		tags = []string{}
	}
	raw, err := json.Marshal(tags)
	if err != nil {
		return fmt.Errorf("marshal tags: %w", err)
	}
	tag, err := ps.pool.Exec(ctx,
		"UPDATE projects SET tags = $1::jsonb WHERE id = $2", string(raw), projectID)
	if err != nil {
		return fmt.Errorf("set tags: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", store.ErrProjectNotFound, projectID)
	}
	return nil
}

// DeleteProject removes a project and all its builds (CASCADE).
func (ps *PGProjectStore) DeleteProject(ctx context.Context, id string) error {
	tag, err := ps.pool.Exec(ctx, "DELETE FROM projects WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete project: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: %s", store.ErrProjectNotFound, id)
	}
	return nil
}

// ProjectExists returns true if a project with the given ID exists.
func (ps *PGProjectStore) ProjectExists(ctx context.Context, id string) (bool, error) {
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
func (ps *PGProjectStore) InsertOrIgnore(ctx context.Context, id string) error {
	_, err := ps.pool.Exec(ctx,
		"INSERT INTO projects(id) VALUES($1) ON CONFLICT (id) DO NOTHING", id)
	if err != nil {
		return fmt.Errorf("insert project %q: %w", id, err)
	}
	return nil
}

// parseTags unmarshals a JSON tags byte slice into a []string.
func parseTags(raw []byte) []string {
	if len(raw) == 0 {
		return []string{}
	}
	var tags []string
	if err := json.Unmarshal(raw, &tags); err != nil {
		return []string{}
	}
	if tags == nil {
		return []string{}
	}
	return tags
}

// isUniqueViolation returns true if err is a PostgreSQL unique constraint violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Compile-time interface check.
var _ store.ProjectStorer = (*PGProjectStore)(nil)
