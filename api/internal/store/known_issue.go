package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrKnownIssueNotFound is returned when a known issue does not exist.
var ErrKnownIssueNotFound = errors.New("known issue not found")

// KnownIssue holds known-issue metadata from the database.
type KnownIssue struct {
	ID          int64
	ProjectID   string
	TestName    string
	Pattern     string
	TicketURL   string
	Description string
	IsActive    bool
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// KnownIssueStore provides operations on the known_issues table.
type KnownIssueStore struct {
	db *sql.DB
}

// NewKnownIssueStore creates a KnownIssueStore backed by the given SQLiteStore.
func NewKnownIssueStore(s *SQLiteStore) *KnownIssueStore {
	return &KnownIssueStore{db: s.db}
}

// Create inserts a new known issue for the given project.
func (ks *KnownIssueStore) Create(ctx context.Context, projectID, testName, pattern, ticketURL, description string) (*KnownIssue, error) {
	res, err := ks.db.ExecContext(ctx, `
		INSERT INTO known_issues(project_id, test_name, pattern, ticket_url, description)
		VALUES(?, ?, ?, ?, ?)`,
		projectID, testName, pattern, ticketURL, description)
	if err != nil {
		return nil, fmt.Errorf("create known issue: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("last insert id: %w", err)
	}
	return ks.Get(ctx, id)
}

// Get returns a single known issue by ID.
func (ks *KnownIssueStore) Get(ctx context.Context, id int64) (*KnownIssue, error) {
	var ki KnownIssue
	var createdAt, updatedAt string
	var isActive int
	err := ks.db.QueryRowContext(ctx, `
		SELECT id, project_id, test_name, pattern, ticket_url, description, is_active, created_at, updated_at
		FROM known_issues WHERE id = ?`, id,
	).Scan(&ki.ID, &ki.ProjectID, &ki.TestName, &ki.Pattern, &ki.TicketURL, &ki.Description,
		&isActive, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%d", ErrKnownIssueNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get known issue: %w", err)
	}
	ki.IsActive = isActive == 1
	if t, err := time.Parse("2006-01-02T15:04:05Z", createdAt); err == nil {
		ki.CreatedAt = t
	}
	if t, err := time.Parse("2006-01-02T15:04:05Z", updatedAt); err == nil {
		ki.UpdatedAt = t
	}
	return &ki, nil
}

// List returns all known issues for a project. When activeOnly=true, only is_active=1 rows.
func (ks *KnownIssueStore) List(ctx context.Context, projectID string, activeOnly bool) ([]KnownIssue, error) {
	query := `SELECT id, project_id, test_name, pattern, ticket_url, description, is_active, created_at, updated_at
		FROM known_issues WHERE project_id = ?`
	args := []any{projectID}
	if activeOnly {
		query += " AND is_active = 1"
	}
	query += " ORDER BY id DESC"

	rows, err := ks.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list known issues: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return ks.scanRows(rows)
}

// ListPaginated returns a page of known issues and the total count.
func (ks *KnownIssueStore) ListPaginated(ctx context.Context, projectID string, activeOnly bool, page, perPage int) ([]KnownIssue, int, error) {
	whereClause := "WHERE project_id = ?"
	args := []any{projectID}
	if activeOnly {
		whereClause += " AND is_active = 1"
	}

	var total int
	if err := ks.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM known_issues "+whereClause, args...,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count known issues: %w", err)
	}

	offset := (page - 1) * perPage
	// whereClause is built from hardcoded SQL fragments only; no user input is concatenated.
	paginatedQuery := `SELECT id, project_id, test_name, pattern, ticket_url, description, is_active, created_at, updated_at` + //nolint:gosec // G202: false positive — whereClause contains only hardcoded SQL, no user input
		` FROM known_issues ` + whereClause + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	rows, err := ks.db.QueryContext(ctx, paginatedQuery, append(args, perPage, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("list paginated known issues: %w", err)
	}
	defer func() { _ = rows.Close() }()

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
	active := 0
	if isActive {
		active = 1
	}
	res, err := ks.db.ExecContext(ctx, `
		UPDATE known_issues
		SET ticket_url=?, description=?, is_active=?,
		    updated_at=strftime('%Y-%m-%dT%H:%M:%SZ','now')
		WHERE id=? AND project_id=?`,
		ticketURL, description, active, id, projectID)
	if err != nil {
		return fmt.Errorf("update known issue: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: id=%d", ErrKnownIssueNotFound, id)
	}
	return nil
}

// Delete removes a known issue by ID, scoped to the given project.
// The projectID parameter prevents cross-project IDOR attacks.
func (ks *KnownIssueStore) Delete(ctx context.Context, id int64, projectID string) error {
	res, err := ks.db.ExecContext(ctx, "DELETE FROM known_issues WHERE id=? AND project_id=?", id, projectID)
	if err != nil {
		return fmt.Errorf("delete known issue: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("%w: id=%d", ErrKnownIssueNotFound, id)
	}
	return nil
}

// IsKnown returns true if an active known issue with the given test_name exists for the project.
func (ks *KnownIssueStore) IsKnown(ctx context.Context, projectID, testName string) (bool, error) {
	var count int
	err := ks.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM known_issues WHERE project_id=? AND test_name=? AND is_active=1",
		projectID, testName,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("is known: %w", err)
	}
	return count > 0, nil
}

func (ks *KnownIssueStore) scanRows(rows *sql.Rows) ([]KnownIssue, error) {
	var issues []KnownIssue
	for rows.Next() {
		var ki KnownIssue
		var createdAt, updatedAt string
		var isActive int
		if err := rows.Scan(&ki.ID, &ki.ProjectID, &ki.TestName, &ki.Pattern,
			&ki.TicketURL, &ki.Description, &isActive, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan known issue: %w", err)
		}
		ki.IsActive = isActive == 1
		if t, err := time.Parse("2006-01-02T15:04:05Z", createdAt); err == nil {
			ki.CreatedAt = t
		}
		if t, err := time.Parse("2006-01-02T15:04:05Z", updatedAt); err == nil {
			ki.UpdatedAt = t
		}
		issues = append(issues, ki)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate known issue rows: %w", err)
	}
	return issues, nil
}
