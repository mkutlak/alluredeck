package pg

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// APIKeyStore provides API key storage backed by PostgreSQL.
type APIKeyStore struct {
	pool *pgxpool.Pool
}

// NewAPIKeyStore creates a APIKeyStore backed by the given PGStore.
func NewAPIKeyStore(s *PGStore) *APIKeyStore {
	return &APIKeyStore{pool: s.pool}
}

var _ store.APIKeyStorer = (*APIKeyStore)(nil)

// Create inserts a new API key record and returns it with ID and created_at populated.
func (s *APIKeyStore) Create(ctx context.Context, key *store.APIKey) (*store.APIKey, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO api_keys (name, prefix, key_hash, username, role, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`,
		key.Name, key.Prefix, key.KeyHash, key.Username, key.Role, key.ExpiresAt,
	).Scan(&key.ID, &key.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create api key: %w", err)
	}
	return key, nil
}

// ListByUsername returns all API keys for the given username, newest first.
func (s *APIKeyStore) ListByUsername(ctx context.Context, username string) ([]store.APIKey, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, prefix, key_hash, username, role, expires_at, last_used, created_at
		FROM api_keys
		WHERE username = $1
		ORDER BY created_at DESC`,
		username,
	)
	if err != nil {
		return nil, fmt.Errorf("list api keys: %w", err)
	}
	defer rows.Close()

	var keys []store.APIKey
	for rows.Next() {
		var k store.APIKey
		if err := rows.Scan(&k.ID, &k.Name, &k.Prefix, &k.KeyHash, &k.Username,
			&k.Role, &k.ExpiresAt, &k.LastUsed, &k.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan api key: %w", err)
		}
		keys = append(keys, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate api key rows: %w", err)
	}
	return keys, nil
}

// GetByHash looks up an API key by its SHA-256 hash.
// Returns ErrAPIKeyNotFound when no matching key exists.
func (s *APIKeyStore) GetByHash(ctx context.Context, keyHash string) (*store.APIKey, error) {
	var k store.APIKey
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, prefix, key_hash, username, role, expires_at, last_used, created_at
		FROM api_keys
		WHERE key_hash = $1`,
		keyHash,
	).Scan(&k.ID, &k.Name, &k.Prefix, &k.KeyHash, &k.Username,
		&k.Role, &k.ExpiresAt, &k.LastUsed, &k.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: hash=%s", store.ErrAPIKeyNotFound, keyHash)
	}
	if err != nil {
		return nil, fmt.Errorf("get api key by hash: %w", err)
	}
	return &k, nil
}

// UpdateLastUsed sets last_used to the current time for the given key ID.
func (s *APIKeyStore) UpdateLastUsed(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx,
		"UPDATE api_keys SET last_used = NOW() WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("update api key last_used: %w", err)
	}
	return nil
}

// Delete removes an API key by ID scoped to the given username (IDOR prevention).
// Returns ErrAPIKeyNotFound if no row was deleted.
func (s *APIKeyStore) Delete(ctx context.Context, id int64, username string) error {
	tag, err := s.pool.Exec(ctx,
		"DELETE FROM api_keys WHERE id = $1 AND username = $2", id, username)
	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%d", store.ErrAPIKeyNotFound, id)
	}
	return nil
}

// CountByUsername returns the number of API keys owned by the given username.
func (s *APIKeyStore) CountByUsername(ctx context.Context, username string) (int, error) {
	var count int
	err := s.pool.QueryRow(ctx,
		"SELECT COUNT(*) FROM api_keys WHERE username = $1", username,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count api keys: %w", err)
	}
	return count, nil
}
