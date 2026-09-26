package security

import (
	"errors"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestJWTManager: access tokens carry sub, role and a jti; each token
// validates only as its own type; blacklisting the jti revokes the token.
func TestJWTManager(t *testing.T) {
	t.Parallel()
	manager := NewJWTManager(&config.Config{
		JWTSecret:          "test-secret",
		AccessTokenExpiry:  config.DurationSeconds(15 * time.Minute),
		RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
	}, testutil.NewMemBlacklist(), zap.NewNop())

	access, refresh, err := manager.GenerateTokens("testuser", "admin")
	if err != nil {
		t.Fatalf("GenerateTokens: %v", err)
	}
	_, claims, err := manager.ValidateToken(access, "access")
	if err != nil {
		t.Fatalf("ValidateToken(access): %v", err)
	}
	jti, _ := claims["jti"].(string)
	if claims["sub"] != "testuser" || claims["role"] != "admin" || jti == "" {
		t.Errorf("access claims = %v, want sub testuser, role admin and a jti", claims)
	}
	if _, _, err := manager.ValidateToken(refresh, "refresh"); err != nil {
		t.Errorf("ValidateToken(refresh): %v", err)
	}
	if _, _, err := manager.ValidateToken(access, "refresh"); !errors.Is(err, ErrInvalidTokenType) {
		t.Errorf("access token as refresh: got %v, want ErrInvalidTokenType", err)
	}

	manager.AddToBlacklist(jti, time.Now().Add(15*time.Minute))
	if !manager.IsBlacklisted(jti) {
		t.Error("jti not blacklisted after AddToBlacklist")
	}
	if _, _, err := manager.ValidateToken(access, "access"); !errors.Is(err, ErrTokenRevoked) {
		t.Errorf("blacklisted token: got %v, want ErrTokenRevoked", err)
	}
}
