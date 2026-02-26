package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/security"
)

// memBlacklist is an in-memory security.BlacklistStore for tests.
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

func TestAuthMiddleware(t *testing.T) {
	cfg := &config.Config{
		SecurityEnabled:    true,
		JWTSecret:          "test-secret",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 30 * 24 * time.Hour,
	}
	jwtManager := security.NewJWTManager(cfg, newMemBlacklist())

	handler := AuthMiddleware(cfg, jwtManager, false)(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	t.Run("SecurityDisabled", func(t *testing.T) {
		disabledCfg := &config.Config{SecurityEnabled: false}
		h := AuthMiddleware(disabledCfg, jwtManager, false)(func(w http.ResponseWriter, _ *http.Request) {
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
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Errorf("Expected 401, got %d", rr.Code)
		}
	})

	t.Run("ValidTokenHeader", func(t *testing.T) {
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
		meta, ok := resp["meta_data"].(map[string]any)
		if !ok {
			t.Fatal("expected meta_data in response")
		}
		msg, _ := meta["message"].(string)
		if msg != "Invalid token" {
			t.Errorf("Expected exact message \"Invalid token\", got %q", msg)
		}
	})

	t.Run("ExpiredToken_NoDetailsLeaked", func(t *testing.T) {
		// Create a config with a very short expiry and generate an already-expired token
		shortCfg := &config.Config{
			SecurityEnabled:    true,
			JWTSecret:          "test-secret",
			AccessTokenExpiry:  1 * time.Millisecond,
			RefreshTokenExpiry: 30 * 24 * time.Hour,
		}
		shortMgr := security.NewJWTManager(shortCfg, newMemBlacklist())
		expiredToken, _, _ := shortMgr.GenerateTokens("testuser", "admin")

		// Wait for the token to expire
		time.Sleep(5 * time.Millisecond)

		expiredHandler := AuthMiddleware(cfg, jwtManager, false)(func(w http.ResponseWriter, _ *http.Request) {
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
		meta, ok := resp["meta_data"].(map[string]any)
		if !ok {
			t.Fatal("expected meta_data in response")
		}
		msg, _ := meta["message"].(string)
		if msg != "Invalid token" {
			t.Errorf("Expected exact message \"Invalid token\" (no expiry details), got %q", msg)
		}
	})
}

func TestRequireRole(t *testing.T) {
	cfg := &config.Config{
		SecurityEnabled:    true,
		JWTSecret:          "test-secret",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 30 * 24 * time.Hour,
	}
	jwtMgr := security.NewJWTManager(cfg, newMemBlacklist())

	// Helper: build a handler chain with auth + role requirement
	makeHandler := func(requiredRole string) http.HandlerFunc {
		inner := func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}
		return AuthMiddleware(cfg, jwtMgr, false)(RequireRole(requiredRole)(inner))
	}

	t.Run("AdminAllowedOnAdminEndpoint", func(t *testing.T) {
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
}

func TestCORSMiddleware(t *testing.T) {
	t.Run("AllowAll", func(t *testing.T) {
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
	})

	t.Run("AllowSpecific", func(t *testing.T) {
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

		// Disallowed origin
		req = httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Origin", "http://malicious.com")
		rr = httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("Expected no Access-Control-Allow-Origin header")
		}
	})

	t.Run("Preflight", func(t *testing.T) {
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
	})
}
