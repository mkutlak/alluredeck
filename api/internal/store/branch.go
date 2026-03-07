package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrBranchNotFound is returned when a branch does not exist.
var ErrBranchNotFound = errors.New("branch not found")

// ErrCannotDeleteDefaultBranch is returned when attempting to delete the default branch.
var ErrCannotDeleteDefaultBranch = errors.New("cannot delete default branch")

// Branch holds branch metadata from the database.
type Branch struct {
	ID        int64
	ProjectID string
	Name      string
	IsDefault bool
	CreatedAt time.Time
}

// BranchStore provides operations on the branches table.
type BranchStore struct {
	db *sql.DB
}

// NewBranchStore creates a BranchStore backed by the given SQLiteStore.
func NewBranchStore(s *SQLiteStore) *BranchStore {
	return &BranchStore{db: s.db}
}

// GetOrCreate upserts a branch for the given project+name.
// Returns the branch and whether it was newly created.
// The first branch created for a project is automatically set as default.
func (bs *BranchStore) GetOrCreate(ctx context.Context, projectID, name string) (*Branch, bool, error) {
	// Try to get existing branch first.
	b, err := bs.GetByName(ctx, projectID, name)
	if err == nil {
		return b, false, nil
	}
	if !errors.Is(err, ErrBranchNotFound) {
		return nil, false, err
	}

	// Does the project have any branches yet? If not, this one becomes default.
	var count int
	if err := bs.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM branches WHERE project_id = ?", projectID,
	).Scan(&count); err != nil {
		return nil, false, fmt.Errorf("count branches: %w", err)
	}
	isDefault := 0
	if count == 0 {
		isDefault = 1
	}

	res, err := bs.db.ExecContext(ctx,
		"INSERT INTO branches(project_id, name, is_default) VALUES(?, ?, ?)",
		projectID, name, isDefault)
	if err != nil {
		return nil, false, fmt.Errorf("insert branch: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, false, fmt.Errorf("last insert id: %w", err)
	}

	b, err = bs.getByID(ctx, id)
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

// List returns all branches for a project ordered by name.
func (bs *BranchStore) List(ctx context.Context, projectID string) ([]Branch, error) {
	rows, err := bs.db.QueryContext(ctx,
		"SELECT id, project_id, name, is_default, created_at FROM branches WHERE project_id = ? ORDER BY name ASC",
		projectID)
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var branches []Branch
	for rows.Next() {
		b, err := scanBranchRow(rows)
		if err != nil {
			return nil, fmt.Errorf("scan branch: %w", err)
		}
		branches = append(branches, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate branch rows: %w", err)
	}
	if branches == nil {
		branches = []Branch{}
	}
	return branches, nil
}

// GetDefault returns the default branch for a project.
func (bs *BranchStore) GetDefault(ctx context.Context, projectID string) (*Branch, error) {
	row := bs.db.QueryRowContext(ctx,
		"SELECT id, project_id, name, is_default, created_at FROM branches WHERE project_id = ? AND is_default = 1",
		projectID)
	b, err := scanBranchSingleRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: no default branch for project=%s", ErrBranchNotFound, projectID)
		}
		return nil, fmt.Errorf("get default branch: %w", err)
	}
	return &b, nil
}

// SetDefault sets a branch as the default for its project (in a transaction).
// Clears is_default from all other branches in the project first.
func (bs *BranchStore) SetDefault(ctx context.Context, projectID string, branchID int64) error {
	tx, err := bs.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Verify branch belongs to project.
	var count int
	if err := tx.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM branches WHERE id = ? AND project_id = ?", branchID, projectID,
	).Scan(&count); err != nil {
		return fmt.Errorf("verify branch: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("%w: branch=%d project=%s", ErrBranchNotFound, branchID, projectID)
	}

	if _, err := tx.ExecContext(ctx,
		"UPDATE branches SET is_default = 0 WHERE project_id = ?", projectID); err != nil {
		return fmt.Errorf("clear defaults: %w", err)
	}
	if _, err := tx.ExecContext(ctx,
		"UPDATE branches SET is_default = 1 WHERE id = ?", branchID); err != nil {
		return fmt.Errorf("set default: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit set default: %w", err)
	}
	return nil
}

// Delete deletes a non-default branch by ID. Returns ErrCannotDeleteDefaultBranch
// if the branch is the current default.
func (bs *BranchStore) Delete(ctx context.Context, projectID string, branchID int64) error {
	var isDefault int
	err := bs.db.QueryRowContext(ctx,
		"SELECT is_default FROM branches WHERE id = ? AND project_id = ?", branchID, projectID,
	).Scan(&isDefault)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: branch=%d project=%s", ErrBranchNotFound, branchID, projectID)
		}
		return fmt.Errorf("get branch for delete: %w", err)
	}
	if isDefault == 1 {
		return ErrCannotDeleteDefaultBranch
	}

	if _, err := bs.db.ExecContext(ctx,
		"DELETE FROM branches WHERE id = ? AND project_id = ?", branchID, projectID); err != nil {
		return fmt.Errorf("delete branch: %w", err)
	}
	return nil
}

// GetByName returns a branch by project and name.
func (bs *BranchStore) GetByName(ctx context.Context, projectID, name string) (*Branch, error) {
	row := bs.db.QueryRowContext(ctx,
		"SELECT id, project_id, name, is_default, created_at FROM branches WHERE project_id = ? AND name = ?",
		projectID, name)
	b, err := scanBranchSingleRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: branch=%q project=%s", ErrBranchNotFound, name, projectID)
		}
		return nil, fmt.Errorf("get branch by name: %w", err)
	}
	return &b, nil
}

// getByID returns a branch by its primary key.
func (bs *BranchStore) getByID(ctx context.Context, id int64) (*Branch, error) {
	row := bs.db.QueryRowContext(ctx,
		"SELECT id, project_id, name, is_default, created_at FROM branches WHERE id = ?", id)
	b, err := scanBranchSingleRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("%w: id=%d", ErrBranchNotFound, id)
		}
		return nil, fmt.Errorf("get branch by id: %w", err)
	}
	return &b, nil
}

// scanBranchRow scans a Branch from a *sql.Rows cursor.
func scanBranchRow(rows *sql.Rows) (Branch, error) {
	var b Branch
	var createdAt string
	var isDefault int
	if err := rows.Scan(&b.ID, &b.ProjectID, &b.Name, &isDefault, &createdAt); err != nil {
		return Branch{}, err
	}
	b.IsDefault = isDefault == 1
	if t, err := time.Parse("2006-01-02T15:04:05Z", createdAt); err == nil {
		b.CreatedAt = t
	}
	return b, nil
}

// scanBranchSingleRow scans a Branch from a *sql.Row.
func scanBranchSingleRow(row *sql.Row) (Branch, error) {
	var b Branch
	var createdAt string
	var isDefault int
	if err := row.Scan(&b.ID, &b.ProjectID, &b.Name, &isDefault, &createdAt); err != nil {
		return Branch{}, err
	}
	b.IsDefault = isDefault == 1
	if t, err := time.Parse("2006-01-02T15:04:05Z", createdAt); err == nil {
		b.CreatedAt = t
	}
	return b, nil
}
