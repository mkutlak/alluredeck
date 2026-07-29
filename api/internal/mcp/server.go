package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/mcp/tools"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/version"
)

// ResourceMetadataPath is the RFC 9728 protected-resource metadata path. It is
// served unauthenticated (discovery must work before the client holds a token)
// and is advertised in the WWW-Authenticate header of every 401.
const ResourceMetadataPath = "/.well-known/oauth-protected-resource"

// Cache-scope values for the Cacheable hint on list results (MCP 2026-07-28).
// "public" means any client or intermediary may cache and serve the response;
// both lists below are identical for every caller, so "public" is correct.
const cacheScopePublic = "public"

// TTL hints advertised on list results. The tool and resource-template sets are
// built once in NewServer and never mutate, so they are static for the process
// lifetime; the ceilings below bound how long a client may miss a redeploy.
const (
	toolsListTTLMs     = 60 * 60 * 1000 // 1 hour
	resourcesListTTLMs = 5 * 60 * 1000  // 5 minutes
)

// serverInstructions is handed to clients at discovery time. It carries the
// cross-tool workflow rules that no single tool description can own, so agents
// learn them before making the first call rather than after a failed one.
const serverInstructions = `AllureDeck exposes Allure test-report data: projects, builds, failing tests, failure history, and defect triage.

Workflow rules:
  - Given an AllureDeck UI URL, call resolve_url FIRST. The build_number in a URL is NOT the build_id every other tool requires; passing one for the other silently returns the wrong build.
  - Call list_projects to discover project IDs before anything else if you were given a project name rather than an ID.
  - history_id is mandatory wherever it appears and must be non-empty. Obtain it from find_test_by_name or list_failing_tests; do not construct one.
  - Order builds by build_order, never by id. The id column reflects ingestion order and diverges from build order after a backfill.
  - diagnose_failure is the highest-signal entry point for "why did this build fail" — it returns per-test error messages, failed-step paths, triage signals, and a last_good pointer in one call. Prefer it over calling get_test_failure in a loop.

The propose_* tools never apply a change directly. They record a proposal that a human must approve in the AllureDeck admin UI, and they require an editor/admin role plus an API key with allow_mcp_writes enabled.`

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
	// ToolCosts maps a tool name to how many requests a call to it is worth
	// for rate-limiting purposes. Nil falls back to the built-in defaults.
	ToolCosts map[string]int
	// Stateless selects the MCP 2026-07-28 stateless transport.
	Stateless bool
}

// RegisterTools is called by the tools sub-package to add MCP tool handlers to
// the server. publicURL is prepended to the review links returned by the
// propose_* tools; signingKey authenticates the opaque RequestState those tools
// hand back to the client during a multi-round-trip confirmation.
func RegisterTools(s *mcpsdk.Server, stores *bootstrap.Stores, logger *zap.Logger, publicURL string, signingKey []byte) {
	tools.RegisterAll(s, stores, logger, publicURL, signingKey)
}

// cacheableListMiddleware stamps the MCP 2026-07-28 Cacheable hint (ttlMs /
// cacheScope) onto list results. The SDK exposes no ServerOptions knob for
// this, so it is applied as a receiving middleware over the list methods.
//
// Only list results are stamped. Tool call results and resource reads carry
// live data and set their own freshness where appropriate.
func cacheableListMiddleware() mcpsdk.Middleware {
	return func(next mcpsdk.MethodHandler) mcpsdk.MethodHandler {
		return func(ctx context.Context, method string, req mcpsdk.Request) (mcpsdk.Result, error) {
			res, err := next(ctx, method, req)
			if err != nil {
				return res, err
			}
			switch r := res.(type) {
			case *mcpsdk.ListToolsResult:
				r.TTLMs = toolsListTTLMs
				r.CacheScope = cacheScopePublic
			case *mcpsdk.ListResourcesResult:
				r.TTLMs = resourcesListTTLMs
				r.CacheScope = cacheScopePublic
			case *mcpsdk.ListResourceTemplatesResult:
				r.TTLMs = resourcesListTTLMs
				r.CacheScope = cacheScopePublic
			}
			return res, nil
		}
	}
}

// ProtectedResourceMetadataHandler returns the RFC 9728 metadata document
// handler for this MCP deployment. Mount it at ResourceMetadataPath OUTSIDE the
// auth and rate-limit chain: a client fetches it precisely because it does not
// yet hold a usable token.
//
// AllureDeck is a resource server only. It issues no OAuth tokens, so
// AuthorizationServers is intentionally empty — the document exists so clients
// can discover the resource identifier and that bearer tokens go in the header.
func ProtectedResourceMetadataHandler(publicURL string) http.Handler {
	base := strings.TrimRight(publicURL, "/")
	return mcpauth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{
		Resource:               base + "/mcp",
		ResourceName:           "AllureDeck MCP",
		BearerMethodsSupported: []string{"header"},
		ScopesSupported:        []string{"viewer", "editor", "admin"},
	})
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
		Name:        "alluredeck-mcp",
		Title:       "AllureDeck",
		Description: "Query Allure test-report data — builds, failing tests, failure history, and defect triage.",
		Version:     version.Version,
	}
	mcpServer := mcpsdk.NewServer(impl, &mcpsdk.ServerOptions{
		Instructions: serverInstructions,
	})

	// Advertise cache lifetimes on list results (MCP 2026-07-28).
	mcpServer.AddReceivingMiddleware(cacheableListMiddleware())

	// Register tools.
	RegisterTools(mcpServer, stores, logger, cfg.PublicURL, cfg.SigningKey)

	// Register resource handlers.
	RegisterResources(mcpServer, stores, logger, cfg.SigningKey, cfg.PublicURL, cfg.DataStore)

	// Streamable HTTP transport — one MCP server instance shared across all requests.
	streamHandler := mcpsdk.NewStreamableHTTPHandler(func(_ *http.Request) *mcpsdk.Server {
		return mcpServer
	}, &mcpsdk.StreamableHTTPOptions{
		// Stateless is the 2026-07-28 core model: no initialize handshake, no
		// Mcp-Session-Id, every request self-describing. Nothing here uses the
		// server→client SSE stream, so rejecting GET/DELETE costs nothing and
		// lets mcp.replicaCount exceed 1 behind the ingress.
		Stateless: cfg.Stateless,
		// Tie the handler context to HTTP request cancellation (protocol
		// >= 2026-07-28). diagnose_failure and get_test_history run heavy
		// joins; a client abort should cancel the query, not orphan it.
		PropagateRequestCancellation: true,
		// OriginMiddleware below is our DNS-rebinding defence.
		DisableLocalhostProtection: true,
	})

	// Build middleware chain (innermost first, then wrap outward).
	// 1. Rate limiter (innermost — runs after auth injects identity).
	rateLimiter := NewRateLimiterWithCosts(cfg.RateLimitPerMin, cfg.RateLimitBurst, cfg.ToolCosts)

	// 2. Auth middleware from go-sdk/auth. ResourceMetadataURL is echoed in the
	// WWW-Authenticate header of every 401 so clients can discover how to
	// authenticate (RFC 9728) instead of being told only that they failed.
	verifier := NewVerifier(stores.APIKey, jwtManager, userActiveCache, logger)
	authMiddleware := mcpauth.RequireBearerToken(verifier.Verify, &mcpauth.RequireBearerTokenOptions{
		ResourceMetadataURL: strings.TrimRight(cfg.PublicURL, "/") + ResourceMetadataPath,
	})

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
