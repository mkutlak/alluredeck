package mcp

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
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

// TestVerifyJWT pins 1b80c3c (an accepted token's Expiration is its exp
// claim) and 585de1c (the F-3 recheck refuses a deactivated user's token, an
// OIDC one by its email sub).
func TestVerifyJWT(t *testing.T) {
	t.Parallel()
	jwtMgr := security.NewJWTManager(&config.Config{
		JWTSecret:          "test-secret-key",
		AccessTokenExpiry:  config.DurationSeconds(15 * time.Minute),
		RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
	}, testutil.NewMemBlacklist(), zap.NewNop())

	// User 1 (active@example.com) is active; user 2 (gone@example.com) is deactivated.
	users := testutil.NewMemUserStore()
	for _, email := range []string{"active@example.com", "gone@example.com"} {
		if _, err := users.UpsertByOIDC(context.Background(), "oidc", "sub-"+email, email, email, "viewer"); err != nil {
			t.Fatal(err)
		}
	}
	if err := users.Deactivate(context.Background(), 2); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name, sub, provider, wantErr string
	}{
		{"expiration matches the exp claim", "user@example.com", "local", ""},
		{"active OIDC user", "active@example.com", "oidc", ""},
		{"deactivated OIDC user is refused", "gone@example.com", "oidc", "account inactive"},
		{"deactivated local user is refused", "2", "local", "account inactive"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			accessToken, _, err := jwtMgr.GenerateTokens(tc.sub, "viewer", tc.provider)
			if err != nil {
				t.Fatalf("GenerateTokens: %v", err)
			}
			cache := middleware.NewUserActiveCache(users, time.Minute, 0)

			info, err := NewVerifier(nil, jwtMgr, cache, zap.NewNop()).verifyJWT(context.Background(), accessToken)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("err = %v, want %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("verifyJWT: %v", err)
			}
			_, claims, err := jwtMgr.ValidateToken(accessToken, "access")
			if err != nil {
				t.Fatalf("ValidateToken: %v", err)
			}
			exp, err := claims.GetExpirationTime()
			if err != nil || exp == nil {
				t.Fatalf("GetExpirationTime: exp=%v err=%v", exp, err)
			}
			if !info.Expiration.Equal(exp.Time) {
				t.Errorf("Expiration = %v, want the exp claim %v", info.Expiration, exp.Time)
			}
		})
	}
}
