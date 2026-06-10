package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/logging"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

type contextKey string

// ClaimsKey is the context key used to store JWT claims for downstream handlers
const ClaimsKey contextKey = "jwt_claims"

// AuthMiddleware protects routes using the JWT manager.
// When security is disabled, the request passes through without validation.
// On success, the parsed JWT claims are stored in the request context under ClaimsKey.
// When apiKeyStore is non-nil and the token has the "ald_" prefix, API key authentication
// is used instead of JWT validation.
//
// userActiveCache, when non-nil, enforces a per-request re-check of
// users.is_active for DB-backed users (numeric JWT sub) and for API-key
// authenticated requests. This is the F-3 defence-in-depth control: even if a
// future code path forgets to revoke explicit sessions on deactivation, the
// next request after the cache TTL (30s by default) will be rejected.
// Pass nil to disable the recheck (preserves old test wiring).
func AuthMiddleware(
	cfg *config.Config,
	jwtManager *security.JWTManager,
	isRefresh bool,
	apiKeyStore store.APIKeyStorer,
	userActiveCache *UserActiveCache,
) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !cfg.SecurityEnabled {
				next(w, r)
				return
			}

			cookieName := "jwt"
			expectedType := "access"
			if isRefresh {
				cookieName = "refresh_jwt"
				expectedType = "refresh"
			}

			tokenStr := extractToken(r, cookieName)

			if tokenStr == "" {
				writeMiddlewareError(w, http.StatusUnauthorized, "Missing authorization token")
				return
			}

			// API key authentication path: ald_ prefix tokens
			if !isRefresh && strings.HasPrefix(tokenStr, security.APIKeyPrefix) && apiKeyStore != nil {
				hash := security.HashAPIKey(tokenStr)
				apiKey, err := apiKeyStore.GetByHash(r.Context(), hash)
				if err != nil {
					writeMiddlewareError(w, http.StatusUnauthorized, "Invalid API key")
					return
				}
				if apiKey.ExpiresAt != nil && apiKey.ExpiresAt.Before(time.Now()) {
					writeMiddlewareError(w, http.StatusUnauthorized, "API key has expired")
					return
				}
				// F-3: re-check that the API key's owning user is still active.
				// Dispatch by username shape (env literal / numeric ID / email)
				// so all historical api_keys.username values are handled. On
				// transient DB error or ErrUserNotFound (data inconsistency)
				// we fail open and log a warning — the explicit revocation in
				// F-2 is the primary control, and we do not want a blip or
				// a stale row to block legitimate API-key clients.
				if userActiveCache != nil {
					active, recheckErr := userActiveCache.IsActiveByAPIKeyUsername(r.Context(), apiKey.Username)
					if recheckErr != nil {
						logging.FromContext(r.Context()).Warn("auth: api key user active recheck failed",
							zap.String("username", apiKey.Username), zap.Error(recheckErr))
						// Fail open: data-inconsistency / transient DB error is
						// not an authn signal; let the request through.
					} else if !active {
						writeMiddlewareError(w, http.StatusUnauthorized, "Account inactive")
						return
					}
				}
				// Update last_used asynchronously — fire-and-forget.
				// Use the request context so the update is cancelled if the
				// client disconnects, and to satisfy gosec G118.
				reqCtx := r.Context()
				go func() {
					_ = apiKeyStore.UpdateLastUsed(reqCtx, apiKey.ID)
				}()
				// Inject claims compatible with existing JWT claims structure.
				// project_ids is stored as []int64 (in-process, not JSON-round-tripped)
				// so downstream enforcement can type-assert directly. JWT-authenticated
				// users do not receive this claim and are therefore unrestricted.
				claims := jwt.MapClaims{
					"sub":         apiKey.Username,
					"role":        apiKey.Role,
					"auth_type":   "api_key",
					"project_ids": apiKey.ProjectIDs,
				}
				ctx := context.WithValue(r.Context(), ClaimsKey, claims)
				next(w, r.WithContext(ctx))
				return
			}

			_, claims, err := jwtManager.ValidateToken(tokenStr, expectedType)
			if err != nil {
				logging.FromContext(r.Context()).Warn("auth: token validation failed", zap.Error(err))
				writeMiddlewareError(w, http.StatusUnauthorized, "Invalid token")
				return
			}

			// F-3: re-check users.is_active for DB-backed users (numeric sub).
			// IsActive returns (true, nil) for non-numeric sub values (env
			// users), so this call is safe to invoke unconditionally.
			if userActiveCache != nil {
				sub, _ := claims["sub"].(string)
				active, recheckErr := userActiveCache.IsActive(r.Context(), sub)
				if recheckErr != nil {
					logging.FromContext(r.Context()).Warn("auth: user active recheck failed",
						zap.String("sub", sub), zap.Error(recheckErr))
				} else if !active {
					writeMiddlewareError(w, http.StatusUnauthorized, "Account inactive")
					return
				}
			}

			// Propagate claims through context so handlers can access username/role
			ctx := context.WithValue(r.Context(), ClaimsKey, claims)
			next(w, r.WithContext(ctx))
		}
	}
}

// extractToken tries to get a JWT string from the named cookie first,
// then falls back to the Authorization: Bearer header.
func extractToken(r *http.Request, cookieName string) string {
	if cookie, err := r.Cookie(cookieName); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	authHeader := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
		return after
	}
	return ""
}

// ClaimsFromContext retrieves the JWT claims stored by AuthMiddleware
func ClaimsFromContext(ctx context.Context) (jwt.MapClaims, bool) {
	claims, ok := ctx.Value(ClaimsKey).(jwt.MapClaims)
	return claims, ok
}

// roleLevel maps role names to a numeric hierarchy for comparison.
// Higher values imply more permissions. Unknown roles get level 0.
var roleLevel = map[string]int{
	"viewer": 1,
	"editor": 2,
	"admin":  3,
}

// RequireRole returns middleware that enforces a minimum role level.
// It reads the "role" claim from the JWT claims set by AuthMiddleware.
// Returns 403 Forbidden if the user's role level is below the required level.
func RequireRole(required string) func(http.HandlerFunc) http.HandlerFunc {
	requiredLevel := roleLevel[required]

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			claims, ok := ClaimsFromContext(r.Context())
			if !ok {
				writeMiddlewareError(w, http.StatusForbidden, "Access denied: missing claims")
				return
			}

			userRole, _ := claims["role"].(string)
			userLevel := roleLevel[userRole]

			if userLevel < requiredLevel {
				writeMiddlewareError(w, http.StatusForbidden, "Access denied: insufficient permissions")
				return
			}

			next(w, r)
		}
	}
}
