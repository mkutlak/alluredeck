package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"

	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/config"
	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/security"
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
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"meta_data": map[string]string{"message": "Invalid token: " + err.Error()},
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
