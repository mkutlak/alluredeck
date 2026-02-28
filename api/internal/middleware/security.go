package middleware

import (
	"encoding/json"
	"net/http"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/logging"
)

// SecurityHeaders adds security-related HTTP response headers to every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Content-Security-Policy", "default-src 'self'")
		next.ServeHTTP(w, r)
	})
}

// Recovery catches panics in downstream handlers, logs them, and returns a 500
// response instead of crashing the server process.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				logging.FromContext(r.Context()).Error("panic recovered", zap.Any("error", err))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"metadata": map[string]string{"message": "internal server error"},
				})
			}
		}()
		next.ServeHTTP(w, r)
	})
}
