package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// RefreshTokenFamilyStore provides refresh-token family storage backed by PostgreSQL.
// It supports the OAuth 2.0 BCP refresh-token rotation flow with reuse detection.
type RefreshTokenFamilyStore struct {
	pool *pgxpool.Pool
}

// NewRefreshTokenFamilyStore creates a RefreshTokenFamilyStore backed by the given PGStore.
func NewRefreshTokenFamilyStore(s *PGStore) *RefreshTokenFamilyStore {
	return &RefreshTokenFamilyStore{pool: s.pool}
}

var _ store.RefreshTokenFamilyStorer = (*RefreshTokenFamilyStore)(nil)

// Create inserts a new refresh-token family row. The caller is responsible for
// populating FamilyID with a UUID string; an empty FamilyID is rejected because
// the schema uses UUID PRIMARY KEY without a default and we avoid adding a new
// UUID dependency to the Go module. Status defaults to 'active' when empty.
func (r *RefreshTokenFamilyStore) Create(ctx context.Context, family store.RefreshTokenFamily) error {
	if family.FamilyID == "" {
		return fmt.Errorf("create refresh token family: family_id is required")
	}
	if family.Status == "" {
		family.Status = store.RefreshTokenFamilyStatusActive
	}
	if family.Provider == "" {
		family.Provider = "local"
	}

	_, err := r.pool.Exec(ctx, `
		INSERT INTO refresh_token_families (
			family_id, user_id, role, provider,
			current_jti, previous_jti, grace_until,
			status, expires_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		family.FamilyID,
		family.UserID,
		family.Role,
		family.Provider,
		family.CurrentJTI,
		family.PreviousJTI,
		family.GraceUntil,
		family.Status,
		family.ExpiresAt.UTC(),
	)
	if err != nil {
		return fmt.Errorf("create refresh token family: %w", err)
	}
	return nil
}

// GetByID retrieves a single refresh-token family row by its UUID. It returns
// (nil, nil) when no matching row exists so callers can distinguish not-found
// from database errors — matching the convention used by BlacklistStore.
func (r *RefreshTokenFamilyStore) GetByID(ctx context.Context, familyID string) (*store.RefreshTokenFamily, error) {
	var f store.RefreshTokenFamily
	err := r.pool.QueryRow(ctx, `
		SELECT family_id, user_id, role, provider,
		       current_jti, previous_jti, grace_until,
		       status, created_at, updated_at, expires_at
		FROM refresh_token_families
		WHERE family_id = $1`, familyID,
	).Scan(
		&f.FamilyID,
		&f.UserID,
		&f.Role,
		&f.Provider,
		&f.CurrentJTI,
		&f.PreviousJTI,
		&f.GraceUntil,
		&f.Status,
		&f.CreatedAt,
		&f.UpdatedAt,
		&f.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get refresh token family: %w", err)
	}
	return &f, nil
}

// Rotate atomically moves the current refresh JTI into previous_jti, installs
// the new JTI, opens a grace window, and bumps updated_at. It performs the work
// in a single UPDATE statement so no SELECT-then-UPDATE race is possible.
// Passing graceSeconds <= 0 collapses the grace window to NOW().
func (r *RefreshTokenFamilyStore) Rotate(ctx context.Context, familyID, newJTI string, graceSeconds int) error {
	if graceSeconds < 0 {
		graceSeconds = 0
	}
	tag, err := r.pool.Exec(ctx, `
		UPDATE refresh_token_families
		SET previous_jti = current_jti,
		    current_jti  = $2,
		    grace_until  = NOW() + make_interval(secs => $3),
		    updated_at   = NOW()
		WHERE family_id = $1`,
		familyID, newJTI, graceSeconds,
	)
	if err != nil {
		return fmt.Errorf("rotate refresh token family: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: family_id=%s", store.ErrRefreshFamilyNotFound, familyID)
	}
	return nil
}

// MarkCompromised transitions the family to the 'compromised' status. Callers
// should invoke this when a rotated refresh JTI is presented outside the grace
// window (token theft detection).
func (r *RefreshTokenFamilyStore) MarkCompromised(ctx context.Context, familyID string) error {
	return r.setStatus(ctx, familyID, store.RefreshTokenFamilyStatusCompromised)
}

// Revoke transitions the family to the 'revoked' status. Use for logout or
// explicit session termination.
func (r *RefreshTokenFamilyStore) Revoke(ctx context.Context, familyID string) error {
	return r.setStatus(ctx, familyID, store.RefreshTokenFamilyStatusRevoked)
}

// setStatus is the shared implementation for status transitions.
func (r *RefreshTokenFamilyStore) setStatus(ctx context.Context, familyID, status string) error {
	tag, err := r.pool.Exec(ctx, `
		UPDATE refresh_token_families
		SET status = $2, updated_at = NOW()
		WHERE family_id = $1`,
		familyID, status,
	)
	if err != nil {
		return fmt.Errorf("set refresh token family status: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: family_id=%s", store.ErrRefreshFamilyNotFound, familyID)
	}
	return nil
}

// DeleteExpired removes every row whose expires_at is strictly before NOW().
// It returns the number of rows deleted.
func (r *RefreshTokenFamilyStore) DeleteExpired(ctx context.Context) (int, error) {
	tag, err := r.pool.Exec(ctx, `
		DELETE FROM refresh_token_families
		WHERE expires_at < $1`, time.Now().UTC(),
	)
	if err != nil {
		return 0, fmt.Errorf("delete expired refresh token families: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
