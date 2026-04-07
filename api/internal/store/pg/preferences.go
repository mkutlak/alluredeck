package pg

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// PreferenceStore provides user preferences storage backed by PostgreSQL.
type PreferenceStore struct {
	pool *pgxpool.Pool
}

// NewPreferenceStore creates a PreferenceStore backed by the given PGStore.
func NewPreferenceStore(s *PGStore) *PreferenceStore {
	return &PreferenceStore{pool: s.pool}
}

var _ store.PreferenceStorer = (*PreferenceStore)(nil)

// GetPreferences returns the preferences for a user, or ErrPreferencesNotFound if none exist.
func (s *PreferenceStore) GetPreferences(ctx context.Context, username string) (*store.UserPreferences, error) {
	var p store.UserPreferences
	err := s.pool.QueryRow(ctx,
		`SELECT username, preferences, updated_at FROM user_preferences WHERE username = $1`,
		username,
	).Scan(&p.Username, &p.Preferences, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, store.ErrPreferencesNotFound
		}
		return nil, err
	}
	return &p, nil
}

// UpsertPreferences inserts or updates preferences for a user and returns the result.
func (s *PreferenceStore) UpsertPreferences(ctx context.Context, username string, preferences json.RawMessage) (*store.UserPreferences, error) {
	var p store.UserPreferences
	err := s.pool.QueryRow(ctx, `
		INSERT INTO user_preferences (username, preferences, updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (username) DO UPDATE SET
			preferences = EXCLUDED.preferences,
			updated_at  = NOW()
		RETURNING username, preferences, updated_at`,
		username, preferences,
	).Scan(&p.Username, &p.Preferences, &p.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &p, nil
}
