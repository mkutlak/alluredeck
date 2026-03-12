package handlers

import (
	"encoding/json"
	"net/http"
)

// writeJSON sets Content-Type to application/json, writes the given status code,
// and encodes v as JSON into the response body.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError writes a JSON error response with the standard metadata envelope:
//
//	{"metadata": {"message": msg}}
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{
		"metadata": map[string]string{"message": msg},
	})
}
