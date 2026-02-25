package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// BlacklistStore provides persistent JWT revocation storage.
type BlacklistStore struct {
	db *sql.DB
}

// NewBlacklistStore creates a BlacklistStore backed by the given SQLiteStore.
func NewBlacklistStore(s *SQLiteStore) *BlacklistStore {
	return &BlacklistStore{db: s.db}
}

// AddToBlacklist records a revoked JWT JTI with its expiry time.
func (bl *BlacklistStore) AddToBlacklist(ctx context.Context, jti string, expiresAt time.Time) error {
	_, err := bl.db.ExecContext(ctx,
		"INSERT OR REPLACE INTO jwt_blacklist(jti, expires_at) VALUES(?, ?)",
		jti, expiresAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("add to blacklist: %w", err)
	}
	return nil
}

// IsBlacklisted returns true if the JTI is present and has not yet expired.
func (bl *BlacklistStore) IsBlacklisted(ctx context.Context, jti string) (bool, error) {
	var expiresAt string
	err := bl.db.QueryRowContext(ctx,
		"SELECT expires_at FROM jwt_blacklist WHERE jti = ?", jti,
	).Scan(&expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("is blacklisted: %w", err)
	}

	expiry, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return false, fmt.Errorf("parse expires_at: %w", err)
	}
	return time.Now().Before(expiry), nil
}

// PruneExpired removes all expired entries. Returns the number of rows deleted.
func (bl *BlacklistStore) PruneExpired(ctx context.Context) (int64, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := bl.db.ExecContext(ctx,
		"DELETE FROM jwt_blacklist WHERE expires_at <= ?", now,
	)
	if err != nil {
		return 0, fmt.Errorf("prune expired blacklist: %w", err)
	}
	n, _ := res.RowsAffected()
	return n, nil
}
