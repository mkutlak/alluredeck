package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// PGUserStore provides user storage backed by PostgreSQL.
type PGUserStore struct {
	pool *pgxpool.Pool
}

// NewUserStore creates a PGUserStore backed by the given PGStore.
func NewUserStore(s *PGStore) *PGUserStore {
	return &PGUserStore{pool: s.pool}
}

var _ store.UserStorer = (*PGUserStore)(nil)

// UpsertByOIDC inserts a new user or updates email, name, role, and last_login on conflict.
// The conflict target is (provider, provider_sub) filtered by provider_sub != ”.
func (s *PGUserStore) UpsertByOIDC(ctx context.Context, provider, sub, email, name, role string) (*store.User, error) {
	var u store.User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (email, name, provider, provider_sub, role, last_login, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (provider, provider_sub) WHERE provider_sub != ''
		DO UPDATE SET
			email      = EXCLUDED.email,
			name       = EXCLUDED.name,
			role       = EXCLUDED.role,
			last_login = NOW(),
			updated_at = NOW()
		RETURNING id, email, name, provider, provider_sub, role, is_active, last_login, created_at, updated_at`,
		email, name, provider, sub, role,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Provider, &u.ProviderSub,
		&u.Role, &u.IsActive, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("upsert user by oidc: %w", err)
	}
	return &u, nil
}

// GetByID retrieves a user by primary key.
func (s *PGUserStore) GetByID(ctx context.Context, id int64) (*store.User, error) {
	var u store.User
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, name, provider, provider_sub, role, is_active, last_login, created_at, updated_at
		FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Provider, &u.ProviderSub,
		&u.Role, &u.IsActive, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%d", store.ErrUserNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

// GetByEmail retrieves a user by email address.
func (s *PGUserStore) GetByEmail(ctx context.Context, email string) (*store.User, error) {
	var u store.User
	err := s.pool.QueryRow(ctx, `
		SELECT id, email, name, provider, provider_sub, role, is_active, last_login, created_at, updated_at
		FROM users WHERE email = $1`, email,
	).Scan(&u.ID, &u.Email, &u.Name, &u.Provider, &u.ProviderSub,
		&u.Role, &u.IsActive, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: email=%s", store.ErrUserNotFound, email)
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &u, nil
}

// List returns all users ordered by created_at descending.
func (s *PGUserStore) List(ctx context.Context) ([]store.User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, email, name, provider, provider_sub, role, is_active, last_login, created_at, updated_at
		FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []store.User
	for rows.Next() {
		var u store.User
		if err := rows.Scan(&u.ID, &u.Email, &u.Name, &u.Provider, &u.ProviderSub,
			&u.Role, &u.IsActive, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user rows: %w", err)
	}
	return users, nil
}

// Deactivate sets is_active=false for the given user ID.
func (s *PGUserStore) Deactivate(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx,
		"UPDATE users SET is_active = FALSE, updated_at = NOW() WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("deactivate user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%d", store.ErrUserNotFound, id)
	}
	return nil
}
