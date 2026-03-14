package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestAuthMiddleware(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		SecurityEnabled:    true,
		JWTSecret:          "test-secret",
		AccessTokenExpiry:  config.DurationSeconds(15 * time.Minute),
		RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
	}
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())

	handler := AuthMiddleware(cfg, jwtManager, false, nil)(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("SecurityDisabled", func(t *testing.T) {
		t.Parallel()
		disabledCfg := &config.Config{SecurityEnabled: false}
		h := AuthMiddleware(disabledCfg, jwtManager, false, nil)(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rr.Code)
		}
	})

	t.Run("MissingToken", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", rr.Code)
		}
	})

	t.Run("ValidTokenHeader", func(t *testing.T) {
		t.Parallel()
		accessToken, _, _ := jwtManager.GenerateTokens("testuser", "admin")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rr.Code)
		}
	})

	t.Run("ValidTokenCookie", func(t *testing.T) {
		t.Parallel()
		accessToken, _, _ := jwtManager.GenerateTokens("testuser", "admin")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.AddCookie(&http.Cookie{Name: "jwt", Value: accessToken})
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rr.Code)
		}
	})

	t.Run("InvalidToken", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", rr.Code)
		}

		// Must not leak internal error details (REVIEW #7)
		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to decode response body: %v", err)
		}
		meta, ok := resp["metadata"].(map[string]any)
		if !ok {
			t.Fatal("expected metadata in response")
		}
		msg, _ := meta["message"].(string)
		if msg != "Invalid token" {
			t.Errorf("Expected exact message \"Invalid token\", got %q", msg)
		}
	})

	t.Run("ExpiredToken_NoDetailsLeaked", func(t *testing.T) {
		t.Parallel()
		// Create a config with a very short expiry and generate an already-expired token
		shortCfg := &config.Config{
			SecurityEnabled:    true,
			JWTSecret:          "test-secret",
			AccessTokenExpiry:  config.DurationSeconds(1 * time.Millisecond),
			RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
		}
		shortMgr := security.NewJWTManager(shortCfg, testutil.NewMemBlacklist(), zap.NewNop())
		expiredToken, _, _ := shortMgr.GenerateTokens("testuser", "admin")

		// Wait for the token to expire
		time.Sleep(5 * time.Millisecond)

		expiredHandler := AuthMiddleware(cfg, jwtManager, false, nil)(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+expiredToken)
		rr := httptest.NewRecorder()
		expiredHandler.ServeHTTP(rr, req)

		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401 for expired token, got %d", rr.Code)
		}

		var resp map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to decode response body: %v", err)
		}
		meta, ok := resp["metadata"].(map[string]any)
		if !ok {
			t.Fatal("expected metadata in response")
		}
		msg, _ := meta["message"].(string)
		if msg != "Invalid token" {
			t.Errorf("Expected exact message \"Invalid token\" (no expiry details), got %q", msg)
		}
	})
}

func TestRequireRole(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		SecurityEnabled:    true,
		JWTSecret:          "test-secret",
		AccessTokenExpiry:  config.DurationSeconds(15 * time.Minute),
		RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
	}
	jwtMgr := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())

	// Helper: build a handler chain with auth + role requirement
	makeHandler := func(requiredRole string) http.HandlerFunc {
		inner := func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}
		return AuthMiddleware(cfg, jwtMgr, false, nil)(RequireRole(requiredRole)(inner))
	}

	t.Run("AdminAllowedOnAdminEndpoint", func(t *testing.T) {
		t.Parallel()
		token, _, _ := jwtMgr.GenerateTokens("admin-user", "admin")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		makeHandler("admin").ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for admin on admin endpoint, got %d", rr.Code)
		}
	})

	t.Run("ViewerDeniedOnAdminEndpoint", func(t *testing.T) {
		t.Parallel()
		token, _, _ := jwtMgr.GenerateTokens("viewer-user", "viewer")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		makeHandler("admin").ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 for viewer on admin endpoint, got %d", rr.Code)
		}
	})

	t.Run("ViewerAllowedOnViewerEndpoint", func(t *testing.T) {
		t.Parallel()
		token, _, _ := jwtMgr.GenerateTokens("viewer-user", "viewer")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		makeHandler("viewer").ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for viewer on viewer endpoint, got %d", rr.Code)
		}
	})

	t.Run("AdminAllowedOnViewerEndpoint", func(t *testing.T) {
		t.Parallel()
		token, _, _ := jwtMgr.GenerateTokens("admin-user", "admin")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		makeHandler("viewer").ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for admin on viewer endpoint (hierarchy), got %d", rr.Code)
		}
	})

	t.Run("MissingClaimsReturns403", func(t *testing.T) {
		t.Parallel()
		// Call RequireRole directly without AuthMiddleware setting claims
		handler := RequireRole("admin")(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 when claims are missing, got %d", rr.Code)
		}
	})

	t.Run("EditorAllowedOnEditorEndpoint", func(t *testing.T) {
		t.Parallel()
		token, _, _ := jwtMgr.GenerateTokens("editor-user", "editor")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		makeHandler("editor").ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for editor on editor endpoint, got %d", rr.Code)
		}
	})

	t.Run("ViewerDeniedOnEditorEndpoint", func(t *testing.T) {
		t.Parallel()
		token, _, _ := jwtMgr.GenerateTokens("viewer-user", "viewer")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		makeHandler("editor").ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 for viewer on editor endpoint, got %d", rr.Code)
		}
	})

	t.Run("AdminAllowedOnEditorEndpoint", func(t *testing.T) {
		t.Parallel()
		token, _, _ := jwtMgr.GenerateTokens("admin-user", "admin")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		makeHandler("editor").ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for admin on editor endpoint (hierarchy), got %d", rr.Code)
		}
	})

	t.Run("EditorDeniedOnAdminEndpoint", func(t *testing.T) {
		t.Parallel()
		token, _, _ := jwtMgr.GenerateTokens("editor-user", "editor")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		makeHandler("admin").ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("expected 403 for editor on admin endpoint, got %d", rr.Code)
		}
	})

	t.Run("EditorAllowedOnViewerEndpoint", func(t *testing.T) {
		t.Parallel()
		token, _, _ := jwtMgr.GenerateTokens("editor-user", "editor")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		makeHandler("viewer").ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("expected 200 for editor on viewer endpoint (hierarchy), got %d", rr.Code)
		}
	})
}

func TestAuthMiddleware_APIKeyValid(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		SecurityEnabled:    true,
		JWTSecret:          "test-secret",
		AccessTokenExpiry:  config.DurationSeconds(15 * time.Minute),
		RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
	}
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())

	now := time.Now().Add(time.Hour)
	apiKeyStore := &testutil.MockAPIKeyStore{
		GetByHashFn: func(_ context.Context, _ string) (*store.APIKey, error) {
			return &store.APIKey{
				ID:        1,
				Username:  "apiuser",
				Role:      "admin",
				ExpiresAt: &now,
			}, nil
		},
		UpdateLastUsedFn: func(_ context.Context, _ int64) error {
			return nil
		},
	}

	// Use a valid ald_ token — hash lookup is mocked so value doesn't matter
	token := "ald_" + "a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"

	var capturedClaims any
	handler := AuthMiddleware(cfg, jwtManager, false, apiKeyStore)(func(w http.ResponseWriter, r *http.Request) {
		capturedClaims, _ = ClaimsFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for valid API key, got %d", rr.Code)
	}
	if capturedClaims == nil {
		t.Error("expected claims to be injected in context")
	}
}

func TestAuthMiddleware_APIKeyExpired(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		SecurityEnabled:    true,
		JWTSecret:          "test-secret",
		AccessTokenExpiry:  config.DurationSeconds(15 * time.Minute),
		RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
	}
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())

	past := time.Now().Add(-time.Hour)
	apiKeyStore := &testutil.MockAPIKeyStore{
		GetByHashFn: func(_ context.Context, _ string) (*store.APIKey, error) {
			return &store.APIKey{
				ID:        2,
				Username:  "apiuser",
				Role:      "viewer",
				ExpiresAt: &past,
			}, nil
		},
	}

	token := "ald_" + "b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3"

	handler := AuthMiddleware(cfg, jwtManager, false, apiKeyStore)(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired API key, got %d", rr.Code)
	}
}

func TestAuthMiddleware_APIKeyInvalid(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		SecurityEnabled:    true,
		JWTSecret:          "test-secret",
		AccessTokenExpiry:  config.DurationSeconds(15 * time.Minute),
		RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
	}
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())

	apiKeyStore := &testutil.MockAPIKeyStore{
		GetByHashFn: func(_ context.Context, _ string) (*store.APIKey, error) {
			return nil, store.ErrAPIKeyNotFound
		},
	}

	token := "ald_" + "c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4"

	handler := AuthMiddleware(cfg, jwtManager, false, apiKeyStore)(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/api-keys", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid API key, got %d", rr.Code)
	}
}

func TestAuthMiddleware_NonAldBearerUsesJWT(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		SecurityEnabled:    true,
		JWTSecret:          "test-secret",
		AccessTokenExpiry:  config.DurationSeconds(15 * time.Minute),
		RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
	}
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())

	// Provide an apiKeyStore that should NOT be called for a non-ald_ token
	apiKeyStore := &testutil.MockAPIKeyStore{
		GetByHashFn: func(_ context.Context, _ string) (*store.APIKey, error) {
			panic("apiKeyStore.GetByHash should not be called for non-ald_ token")
		},
	}

	accessToken, _, _ := jwtManager.GenerateTokens("jwtuser", "viewer")

	handler := AuthMiddleware(cfg, jwtManager, false, apiKeyStore)(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer "+accessToken)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for valid JWT token (non-ald_), got %d", rr.Code)
	}
}

func TestCORSMiddleware(t *testing.T) {
	t.Parallel()
	t.Run("AllowAll", func(t *testing.T) {
		t.Parallel()
		cfg := &config.Config{CORSAllowedOrigins: []string{"*"}}
		next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
		h := CORSMiddleware(cfg, next)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "http://example.com")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("Expected Access-Control-Allow-Origin: *")
		}
		if rr.Header().Get("Vary") != "Origin" {
			t.Errorf("Expected Vary: Origin, got %q", rr.Header().Get("Vary"))
		}
	})

	t.Run("AllowSpecific", func(t *testing.T) {
		t.Parallel()
		cfg := &config.Config{CORSAllowedOrigins: []string{"http://example.com"}}
		next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
		h := CORSMiddleware(cfg, next)

		// Allowed origin
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "http://example.com")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Header().Get("Access-Control-Allow-Origin") != "http://example.com" {
			t.Errorf("Expected Access-Control-Allow-Origin: http://example.com")
		}
		if rr.Header().Get("Access-Control-Allow-Credentials") != "true" {
			t.Errorf("Expected Access-Control-Allow-Credentials: true")
		}
		if rr.Header().Get("Vary") != "Origin" {
			t.Errorf("Expected Vary: Origin for allowed origin, got %q", rr.Header().Get("Vary"))
		}

		// Disallowed origin
		req = httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "http://malicious.com")
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("Expected no Access-Control-Allow-Origin header")
		}
		// Vary: Origin must still be set even for disallowed origins (cache keying)
		if rr.Header().Get("Vary") != "Origin" {
			t.Errorf("Expected Vary: Origin for disallowed origin, got %q", rr.Header().Get("Vary"))
		}
	})

	t.Run("Preflight", func(t *testing.T) {
		t.Parallel()
		cfg := &config.Config{CORSAllowedOrigins: []string{"*"}}
		next := http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {})
		h := CORSMiddleware(cfg, next)

		req := httptest.NewRequest(http.MethodOptions, "/", nil)
		req.Header.Set("Origin", "http://example.com")
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200 OK for preflight")
		}
		if rr.Header().Get("Access-Control-Allow-Methods") == "" {
			t.Errorf("Expected Access-Control-Allow-Methods header")
		}
		if rr.Header().Get("Vary") != "Origin" {
			t.Errorf("Expected Vary: Origin on preflight, got %q", rr.Header().Get("Vary"))
		}
	})
}
