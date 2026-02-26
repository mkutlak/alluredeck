package middleware

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/security"
)

type contextKey string

// ClaimsKey is the context key used to store JWT claims for downstream handlers
const ClaimsKey contextKey = "jwt_claims"

// AuthMiddleware protects routes using the JWT manager.
// When security is disabled, the request passes through without validation.
// On success, the parsed JWT claims are stored in the request context under ClaimsKey.
func AuthMiddleware(cfg *config.Config, jwtManager *security.JWTManager, isRefresh bool) func(http.HandlerFunc) http.HandlerFunc {
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
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"meta_data": map[string]string{"message": "Missing authorization token"},
				})
				return
			}

			_, claims, err := jwtManager.ValidateToken(tokenStr, expectedType)
			if err != nil {
				log.Printf("auth: token validation failed: %v", err)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"meta_data": map[string]string{"message": "Invalid token"},
				})
				return
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
	"admin":  2,
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
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"meta_data": map[string]string{"message": "Access denied: missing claims"},
				})
				return
			}

			userRole, _ := claims["role"].(string)
			userLevel := roleLevel[userRole]

			if userLevel < requiredLevel {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"meta_data": map[string]string{"message": "Access denied: insufficient permissions"},
				})
				return
			}

			next(w, r)
		}
	}
}
