package middleware

import (
	"net/http"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// CORSMiddleware enforces CORS using the configured allowlist.
//
// Set CORS_ALLOWED_ORIGINS to a comma-separated list of allowed origins, e.g.:
//
//	CORS_ALLOWED_ORIGINS=https://example.com,https://app.example.com
//
// Use * as the sole value to allow any origin (restores the original permissive
// behaviour, but note that credentials cannot be sent with wildcard origins):
//
//	CORS_ALLOWED_ORIGINS=*
//
// If CORS_ALLOWED_ORIGINS is empty (the default), no CORS headers are added and
// browsers enforce same-origin policy.
func CORSMiddleware(cfg *config.Config, next http.Handler) http.Handler {
	allowAll := false
	allowedOrigins := make(map[string]bool, len(cfg.CORSAllowedOrigins))
	for _, o := range cfg.CORSAllowedOrigins {
		if o == "*" {
			allowAll = true
		} else if o != "" {
			allowedOrigins[o] = true
		}
	}

	setCORSHeaders := func(w http.ResponseWriter, origin string, preflight bool) {
		switch {
		case allowAll:
			// Wildcard origin is incompatible with credentials
			w.Header().Set("Access-Control-Allow-Origin", "*")
		case allowedOrigins[origin]:
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		default:
			return // Origin not in allowlist — no CORS headers
		}

		if preflight {
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Add("Access-Control-Allow-Headers", "Content-Type")
			w.Header().Add("Access-Control-Allow-Headers", "x-csrf-token")
			w.Header().Add("Access-Control-Allow-Headers", "Authorization")
			w.Header().Set("Access-Control-Max-Age", "3600")
		}
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")

		if r.Method == http.MethodOptions {
			setCORSHeaders(w, origin, true)
			w.WriteHeader(http.StatusOK)
			return
		}

		setCORSHeaders(w, origin, false)
		next.ServeHTTP(w, r)
	})
}
