package middleware

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
)

type requestIDKey struct{}

// RequestID is middleware that assigns a correlation ID to every request.
// If the client sends an X-Request-ID header, it is reused; otherwise a
// new UUID-like identifier is generated from crypto/rand.
// The ID is stored in the request context and set on the response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = generateID()
		}

		w.Header().Set("X-Request-ID", id)
		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestIDFromContext retrieves the request ID stored by the RequestID middleware.
// Returns an empty string if no ID is present.
func RequestIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// generateID produces a UUID v4-like string using crypto/rand.
func generateID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	// Set version (4) and variant (RFC 4122) bits
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
