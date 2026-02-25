package middleware

import (
	"context"
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
		accessToken, _, _ := jwtManager.GenerateTokens("testuser")
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+accessToken)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("Expected 200, got %d", rr.Code)
		}
	})

	t.Run("ValidTokenCookie", func(t *testing.T) {
		accessToken, _, _ := jwtManager.GenerateTokens("testuser")
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
