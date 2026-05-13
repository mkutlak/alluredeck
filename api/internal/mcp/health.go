package mcp

import (
	"encoding/json"
	"net/http"
)

// HealthHandler returns a simple JSON health check response.
// It is mounted outside the MCP streamable-HTTP handler so load balancers
// and Kubernetes probes can reach it without MCP auth.
func HealthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
