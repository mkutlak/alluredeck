package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

func TestGenerateCSRFToken(t *testing.T) {
	t.Parallel()
	token, err := GenerateCSRFToken()
	if err != nil {
		t.Fatalf("GenerateCSRFToken failed: %v", err)
	}
	// 32 random bytes → 64 hex chars
	if len(token) != 64 {
		t.Errorf("expected 64-char hex token, got %d chars: %q", len(token), token)
	}

	// Two tokens must be distinct
	token2, _ := GenerateCSRFToken()
	if token == token2 {
		t.Error("expected distinct tokens on successive calls")
	}
}

func TestCSRFMiddleware_GETPassesWithoutToken(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{SecurityEnabled: true}
	handler := CSRFMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/projects", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("GET should pass without CSRF token, got %d", rr.Code)
	}
}

func TestCSRFMiddleware_HEADPassesWithoutToken(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{SecurityEnabled: true}
	handler := CSRFMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodHead, "/projects", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("HEAD should pass without CSRF token, got %d", rr.Code)
	}
}

func TestCSRFMiddleware_OPTIONSPassesWithoutToken(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{SecurityEnabled: true}
	handler := CSRFMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/projects", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("OPTIONS should pass without CSRF token, got %d", rr.Code)
	}
}

func TestCSRFMiddleware_POSTBlockedWithoutToken(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{SecurityEnabled: true}
	handler := CSRFMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/generate-report", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("POST without CSRF token should be 403, got %d", rr.Code)
	}
}

func TestCSRFMiddleware_POSTAllowedWithMatchingTokens(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{SecurityEnabled: true}
	handler := CSRFMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	token, _ := GenerateCSRFToken()

	req := httptest.NewRequest(http.MethodPost, "/generate-report", nil)
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: token})
	req.Header.Set("X-CSRF-Token", token)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("POST with matching CSRF tokens should be 200, got %d", rr.Code)
	}
}

func TestCSRFMiddleware_MismatchedTokensBlocked(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{SecurityEnabled: true}
	handler := CSRFMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	token1, _ := GenerateCSRFToken()
	token2, _ := GenerateCSRFToken()

	req := httptest.NewRequest(http.MethodPost, "/generate-report", nil)
	req.AddCookie(&http.Cookie{Name: "csrf_token", Value: token1})
	req.Header.Set("X-CSRF-Token", token2)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("POST with mismatched CSRF tokens should be 403, got %d", rr.Code)
	}
}

func TestCSRFMiddleware_SecurityDisabledSkips(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{SecurityEnabled: false}
	handler := CSRFMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodPost, "/generate-report", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("POST with security disabled should pass, got %d", rr.Code)
	}
}

func TestCSRFMiddleware_LoginPathExempt(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{SecurityEnabled: true}
	handler := CSRFMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Both /login and /api/v1/login should be exempt
	for _, path := range []string{"/login", "/api/v1/login"} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Errorf("POST %s should be exempt from CSRF, got %d", path, rr.Code)
		}
	}
}

func TestCSRFMiddleware_DELETEBlockedWithoutToken(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{SecurityEnabled: true}
	handler := CSRFMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodDelete, "/logout", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("DELETE without CSRF token should be 403, got %d", rr.Code)
	}
}
