package pg

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// UserStore provides user storage backed by PostgreSQL.
type UserStore struct {
	pool *pgxpool.Pool
}

// NewUserStore creates a UserStore backed by the given PGStore.
func NewUserStore(s *PGStore) *UserStore {
	return &UserStore{pool: s.pool}
}

var _ store.UserStorer = (*UserStore)(nil)

// userColumns is the canonical projection for user SELECT statements.
const userColumns = "id, email, name, provider, provider_sub, password_hash, role, is_active, last_login, created_at, updated_at"

func scanUser(row pgx.Row, u *store.User) error {
	return row.Scan(&u.ID, &u.Email, &u.Name, &u.Provider, &u.ProviderSub, &u.PasswordHash,
		&u.Role, &u.IsActive, &u.LastLogin, &u.CreatedAt, &u.UpdatedAt)
}

// UpsertByOIDC inserts a new user or updates email, name, role, and last_login on conflict.
// The conflict target is (provider, provider_sub) filtered by provider_sub != ”.
func (s *UserStore) UpsertByOIDC(ctx context.Context, provider, sub, email, name, role string) (*store.User, error) {
	var u store.User
	err := scanUser(s.pool.QueryRow(ctx, `
		INSERT INTO users (email, name, provider, provider_sub, role, last_login, updated_at)
		VALUES ($1, $2, $3, $4, $5, NOW(), NOW())
		ON CONFLICT (provider, provider_sub) WHERE provider_sub != ''
		DO UPDATE SET
			email      = EXCLUDED.email,
			name       = EXCLUDED.name,
			role       = EXCLUDED.role,
			last_login = NOW(),
			updated_at = NOW()
		RETURNING `+userColumns,
		email, name, provider, sub, role,
	), &u)
	if err != nil {
		return nil, fmt.Errorf("upsert user by oidc: %w", err)
	}
	return &u, nil
}

// CreateLocal inserts a new local (password-based) user. Returns
// store.ErrDuplicateEntry when the partial unique index on LOWER(email) for
// active rows rejects the insert.
func (s *UserStore) CreateLocal(ctx context.Context, email, name, passwordHash, role string) (*store.User, error) {
	var u store.User
	err := scanUser(s.pool.QueryRow(ctx, `
		INSERT INTO users (email, name, provider, provider_sub, password_hash, role, is_active, created_at, updated_at)
		VALUES ($1, $2, 'local', '', $3, $4, TRUE, NOW(), NOW())
		RETURNING `+userColumns,
		email, name, passwordHash, role,
	), &u)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("%w: email=%s", store.ErrDuplicateEntry, email)
		}
		return nil, fmt.Errorf("create local user: %w", err)
	}
	return &u, nil
}

// GetByID retrieves a user by primary key.
func (s *UserStore) GetByID(ctx context.Context, id int64) (*store.User, error) {
	var u store.User
	err := scanUser(s.pool.QueryRow(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id), &u)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%d", store.ErrUserNotFound, id)
	}
	if err != nil {
		return nil, fmt.Errorf("get user by id: %w", err)
	}
	return &u, nil
}

// GetByEmail retrieves a user by email address (case-insensitive).
func (s *UserStore) GetByEmail(ctx context.Context, email string) (*store.User, error) {
	var u store.User
	err := scanUser(s.pool.QueryRow(ctx,
		`SELECT `+userColumns+` FROM users WHERE LOWER(email) = LOWER($1) ORDER BY is_active DESC, id ASC LIMIT 1`,
		email), &u)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: email=%s", store.ErrUserNotFound, email)
	}
	if err != nil {
		return nil, fmt.Errorf("get user by email: %w", err)
	}
	return &u, nil
}

// List returns all users ordered by created_at descending.
// Retained for back-compat; new code should prefer ListPaginated.
func (s *UserStore) List(ctx context.Context) ([]store.User, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+userColumns+` FROM users ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list users: %w", err)
	}
	defer rows.Close()

	var users []store.User
	for rows.Next() {
		var u store.User
		if err := scanUser(rows, &u); err != nil {
			return nil, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user rows: %w", err)
	}
	return users, nil
}

// ListPaginated returns a filtered page of users and the total count matching the filters.
func (s *UserStore) ListPaginated(ctx context.Context, params store.ListUsersParams) ([]store.User, int, error) {
	var (
		where    []string
		args     []any
		argIndex = 1
	)

	if search := strings.TrimSpace(params.Search); search != "" {
		where = append(where, fmt.Sprintf("(email ILIKE '%%' || $%d || '%%' OR name ILIKE '%%' || $%d || '%%')", argIndex, argIndex))
		args = append(args, search)
		argIndex++
	}
	if role := strings.TrimSpace(params.Role); role != "" {
		where = append(where, fmt.Sprintf("role = $%d", argIndex))
		args = append(args, role)
		argIndex++
	}
	if params.Active != nil {
		where = append(where, fmt.Sprintf("is_active = $%d", argIndex))
		args = append(args, *params.Active)
		argIndex++
	}

	whereSQL := ""
	if len(where) > 0 {
		whereSQL = " WHERE " + strings.Join(where, " AND ")
	}

	// COUNT
	var total int
	if err := s.pool.QueryRow(ctx, `SELECT COUNT(*) FROM users`+whereSQL, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	// SELECT
	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := max(params.Offset, 0)
	selectArgs := append([]any{}, args...)
	selectArgs = append(selectArgs, limit, offset)
	query := `SELECT ` + userColumns + ` FROM users` + whereSQL +
		fmt.Sprintf(" ORDER BY email ASC LIMIT $%d OFFSET $%d", argIndex, argIndex+1)

	rows, err := s.pool.Query(ctx, query, selectArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list users paginated: %w", err)
	}
	defer rows.Close()

	users := make([]store.User, 0, limit)
	for rows.Next() {
		var u store.User
		if err := scanUser(rows, &u); err != nil {
			return nil, 0, fmt.Errorf("scan user: %w", err)
		}
		users = append(users, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate user rows: %w", err)
	}
	return users, total, nil
}

// UpdateRole changes a user's role.
func (s *UserStore) UpdateRole(ctx context.Context, id int64, role string) error {
	tag, err := s.pool.Exec(ctx,
		"UPDATE users SET role = $1, updated_at = NOW() WHERE id = $2", role, id)
	if err != nil {
		return fmt.Errorf("update user role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%d", store.ErrUserNotFound, id)
	}
	return nil
}

// UpdateActive toggles a user's is_active flag.
func (s *UserStore) UpdateActive(ctx context.Context, id int64, active bool) error {
	tag, err := s.pool.Exec(ctx,
		"UPDATE users SET is_active = $1, updated_at = NOW() WHERE id = $2", active, id)
	if err != nil {
		return fmt.Errorf("update user active: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%d", store.ErrUserNotFound, id)
	}
	return nil
}

// UpdateProfile updates a user's display name.
func (s *UserStore) UpdateProfile(ctx context.Context, id int64, name string) error {
	tag, err := s.pool.Exec(ctx,
		"UPDATE users SET name = $1, updated_at = NOW() WHERE id = $2", name, id)
	if err != nil {
		return fmt.Errorf("update user profile: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%d", store.ErrUserNotFound, id)
	}
	return nil
}

// UpdatePasswordHash replaces the user's password_hash and bumps updated_at.
// Returns ErrUserNotFound when no row matches the supplied id.
func (s *UserStore) UpdatePasswordHash(ctx context.Context, id int64, passwordHash string) error {
	tag, err := s.pool.Exec(ctx,
		"UPDATE users SET password_hash = $1, updated_at = NOW() WHERE id = $2", passwordHash, id)
	if err != nil {
		return fmt.Errorf("update user password hash: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%d", store.ErrUserNotFound, id)
	}
	return nil
}

// UpdateLastLogin refreshes last_login and updated_at to NOW() for the given user.
// Returns ErrUserNotFound when no row matches the supplied id.
func (s *UserStore) UpdateLastLogin(ctx context.Context, id int64) error {
	tag, err := s.pool.Exec(ctx,
		"UPDATE users SET last_login = NOW(), updated_at = NOW() WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("update user last_login: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%d", store.ErrUserNotFound, id)
	}
	return nil
}

// Deactivate sets is_active=false for the given user ID.
func (s *UserStore) Deactivate(ctx context.Context, id int64) error {
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
