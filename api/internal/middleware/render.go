package middleware

import (
	"encoding/json"
	"net/http"
)

// writeMiddlewareError writes a JSON error response with the standard metadata envelope.
// It is the middleware-layer equivalent of handlers.writeError, avoiding a circular import.
func writeMiddlewareError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"metadata": map[string]string{"message": msg},
	})
}
