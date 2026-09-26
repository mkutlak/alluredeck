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
	if token2, _ := GenerateCSRFToken(); token == token2 {
		t.Error("expected distinct tokens on successive calls")
	}
}

// TestCSRFMiddleware covers the double-submit check. Rows carry a jwt session
// cookie unless noted, so a pass is due to the rule under test rather than
// the bypass for requests that are not cookie-authenticated.
func TestCSRFMiddleware(t *testing.T) {
	t.Parallel()
	const token, other = "aaaa1111", "bbbb2222"
	tests := []struct {
		name     string
		security bool
		method   string
		path     string
		noJWT    bool
		cookie   string // csrf_token cookie
		header   string // X-CSRF-Token header
		want     int
	}{
		{"security disabled", false, http.MethodPost, "/generate-report", false, "", "", http.StatusOK},
		{"GET needs no token", true, http.MethodGet, "/projects", false, "", "", http.StatusOK},
		{"HEAD needs no token", true, http.MethodHead, "/projects", false, "", "", http.StatusOK},
		{"OPTIONS needs no token", true, http.MethodOptions, "/projects", false, "", "", http.StatusOK},
		{"login is exempt", true, http.MethodPost, "/login", false, "", "", http.StatusOK},
		{"prefixed login is exempt", true, http.MethodPost, "/api/v1/login", false, "", "", http.StatusOK},
		{"no jwt cookie (API key client) bypasses", true, http.MethodPost, "/api/v1/api-keys", true, "", "", http.StatusOK},
		{"POST without a token", true, http.MethodPost, "/generate-report", false, "", "", http.StatusForbidden},
		{"DELETE without a token", true, http.MethodDelete, "/logout", false, "", "", http.StatusForbidden},
		{"mismatched tokens", true, http.MethodPost, "/generate-report", false, token, other, http.StatusForbidden},
		{"matching tokens", true, http.MethodPost, "/generate-report", false, token, token, http.StatusOK},
	}
	for _, tc := range tests {
		h := CSRFMiddleware(&config.Config{SecurityEnabled: tc.security})(http.HandlerFunc(okHandler))
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if !tc.noJWT {
			req.AddCookie(&http.Cookie{Name: "jwt", Value: "sometoken"})
		}
		if tc.cookie != "" {
			req.AddCookie(&http.Cookie{Name: "csrf_token", Value: tc.cookie})
		}
		if tc.header != "" {
			req.Header.Set("X-CSRF-Token", tc.header)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if rr.Code != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, rr.Code, tc.want)
		}
	}
}
