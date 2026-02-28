package middleware

import (
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/logging"
)

// responseRecorder wraps http.ResponseWriter to capture the first status code written.
type responseRecorder struct {
	http.ResponseWriter
	statusCode int
	written    bool
}

func (r *responseRecorder) WriteHeader(code int) {
	if r.written {
		return
	}
	r.statusCode = code
	r.written = true
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseRecorder) Write(b []byte) (int, error) {
	if !r.written {
		r.WriteHeader(http.StatusOK)
	}
	return r.ResponseWriter.Write(b)
}

// LoggingMiddleware logs each request after it completes.
// It extracts the request_id injected by RequestID middleware, creates a
// child logger with request-scoped fields, stores it in the context via
// logging.WithContext, and after the handler returns emits a "request
// completed" log entry with method, path, status, and duration.
func LoggingMiddleware(logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			requestID := RequestIDFromContext(r.Context())
			childLogger := logger.With(
				zap.String("request_id", requestID),
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
			)

			ctx := logging.WithContext(r.Context(), childLogger)
			rec := &responseRecorder{ResponseWriter: w, statusCode: http.StatusOK}

			next.ServeHTTP(rec, r.WithContext(ctx))

			childLogger.Info("request completed",
				zap.Int("status", rec.statusCode),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}
