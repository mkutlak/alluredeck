package mcp

import (
	"net/http"
	"sync"

	"go.uber.org/zap"
)

// OriginMiddleware returns an HTTP middleware that enforces Origin header
// validation for DNS-rebinding defence (MCP spec 2025-03-26).
//
//   - allowed is a slice of exact-match Origin values. Empty slice = allow all
//     (a warning is emitted once via sync.Once since this is insecure for
//     browser clients, but correct for server-to-server / CLI usage).
//   - A missing Origin header is allowed (non-browser clients, curl, etc.).
//   - An Origin header present but not in the allowlist → 403.
func OriginMiddleware(allowed []string, logger *zap.Logger) func(http.Handler) http.Handler {
	var warnOnce sync.Once

	allowAll := len(allowed) == 0

	// Build O(1) lookup set.
	allowSet := make(map[string]struct{}, len(allowed))
	for _, o := range allowed {
		allowSet[o] = struct{}{}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// No Origin header — non-browser client; always allow.
			if origin == "" {
				next.ServeHTTP(w, r)
				return
			}

			if allowAll {
				warnOnce.Do(func() {
					logger.Warn("mcp: MCP_ALLOWED_ORIGINS is empty — all browser origins accepted; set MCP_ALLOWED_ORIGINS in production")
				})
				next.ServeHTTP(w, r)
				return
			}

			if _, ok := allowSet[origin]; ok {
				next.ServeHTTP(w, r)
				return
			}

			logger.Warn("mcp: origin rejected", zap.String("origin", origin))
			http.Error(w, "origin not allowed", http.StatusForbidden)
		})
	}
}
