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

// TestVerifyAPIKey_Expiration pins 1b80c3c: the MCP SDK rejects a zero
// Expiration, so every accepted key must carry one, and an expired key is
// refused outright.
func TestVerifyAPIKey_Expiration(t *testing.T) {
	t.Parallel()
	future := time.Now().Add(24 * time.Hour).Truncate(time.Second)
	past := time.Now().Add(-time.Hour)

	tests := []struct {
		name      string
		expiresAt *time.Time
		wantErr   string
	}{
		{"database expiry is used as-is", &future, ""},
		{"never-expiring key gets a 15-minute re-verification window", nil, ""},
		{"expired key is refused", &past, "API key has expired"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			keys := &testutil.MockAPIKeyStore{GetByHashFn: func(context.Context, string) (*store.APIKey, error) {
				return &store.APIKey{ID: 1, Username: "user@example.com", Role: "viewer", ExpiresAt: tc.expiresAt}, nil
			}}
			v := NewVerifier(keys, nil, nil, zap.NewNop())

			before := time.Now()
			info, err := v.verifyAPIKey(context.Background(), security.APIKeyPrefix+"dummyhash")
			after := time.Now()
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("verifyAPIKey: %v", err)
			}
			if tc.expiresAt != nil {
				if !info.Expiration.Equal(*tc.expiresAt) {
					t.Errorf("Expiration = %v, want %v", info.Expiration, *tc.expiresAt)
				}
				return
			}
			ceiling := after.Add(15*time.Minute + time.Second)
			if !info.Expiration.After(before) || info.Expiration.After(ceiling) {
				t.Errorf("Expiration = %v, want within (%v, %v]", info.Expiration, before, ceiling)
			}
		})
	}
}

func TestVerifyJWT_ExpirationMatchesJWTClaim(t *testing.T) {
	t.Parallel()
	jwtMgr := security.NewJWTManager(&config.Config{
		JWTSecret:          "test-secret-key",
		AccessTokenExpiry:  config.DurationSeconds(15 * time.Minute),
		RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
	}, testutil.NewMemBlacklist(), zap.NewNop())

	accessToken, _, err := jwtMgr.GenerateTokens("user@example.com", "viewer")
	if err != nil {
		t.Fatalf("GenerateTokens: %v", err)
	}
	_, claims, err := jwtMgr.ValidateToken(accessToken, "access")
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	exp, err := claims.GetExpirationTime()
	if err != nil || exp == nil {
		t.Fatalf("GetExpirationTime: exp=%v err=%v", exp, err)
	}

	info, err := NewVerifier(nil, jwtMgr, nil, zap.NewNop()).verifyJWT(context.Background(), accessToken)
	if err != nil {
		t.Fatalf("verifyJWT: %v", err)
	}
	if !info.Expiration.Equal(exp.Time) {
		t.Errorf("Expiration = %v, want the exp claim %v", info.Expiration, exp.Time)
	}
}
