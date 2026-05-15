package mcp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	mcpauth "github.com/modelcontextprotocol/go-sdk/auth"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// Verifier implements MCP bearer-token verification.
// It supports two token types:
//   - API keys (ald_ prefix): hashed and looked up in the store.
//   - JWT access tokens: validated via JWTManager.
type Verifier struct {
	apiKeyStore     store.APIKeyStorer
	jwtManager      *security.JWTManager
	userActiveCache *middleware.UserActiveCache
	logger          *zap.Logger
}

// NewVerifier constructs a Verifier. userActiveCache may be nil (disables F-3 recheck).
func NewVerifier(
	apiKeyStore store.APIKeyStorer,
	jwtManager *security.JWTManager,
	userActiveCache *middleware.UserActiveCache,
	logger *zap.Logger,
) *Verifier {
	return &Verifier{
		apiKeyStore:     apiKeyStore,
		jwtManager:      jwtManager,
		userActiveCache: userActiveCache,
		logger:          logger,
	}
}

// Verify satisfies auth.TokenVerifier. It is called by auth.RequireBearerToken for
// every inbound request to the MCP server.
func (v *Verifier) Verify(ctx context.Context, token string, req *http.Request) (*mcpauth.TokenInfo, error) {
	// Reject cookie-based auth explicitly — MCP only accepts Bearer tokens.
	if req != nil {
		if _, err := req.Cookie("jwt"); err == nil {
			return nil, fmt.Errorf("cookie auth not accepted by MCP endpoint; use Bearer token")
		}
	}

	if strings.HasPrefix(token, security.APIKeyPrefix) {
		return v.verifyAPIKey(ctx, token)
	}
	return v.verifyJWT(ctx, token)
}

func (v *Verifier) verifyAPIKey(ctx context.Context, token string) (*mcpauth.TokenInfo, error) {
	hash := security.HashAPIKey(token)
	apiKey, err := v.apiKeyStore.GetByHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("invalid API key")
	}

	if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
		return nil, fmt.Errorf("API key has expired")
	}

	// F-3: re-check owning user is still active.
	if v.userActiveCache != nil {
		active, recheckErr := v.userActiveCache.IsActiveByAPIKeyUsername(ctx, apiKey.Username)
		if recheckErr != nil {
			v.logger.Warn("mcp auth: api key user active recheck failed",
				zap.String("username", apiKey.Username), zap.Error(recheckErr))
			// Fail open: transient DB error is not an authn signal.
		} else if !active {
			return nil, fmt.Errorf("account inactive")
		}
	}

	// Update last_used asynchronously — fire-and-forget (preserves F-3 semantics).
	// WithoutCancel keeps request-scoped values (trace IDs, etc.) but detaches
	// from the request's cancellation so the write completes after the response.
	asyncCtx := context.WithoutCancel(ctx)
	go func() {
		_ = v.apiKeyStore.UpdateLastUsed(asyncCtx, apiKey.ID)
	}()

	// Populate Expiration: the MCP SDK rejects tokens with a zero Expiration.
	// For keys with a real expiry, use it directly. For "never expires" keys
	// (ExpiresAt == nil) synthesise a short re-verification window so the SDK
	// re-calls Verify on a reasonable cadence, keeping revocation prompt.
	var expiration time.Time
	if apiKey.ExpiresAt != nil {
		expiration = *apiKey.ExpiresAt
	} else {
		expiration = time.Now().Add(15 * time.Minute)
	}

	allowMCPWrites := "false"
	if apiKey.AllowMCPWrites {
		allowMCPWrites = "true"
	}

	return &mcpauth.TokenInfo{
		UserID:     apiKey.Username,
		Scopes:     []string{apiKey.Role},
		Expiration: expiration,
		Extra: map[string]any{
			"role":             apiKey.Role,
			"api_key_id":       apiKey.ID,
			"allow_mcp_writes": allowMCPWrites,
			"username":         apiKey.Username,
			"user_id":          apiKey.Username,
		},
	}, nil
}

func (v *Verifier) verifyJWT(ctx context.Context, token string) (*mcpauth.TokenInfo, error) {
	_, claims, err := v.jwtManager.ValidateToken(token, "access")
	if err != nil {
		return nil, fmt.Errorf("invalid JWT: %w", err)
	}

	sub, _ := claims["sub"].(string)
	role, _ := claims["role"].(string)

	// F-3: re-check users.is_active for DB-backed users (numeric sub).
	if v.userActiveCache != nil {
		active, recheckErr := v.userActiveCache.IsActive(ctx, sub)
		if recheckErr != nil {
			v.logger.Warn("mcp auth: user active recheck failed",
				zap.String("sub", sub), zap.Error(recheckErr))
		} else if !active {
			return nil, fmt.Errorf("account inactive")
		}
	}

	// Populate Expiration from the JWT exp claim. The MCP SDK rejects tokens
	// with a zero Expiration. Use GetExpirationTime() which normalises float64,
	// *NumericDate, json.Number, and int64 representations of the exp claim.
	// Fall back to a 15-minute window on the unlikely case the claim is absent.
	var jwtExp time.Time
	if expNum, err := claims.GetExpirationTime(); err == nil && expNum != nil {
		jwtExp = expNum.Time
	} else {
		jwtExp = time.Now().Add(15 * time.Minute)
	}

	return &mcpauth.TokenInfo{
		UserID:     sub,
		Scopes:     []string{role},
		Expiration: jwtExp,
		Extra: map[string]any{
			"role":             role,
			"api_key_id":       int64(0),
			"allow_mcp_writes": "false",
			"username":         sub,
			"user_id":          sub,
		},
	}, nil
}
