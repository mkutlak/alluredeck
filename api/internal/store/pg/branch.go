package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// BranchStore provides operations on the branches table backed by PostgreSQL.
type BranchStore struct {
	pool *pgxpool.Pool
}

// NewBranchStore creates a BranchStore backed by the given PGStore.
func NewBranchStore(s *PGStore) *BranchStore {
	return &BranchStore{pool: s.pool}
}

// GetOrCreate upserts a branch for the given project+name.
// Returns the branch and whether it was newly created.
// The first branch created for a project is automatically set as default.
func (bs *BranchStore) GetOrCreate(ctx context.Context, projectID, name string) (*store.Branch, bool, error) {
	// Try to get existing branch first.
	b, err := bs.GetByName(ctx, projectID, name)
	if err == nil {
		return b, false, nil
	}
	if !errors.Is(err, store.ErrBranchNotFound) {
		return nil, false, err
	}

	// Does the project have any branches yet? If not, this one becomes default.
	var count int
	if err := bs.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM branches WHERE project_id = $1", projectID,
	).Scan(&count); err != nil {
		return nil, false, fmt.Errorf("count branches: %w", err)
	}
	isDefault := count == 0

	var id int64
	if err := bs.pool.QueryRow(ctx,
		"INSERT INTO branches(project_id, name, is_default) VALUES($1,$2,$3) RETURNING id",
		projectID, name, isDefault,
	).Scan(&id); err != nil {
		return nil, false, fmt.Errorf("insert branch: %w", err)
	}

	b, err = bs.getByID(ctx, id)
	if err != nil {
		return nil, false, err
	}
	return b, true, nil
}

// List returns all branches for a project ordered by name.
func (bs *BranchStore) List(ctx context.Context, projectID string) ([]store.Branch, error) {
	rows, err := bs.pool.Query(ctx,
		"SELECT id, project_id, name, is_default, created_at FROM branches WHERE project_id = $1 ORDER BY name ASC",
		projectID)
	if err != nil {
		return nil, fmt.Errorf("list branches: %w", err)
	}
	defer rows.Close()

	var branches []store.Branch
	for rows.Next() {
		b, err := scanBranchRows(rows)
		if err != nil {
			return nil, fmt.Errorf("scan branch: %w", err)
		}
		branches = append(branches, b)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate branch rows: %w", err)
	}
	if branches == nil {
		branches = []store.Branch{}
	}
	return branches, nil
}

// GetDefault returns the default branch for a project.
func (bs *BranchStore) GetDefault(ctx context.Context, projectID string) (*store.Branch, error) {
	rows, err := bs.pool.Query(ctx,
		"SELECT id, project_id, name, is_default, created_at FROM branches WHERE project_id = $1 AND is_default = TRUE",
		projectID)
	if err != nil {
		return nil, fmt.Errorf("get default branch: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("get default branch: %w", err)
		}
		return nil, fmt.Errorf("%w: no default branch for project=%s", store.ErrBranchNotFound, projectID)
	}
	b, err := scanBranchRows(rows)
	if err != nil {
		return nil, fmt.Errorf("get default branch: %w", err)
	}
	return &b, nil
}

// SetDefault sets a branch as the default for its project (in a transaction).
// Clears is_default from all other branches in the project first.
func (bs *BranchStore) SetDefault(ctx context.Context, projectID string, branchID int64) error {
	tx, err := bs.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// Verify branch belongs to project.
	var count int
	if err := tx.QueryRow(ctx,
		"SELECT COUNT(*) FROM branches WHERE id = $1 AND project_id = $2", branchID, projectID,
	).Scan(&count); err != nil {
		return fmt.Errorf("verify branch: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("%w: branch=%d project=%s", store.ErrBranchNotFound, branchID, projectID)
	}

	if _, err := tx.Exec(ctx,
		"UPDATE branches SET is_default = FALSE WHERE project_id = $1", projectID); err != nil {
		return fmt.Errorf("clear defaults: %w", err)
	}
	if _, err := tx.Exec(ctx,
		"UPDATE branches SET is_default = TRUE WHERE id = $1", branchID); err != nil {
		return fmt.Errorf("set default: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit set default: %w", err)
	}
	return nil
}

// Delete deletes a non-default branch by ID. Returns ErrCannotDeleteDefaultBranch
// if the branch is the current default.
func (bs *BranchStore) Delete(ctx context.Context, projectID string, branchID int64) error {
	var isDefault bool
	err := bs.pool.QueryRow(ctx,
		"SELECT is_default FROM branches WHERE id = $1 AND project_id = $2", branchID, projectID,
	).Scan(&isDefault)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("%w: branch=%d project=%s", store.ErrBranchNotFound, branchID, projectID)
	}
	if err != nil {
		return fmt.Errorf("get branch for delete: %w", err)
	}
	if isDefault {
		return store.ErrCannotDeleteDefaultBranch
	}

	if _, err := bs.pool.Exec(ctx,
		"DELETE FROM branches WHERE id = $1 AND project_id = $2", branchID, projectID); err != nil {
		return fmt.Errorf("delete branch: %w", err)
	}
	return nil
}

// GetByName returns a branch by project and name.
func (bs *BranchStore) GetByName(ctx context.Context, projectID, name string) (*store.Branch, error) {
	rows, err := bs.pool.Query(ctx,
		"SELECT id, project_id, name, is_default, created_at FROM branches WHERE project_id = $1 AND name = $2",
		projectID, name)
	if err != nil {
		return nil, fmt.Errorf("get branch by name: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("get branch by name: %w", err)
		}
		return nil, fmt.Errorf("%w: branch=%q project=%s", store.ErrBranchNotFound, name, projectID)
	}
	b, err := scanBranchRows(rows)
	if err != nil {
		return nil, fmt.Errorf("get branch by name: %w", err)
	}
	return &b, nil
}

// getByID returns a branch by its primary key.
func (bs *BranchStore) getByID(ctx context.Context, id int64) (*store.Branch, error) {
	rows, err := bs.pool.Query(ctx,
		"SELECT id, project_id, name, is_default, created_at FROM branches WHERE id = $1", id)
	if err != nil {
		return nil, fmt.Errorf("get branch by id: %w", err)
	}
	defer rows.Close()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("get branch by id: %w", err)
		}
		return nil, fmt.Errorf("%w: id=%d", store.ErrBranchNotFound, id)
	}
	b, err := scanBranchRows(rows)
	if err != nil {
		return nil, fmt.Errorf("get branch by id: %w", err)
	}
	return &b, nil
}

// scanBranchRows scans a Branch from a pgx.Rows cursor.
func scanBranchRows(rows pgx.Rows) (store.Branch, error) {
	var b store.Branch
	if err := rows.Scan(&b.ID, &b.ProjectID, &b.Name, &b.IsDefault, &b.CreatedAt); err != nil {
		return store.Branch{}, err
	}
	return b, nil
}

var _ store.BranchStorer = (*BranchStore)(nil)
