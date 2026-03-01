package testutil

import (
	"context"
	"sync"
	"time"
)

// MemBlacklist is an in-memory implementation of security.BlacklistStore for tests.
// It does not import the security package to avoid import cycles when used in
// security_test.go; Go's structural typing satisfies the interface automatically.
type MemBlacklist struct {
	mu      sync.RWMutex
	entries map[string]time.Time
}

// NewMemBlacklist returns a ready-to-use MemBlacklist.
func NewMemBlacklist() *MemBlacklist {
	return &MemBlacklist{entries: make(map[string]time.Time)}
}

func (m *MemBlacklist) AddToBlacklist(_ context.Context, jti string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[jti] = expiresAt
	return nil
}

func (m *MemBlacklist) IsBlacklisted(_ context.Context, jti string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	exp, ok := m.entries[jti]
	if !ok {
		return false, nil
	}
	return time.Now().Before(exp), nil
}

func (m *MemBlacklist) PruneExpired(_ context.Context) (int64, error) { return 0, nil }
