package middleware

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/logging"
)

// SecurityHeaders adds security-related HTTP response headers to every response.
// It is a factory rather than a bare middleware so HSTS can be gated on
// cfg.TLS — sending Strict-Transport-Security over plain HTTP is meaningless
// and confuses operators running local dev. Resolves HIGH #7 from
// SECURITY_REVIEW.md.
func SecurityHeaders(cfg *config.Config) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("X-Frame-Options", "DENY")
			w.Header().Set("Content-Security-Policy", "default-src 'self'")
			// Default browser referrer policy varies; pin to a value that does
			// not leak full URLs cross-origin while still allowing same-origin
			// debugging via Referer.
			w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
			// Deny browser features the API has no business handing to embedded
			// content (notably the report iframes). Add to this list before
			// surfacing any new feature that needs explicit grant.
			w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
			if cfg != nil && cfg.TLS {
				w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Recovery catches panics in downstream handlers, logs them, and returns a 500
// response instead of crashing the server process.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logging.FromContext(r.Context()).Error("panic recovered", zap.Any("error", err))
				writeMiddlewareError(w, http.StatusInternalServerError, "internal server error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}
