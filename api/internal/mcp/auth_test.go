package mcp

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// testConfig returns a minimal config for JWTManager construction in tests.
func testConfig() *config.Config {
	return &config.Config{
		JWTSecret:          "test-secret-key",
		AccessTokenExpiry:  config.DurationSeconds(15 * time.Minute),
		RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
	}
}

// stubAPIKeyStore is a minimal in-memory APIKeyStorer for mcp auth tests.
type stubAPIKeyStore struct {
	key *store.APIKey
	err error
}

func (s *stubAPIKeyStore) GetByHash(_ context.Context, _ string) (*store.APIKey, error) {
	return s.key, s.err
}
func (s *stubAPIKeyStore) Create(_ context.Context, _ *store.APIKey) (*store.APIKey, error) {
	return nil, nil
}
func (s *stubAPIKeyStore) ListByUsername(_ context.Context, _ string) ([]store.APIKey, error) {
	return nil, nil
}
func (s *stubAPIKeyStore) UpdateLastUsed(_ context.Context, _ int64) error { return nil }
func (s *stubAPIKeyStore) Delete(_ context.Context, _ int64, _ string) error {
	return nil
}
func (s *stubAPIKeyStore) CountByUsername(_ context.Context, _ string) (int, error) { return 0, nil }
func (s *stubAPIKeyStore) DeleteAllForUser(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func newTestVerifier(keyStore store.APIKeyStorer, jwtMgr *security.JWTManager) *Verifier {
	return NewVerifier(keyStore, jwtMgr, nil, zap.NewNop())
}

// ---------------------------------------------------------------------------
// verifyAPIKey tests
// ---------------------------------------------------------------------------

func TestVerifyAPIKey_WithExpiration_ReturnsDBExpiry(t *testing.T) {
	t.Parallel()

	exp := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	key := &store.APIKey{
		ID:        1,
		Username:  "user@example.com",
		Role:      "viewer",
		ExpiresAt: &exp,
	}
	stub := &stubAPIKeyStore{key: key}
	v := newTestVerifier(stub, nil)

	info, err := v.verifyAPIKey(context.Background(), security.APIKeyPrefix+"dummyhash")
	if err != nil {
		t.Fatalf("verifyAPIKey returned unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("verifyAPIKey returned nil TokenInfo")
	}
	if !info.Expiration.Equal(exp) {
		t.Errorf("Expiration = %v, want %v", info.Expiration, exp)
	}
}

func TestVerifyAPIKey_NeverExpires_ExpirationInFuture(t *testing.T) {
	t.Parallel()

	key := &store.APIKey{
		ID:        2,
		Username:  "user@example.com",
		Role:      "viewer",
		ExpiresAt: nil, // never expires
	}
	stub := &stubAPIKeyStore{key: key}
	v := newTestVerifier(stub, nil)

	before := time.Now()
	info, err := v.verifyAPIKey(context.Background(), security.APIKeyPrefix+"dummyhash")
	after := time.Now()
	if err != nil {
		t.Fatalf("verifyAPIKey returned unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("verifyAPIKey returned nil TokenInfo")
	}
	if info.Expiration.IsZero() {
		t.Fatal("Expiration must not be zero for never-expires key")
	}
	if !info.Expiration.After(before) {
		t.Errorf("Expiration %v is not after call time %v", info.Expiration, before)
	}
	maxExp := after.Add(15 * time.Minute).Add(time.Second) // small buffer for clock skew
	if info.Expiration.After(maxExp) {
		t.Errorf("Expiration %v exceeds expected 15-min window ceiling %v", info.Expiration, maxExp)
	}
}

func TestVerifyAPIKey_Expired_ReturnsError(t *testing.T) {
	t.Parallel()

	past := time.Now().Add(-1 * time.Hour)
	key := &store.APIKey{
		ID:        3,
		Username:  "user@example.com",
		Role:      "viewer",
		ExpiresAt: &past,
	}
	stub := &stubAPIKeyStore{key: key}
	v := newTestVerifier(stub, nil)

	_, err := v.verifyAPIKey(context.Background(), security.APIKeyPrefix+"dummyhash")
	if err == nil {
		t.Fatal("verifyAPIKey should return error for expired key")
	}
	const want = "API key has expired"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

// ---------------------------------------------------------------------------
// verifyJWT tests
// ---------------------------------------------------------------------------

func TestVerifyJWT_ExpirationMatchesJWTClaim(t *testing.T) {
	t.Parallel()

	cfg := testConfig()
	jwtMgr := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())

	// Generate a real access token — jwt.go always sets exp.
	accessToken, _, err := jwtMgr.GenerateTokens("user@example.com", "viewer")
	if err != nil {
		t.Fatalf("GenerateTokens: %v", err)
	}

	// Extract the expected exp directly from the token claims.
	_, claims, err := jwtMgr.ValidateToken(accessToken, "access")
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	expNum, claimErr := claims.GetExpirationTime()
	if claimErr != nil || expNum == nil {
		t.Fatalf("GetExpirationTime: err=%v, expNum=%v", claimErr, expNum)
	}
	wantExp := expNum.Time

	v := newTestVerifier(nil, jwtMgr)
	info, err := v.verifyJWT(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("verifyJWT returned unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("verifyJWT returned nil TokenInfo")
	}
	if !info.Expiration.Equal(wantExp) {
		t.Errorf("Expiration = %v, want %v", info.Expiration, wantExp)
	}
	if info.Expiration.IsZero() {
		t.Fatal("Expiration must not be zero")
	}
}
