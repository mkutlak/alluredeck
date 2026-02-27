package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// GenerateCSRFToken produces a 64-character hex string from 32 random bytes.
func GenerateCSRFToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// CSRFMiddleware implements double-submit cookie CSRF protection.
// It compares the csrf_token cookie against the X-CSRF-Token header
// for state-changing methods (POST, PUT, DELETE, PATCH).
//
// Exempt: safe methods (GET, HEAD, OPTIONS) and the /login path.
// When security is disabled, the middleware is a no-op pass-through.
func CSRFMiddleware(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !cfg.SecurityEnabled {
				next.ServeHTTP(w, r)
				return
			}

			// Safe methods don't need CSRF protection
			switch r.Method {
			case http.MethodGet, http.MethodHead, http.MethodOptions:
				next.ServeHTTP(w, r)
				return
			}

			// Login endpoint is exempt — no CSRF cookie exists yet
			path := r.URL.Path
			if path == "/login" || strings.HasSuffix(path, "/login") {
				next.ServeHTTP(w, r)
				return
			}

			// Double-submit validation: cookie vs header
			cookie, err := r.Cookie("csrf_token")
			if err != nil || cookie.Value == "" {
				csrfForbidden(w)
				return
			}

			headerToken := r.Header.Get("X-CSRF-Token")
			if headerToken == "" {
				csrfForbidden(w)
				return
			}

			if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(headerToken)) != 1 {
				csrfForbidden(w)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func csrfForbidden(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"metadata": map[string]string{"message": "CSRF token missing or invalid"},
	})
}
