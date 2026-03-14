package security

import (
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func testJWTConfig() *config.Config {
	return &config.Config{
		JWTSecret:          "test-secret",
		AccessTokenExpiry:  config.DurationSeconds(15 * time.Minute),
		RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
	}
}

func TestJWTManager_GenerateAndValidate(t *testing.T) {
	t.Parallel()
	manager := NewJWTManager(testJWTConfig(), testutil.NewMemBlacklist(), zap.NewNop())

	access, refresh, err := manager.GenerateTokens("testuser", "admin")
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

func TestGenerateTokensWithRole(t *testing.T) {
	t.Parallel()
	manager := NewJWTManager(testJWTConfig(), testutil.NewMemBlacklist(), zap.NewNop())

	t.Run("AdminRole", func(t *testing.T) {
		t.Parallel()
		access, _, err := manager.GenerateTokens("admin-user", "admin")
		if err != nil {
			t.Fatalf("GenerateTokens failed: %v", err)
		}
		_, claims, err := manager.ValidateToken(access, "access")
		if err != nil {
			t.Fatalf("ValidateToken failed: %v", err)
		}
		role, ok := claims["role"].(string)
		if !ok || role != "admin" {
			t.Errorf("expected role 'admin', got %v", claims["role"])
		}
	})

	t.Run("ViewerRole", func(t *testing.T) {
		t.Parallel()
		access, _, err := manager.GenerateTokens("viewer-user", "viewer")
		if err != nil {
			t.Fatalf("GenerateTokens failed: %v", err)
		}
		_, claims, err := manager.ValidateToken(access, "access")
		if err != nil {
			t.Fatalf("ValidateToken failed: %v", err)
		}
		role, ok := claims["role"].(string)
		if !ok || role != "viewer" {
			t.Errorf("expected role 'viewer', got %v", claims["role"])
		}
	})
}

func TestJWTManager_Blacklist(t *testing.T) {
	t.Parallel()
	manager := NewJWTManager(testJWTConfig(), testutil.NewMemBlacklist(), zap.NewNop())
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
	t.Parallel()
	manager := NewJWTManager(testJWTConfig(), testutil.NewMemBlacklist(), zap.NewNop())

	access, _, err := manager.GenerateTokens("testuser", "admin")
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
