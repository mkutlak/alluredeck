package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// KnownIssueStore provides operations on the known_issues table using PostgreSQL.
type KnownIssueStore struct {
	pool *pgxpool.Pool
}

// NewKnownIssueStore creates a KnownIssueStore backed by the given PGStore.
func NewKnownIssueStore(s *PGStore) *KnownIssueStore {
	return &KnownIssueStore{pool: s.pool}
}

// Create inserts a new known issue for the given project.
func (ks *KnownIssueStore) Create(ctx context.Context, projectID, testName, pattern, ticketURL, description string) (*store.KnownIssue, error) {
	var id int64
	err := ks.pool.QueryRow(ctx,
		"INSERT INTO known_issues(project_id, test_name, pattern, ticket_url, description) VALUES($1,$2,$3,$4,$5) RETURNING id",
		projectID, testName, pattern, ticketURL, description,
	).Scan(&id)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("create known issue: %w", store.ErrDuplicateEntry)
		}
		return nil, fmt.Errorf("create known issue: %w", err)
	}
	return ks.Get(ctx, id)
}

// Get returns a single known issue by ID.
func (ks *KnownIssueStore) Get(ctx context.Context, id int64) (*store.KnownIssue, error) {
	var ki store.KnownIssue
	err := ks.pool.QueryRow(ctx, `
		SELECT id, project_id, test_name, pattern, ticket_url, description, is_active, created_at, updated_at
		FROM known_issues WHERE id = $1`, id,
	).Scan(&ki.ID, &ki.ProjectID, &ki.TestName, &ki.Pattern, &ki.TicketURL, &ki.Description,
		&ki.IsActive, &ki.CreatedAt, &ki.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%d", store.ErrKnownIssueNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get known issue: %w", err)
	}
	return &ki, nil
}

// List returns all known issues for a project. When activeOnly=true, only is_active=TRUE rows.
func (ks *KnownIssueStore) List(ctx context.Context, projectID string, activeOnly bool) ([]store.KnownIssue, error) {
	query := `SELECT id, project_id, test_name, pattern, ticket_url, description, is_active, created_at, updated_at
		FROM known_issues WHERE project_id = $1`
	args := []any{projectID}
	if activeOnly {
		query += " AND is_active = TRUE"
	}
	query += " ORDER BY id DESC"

	rows, err := ks.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list known issues: %w", err)
	}
	defer rows.Close()
	return ks.scanRows(rows)
}

// ListPaginated returns a page of known issues and the total count.
func (ks *KnownIssueStore) ListPaginated(ctx context.Context, projectID string, activeOnly bool, page, perPage int) ([]store.KnownIssue, int, error) {
	whereClause := "WHERE project_id = $1"
	args := []any{projectID}
	if activeOnly {
		whereClause += " AND is_active = TRUE"
	}

	var total int
	if err := ks.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM known_issues "+whereClause, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count known issues: %w", err)
	}

	offset := (page - 1) * perPage
	// whereClause is built from hardcoded SQL fragments only; no user input is concatenated.
	paginatedQuery := `SELECT id, project_id, test_name, pattern, ticket_url, description, is_active, created_at, updated_at` + //nolint:gosec // G202: false positive — whereClause contains only hardcoded SQL, no user input
		` FROM known_issues ` + whereClause + fmt.Sprintf(" ORDER BY id DESC LIMIT $%d OFFSET $%d", len(args)+1, len(args)+2)
	rows, err := ks.pool.Query(ctx, paginatedQuery, append(args, perPage, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list paginated known issues: %w", err)
	}
	defer rows.Close()

	items, err := ks.scanRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Update modifies ticket_url, description, and is_active for the given issue.
// The projectID parameter ensures updates are scoped to the correct project,
// preventing cross-project IDOR attacks.
func (ks *KnownIssueStore) Update(ctx context.Context, id int64, projectID, ticketURL, description string, isActive bool) error {
	tag, err := ks.pool.Exec(ctx, `
		UPDATE known_issues
		SET ticket_url=$1, description=$2, is_active=$3, updated_at=NOW()
		WHERE id=$4 AND project_id=$5`,
		ticketURL, description, isActive, id, projectID)
	if err != nil {
		return fmt.Errorf("update known issue: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%d", store.ErrKnownIssueNotFound, id)
	}
	return nil
}

// Delete removes a known issue by ID, scoped to the given project.
// The projectID parameter prevents cross-project IDOR attacks.
func (ks *KnownIssueStore) Delete(ctx context.Context, id int64, projectID string) error {
	tag, err := ks.pool.Exec(ctx, "DELETE FROM known_issues WHERE id=$1 AND project_id=$2", id, projectID)
	if err != nil {
		return fmt.Errorf("delete known issue: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%d", store.ErrKnownIssueNotFound, id)
	}
	return nil
}

// IsKnown returns true if an active known issue with the given test_name exists for the project.
func (ks *KnownIssueStore) IsKnown(ctx context.Context, projectID, testName string) (bool, error) {
	var count int
	err := ks.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM known_issues WHERE project_id=$1 AND test_name=$2 AND is_active=TRUE",
		projectID, testName,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("is known: %w", err)
	}
	return count > 0, nil
}

func (ks *KnownIssueStore) scanRows(rows pgx.Rows) ([]store.KnownIssue, error) {
	var issues []store.KnownIssue
	for rows.Next() {
		var ki store.KnownIssue
		if err := rows.Scan(&ki.ID, &ki.ProjectID, &ki.TestName, &ki.Pattern,
			&ki.TicketURL, &ki.Description, &ki.IsActive, &ki.CreatedAt, &ki.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan known issue: %w", err)
		}
		issues = append(issues, ki)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate known issue rows: %w", err)
	}
	return issues, nil
}

// Compile-time interface check.
var _ store.KnownIssueStorer = (*KnownIssueStore)(nil)
