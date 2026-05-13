package mcp

import (
	"fmt"
	"net/http"
	"strings"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/storage"
)

// Config holds MCP server configuration derived from environment variables.
type Config struct {
	// AllowedOrigins is the list of exact-match Origin header values accepted by
	// the MCP endpoint. Empty = allow all (warn once).
	AllowedOrigins []string
	// RateLimitPerMin is the sustained request rate per API key / user (req/min).
	RateLimitPerMin int
	// RateLimitBurst is the burst allowance for the per-identity token bucket.
	RateLimitBurst int
	// PublicURL is the external base URL for the MCP server (used in resource URIs).
	PublicURL string
	// SigningKey is the HMAC-SHA256 key used to sign time-limited attachment
	// download URLs. Must be non-empty when MCPServerEnabled=true.
	SigningKey []byte
	// DataStore is the file storage backend used to inline attachment content.
	// When nil, all attachments are returned as signed resource links.
	DataStore storage.Store
}

// RegisterTools is called by the tools sub-package to add MCP tool handlers to
// the server. The empty body here acts as the extension point; it is populated
// by the tools package in Phase 2.
func RegisterTools(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger) {
	tools.RegisterAll(s, stores, logger)
}

// NewServer wires the full MCP HTTP handler chain:
//
//	otelhttp → OriginMiddleware → auth.RequireBearerToken → RateLimit → streamableHandler
//
// It returns the raw http.Handler (for mounting in a *http.ServeMux), the
// *mcpsdk.Server (so callers can register tools after construction), and any
// initialisation error.
func NewServer(
	cfg Config,
	stores *bootstrap.Stores,
	jwtManager *security.JWTManager,
	userActiveCache *middleware.UserActiveCache,
	logger *zap.Logger,
) (http.Handler, *mcpsdk.Server, error) {
	if stores == nil {
		return nil, nil, fmt.Errorf("mcp.NewServer: stores must not be nil")
	}
	if jwtManager == nil {
		return nil, nil, fmt.Errorf("mcp.NewServer: jwtManager must not be nil")
	}

	// Build the MCP server instance.
	impl := &mcpsdk.Implementation{
		Name:    "alluredeck-mcp",
		Version: "1.0.0",
	}
	mcpServer := mcpsdk.NewServer(impl, nil)

	// Register tools.
	RegisterTools(mcpServer, stores, logger)

	// Register resource handlers (Phase 2).
	// RegisterResources(mcpServer, stores, logger, cfg.SigningKey, cfg.PublicURL, cfg.DataStore)

	// Streamable HTTP transport — one MCP server instance shared across all requests.
	streamHandler := mcpsdk.NewStreamableHTTPHandler(func(_ *http.Request) *mcpsdk.Server {
		return mcpServer
	}, &mcpsdk.StreamableHTTPOptions{
		DisableLocalhostProtection: true,
	})

	// Build middleware chain (innermost first, then wrap outward).
	// 1. Rate limiter (innermost — runs after auth injects identity).
	rateLimiter := NewRateLimiter(cfg.RateLimitPerMin, cfg.RateLimitBurst)

	// 2. Auth middleware from go-sdk/auth.
	verifier := NewVerifier(stores.APIKey, jwtManager, userActiveCache, logger)
	authMiddleware := mcpauth.RequireBearerToken(verifier.Verify, &mcpauth.RequireBearerTokenOptions{})

	// 3. Origin validation (DNS-rebinding defence).
	originMiddleware := OriginMiddleware(cfg.AllowedOrigins, logger)

	// Chain: originMiddleware → authMiddleware → rateLimiter → streamHandler
	inner := rateLimiter.Middleware(streamHandler)
	withAuth := authMiddleware(inner)
	withOrigin := originMiddleware(withAuth)

	// 4. OTel instrumentation (outermost HTTP wrapper).
	handler := otelhttp.NewHandler(withOrigin, "mcp")

	return handler, mcpServer, nil
}

// ParseAllowedOrigins splits a comma-separated MCP_ALLOWED_ORIGINS string into
// a trimmed slice of origin values. Empty input returns nil (allow all).
func ParseAllowedOrigins(raw string) []string {
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
