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

// PGBlacklistStore provides persistent JWT revocation storage backed by PostgreSQL.
type PGBlacklistStore struct {
	pool *pgxpool.Pool
}

// NewBlacklistStore creates a PGBlacklistStore backed by the given PGStore.
func NewBlacklistStore(s *PGStore) *PGBlacklistStore {
	return &PGBlacklistStore{pool: s.pool}
}

// AddToBlacklist records a revoked JWT JTI with its expiry time.
func (bl *PGBlacklistStore) AddToBlacklist(ctx context.Context, jti string, expiresAt time.Time) error {
	_, err := bl.pool.Exec(ctx,
		"INSERT INTO jwt_blacklist(jti, expires_at) VALUES($1,$2) ON CONFLICT (jti) DO UPDATE SET expires_at = EXCLUDED.expires_at",
		jti, expiresAt.UTC())
	if err != nil {
		return fmt.Errorf("add to blacklist: %w", err)
	}
	return nil
}

// IsBlacklisted returns true if the JTI is present and has not yet expired.
func (bl *PGBlacklistStore) IsBlacklisted(ctx context.Context, jti string) (bool, error) {
	var expiry time.Time
	err := bl.pool.QueryRow(ctx,
		"SELECT expires_at FROM jwt_blacklist WHERE jti = $1", jti,
	).Scan(&expiry)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("is blacklisted: %w", err)
	}
	return time.Now().Before(expiry), nil
}

// PruneExpired removes all expired entries. Returns the number of rows deleted.
func (bl *PGBlacklistStore) PruneExpired(ctx context.Context) (int64, error) {
	tag, err := bl.pool.Exec(ctx,
		"DELETE FROM jwt_blacklist WHERE expires_at <= $1", time.Now().UTC())
	if err != nil {
		return 0, fmt.Errorf("prune expired blacklist: %w", err)
	}
	return tag.RowsAffected(), nil
}

var _ store.BlacklistStorer = (*PGBlacklistStore)(nil)
