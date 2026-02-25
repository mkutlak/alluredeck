package security

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// memBlacklist is an in-memory BlacklistStore for tests.
type memBlacklist struct {
	mu      sync.RWMutex
	entries map[string]time.Time
}

func newMemBlacklist() *memBlacklist {
	return &memBlacklist{entries: make(map[string]time.Time)}
}

func (m *memBlacklist) AddToBlacklist(_ context.Context, jti string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[jti] = expiresAt
	return nil
}

func (m *memBlacklist) IsBlacklisted(_ context.Context, jti string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	exp, ok := m.entries[jti]
	if !ok {
		return false, nil
	}
	return time.Now().Before(exp), nil
}

func (m *memBlacklist) PruneExpired(_ context.Context) (int64, error) { return 0, nil }

func testJWTConfig() *config.Config {
	return &config.Config{
		JWTSecret:          "test-secret",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 30 * 24 * time.Hour,
	}
}

func TestJWTManager_GenerateAndValidate(t *testing.T) {
	manager := NewJWTManager(testJWTConfig(), newMemBlacklist())

	access, refresh, err := manager.GenerateTokens("testuser")
	if err != nil {
		t.Fatalf("Failed to generate tokens: %v", err)
	}

	if access == "" || refresh == "" {
		t.Fatalf("Tokens should not be empty")
	}

	// Validate access token
	_, claims, err := manager.ValidateToken(access, "access")
	if err != nil {
		t.Fatalf("Failed to validate access token: %v", err)
	}

	if sub, ok := claims["sub"].(string); !ok || sub != "testuser" {
		t.Errorf("Expected sub 'testuser', got %v", claims["sub"])
	}

	// JTI must be present
	if jti, ok := claims["jti"].(string); !ok || jti == "" {
		t.Errorf("Expected non-empty jti claim")
	}

	// Validate refresh token
	_, _, err = manager.ValidateToken(refresh, "refresh")
	if err != nil {
		t.Fatalf("Failed to validate refresh token: %v", err)
	}

	// Wrong type must be rejected
	_, _, err = manager.ValidateToken(access, "refresh")
	if err == nil {
		t.Fatalf("Expected error when validating access token as refresh token")
	}
}

func TestJWTManager_Blacklist(t *testing.T) {
	manager := NewJWTManager(testJWTConfig(), newMemBlacklist())
	jti := "test-jti-123"

	if manager.IsBlacklisted(jti) {
		t.Errorf("Expected jti not to be blacklisted initially")
	}

	manager.AddToBlacklist(jti, time.Now().Add(time.Minute))

	if !manager.IsBlacklisted(jti) {
		t.Errorf("Expected jti to be blacklisted")
	}
}

func TestJWTManager_BlacklistedTokenRejected(t *testing.T) {
	manager := NewJWTManager(testJWTConfig(), newMemBlacklist())

	access, _, err := manager.GenerateTokens("testuser")
	if err != nil {
		t.Fatalf("Failed to generate tokens: %v", err)
	}

	_, claims, err := manager.ValidateToken(access, "access")
	if err != nil {
		t.Fatalf("Expected valid token before blacklisting: %v", err)
	}

	jti, ok := claims["jti"].(string)
	if !ok {
		t.Fatal("expected claims[\"jti\"] to be string")
	}
	manager.AddToBlacklist(jti, time.Now().Add(15*time.Minute))

	_, _, err = manager.ValidateToken(access, "access")
	if err == nil {
		t.Fatalf("Expected error after blacklisting token JTI")
	}
}
