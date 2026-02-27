package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/security"
)

// AuthHandler handles HTTP requests for authentication (login/logout).
type AuthHandler struct {
	cfg        *config.Config
	jwtManager *security.JWTManager
}

// NewAuthHandler creates and returns a new AuthHandler.
func NewAuthHandler(cfg *config.Config, jwtManager *security.JWTManager) *AuthHandler {
	return &AuthHandler{
		cfg:        cfg,
		jwtManager: jwtManager,
	}
}

// LoginRequest structure
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"` //nolint:gosec // G117: field name is intentional for login request struct
}

// Login godoc
// @Summary      Authenticate user
// @Description  Validates credentials and issues JWT access and refresh tokens.
// @Tags         auth
// @Accept       json
// @Produce      json
// @Param        body  body      LoginRequest  true  "Login credentials"
// @Success      200   {object}  map[string]any
// @Failure      400   {object}  map[string]any
// @Failure      401   {object}  map[string]any
// @Router       /login [post]
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if !h.cfg.SecurityEnabled {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]string{"message": "SECURITY is not enabled"},
		})
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]string{"message": "Missing JSON in body request"},
		})
		return
	}

	username := req.Username
	password := req.Password

	// Use constant-time comparison to prevent timing attacks (AUDIT 1.5).
	// Evaluate all comparisons with bitwise AND to avoid short-circuit leaks.
	var roles []string
	valid := false

	adminUserMatch := subtle.ConstantTimeCompare([]byte(username), []byte(h.cfg.SecurityUser))
	adminPassMatch := subtle.ConstantTimeCompare([]byte(password), []byte(h.cfg.SecurityPass))
	if adminUserMatch&adminPassMatch == 1 {
		roles = []string{"admin"}
		valid = true
	}

	if !valid && h.cfg.ViewerUser != "" {
		viewerUserMatch := subtle.ConstantTimeCompare([]byte(username), []byte(h.cfg.ViewerUser))
		viewerPassMatch := subtle.ConstantTimeCompare([]byte(password), []byte(h.cfg.ViewerPass))
		if viewerUserMatch&viewerPassMatch == 1 {
			roles = []string{"viewer"}
			valid = true
		}
	}

	if !valid {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]string{"message": "Invalid username/password"},
		})
		return
	}

	accessToken, refreshToken, err := h.jwtManager.GenerateTokens(username, roles[0])
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]string{"message": "Failed to generate tokens"},
		})
		return
	}

	// Generate CSRF token for double-submit cookie pattern (REVIEW #11).
	csrfToken, err := middleware.GenerateCSRFToken()
	if err != nil {
		log.Printf("auth: failed to generate CSRF token: %v", err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]string{"message": "Failed to generate tokens"},
		})
		return
	}

	response := map[string]any{
		"data": map[string]any{
			"access_token":  accessToken,
			"refresh_token": refreshToken,
			"csrf_token":    csrfToken,
			"expires_in":    int(h.cfg.AccessTokenExpiry.Seconds()),
			"roles":         roles,
		},
		"metadata": map[string]string{"message": "Successfully logged"},
	}

	// CSRF cookie — HttpOnly=false so JavaScript can read it (REVIEW #11).
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    csrfToken,
		HttpOnly: false, //nolint:gosec // intentionally readable by JS for double-submit pattern
		Secure:   h.cfg.TLS,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	// SameSite: Lax prevents CSRF while allowing top-level navigation (AUDIT 1.1).
	http.SetCookie(w, &http.Cookie{
		Name:     "jwt",
		Value:    accessToken,
		HttpOnly: true,
		Secure:   h.cfg.TLS,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_jwt",
		Value:    refreshToken,
		HttpOnly: true,
		Secure:   h.cfg.TLS,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// Logout godoc
// @Summary      Log out
// @Description  Revokes the current access and refresh tokens and expires auth cookies.
// @Tags         auth
// @Produce      json
// @Success      200  {object}  map[string]any
// @Router       /logout [delete]
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if !h.cfg.SecurityEnabled {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"metadata": map[string]string{"message": "SECURITY is not enabled"},
		})
		return
	}

	// Blacklist the access token JTI so it is immediately revoked (AUDIT 1.6).
	tokenStr := extractBearerToken(r)
	if tokenStr == "" {
		tokenStr = extractCookieToken(r, "jwt")
	}
	if tokenStr != "" {
		h.blacklistToken(tokenStr, "access", h.cfg.AccessTokenExpiry)
	}

	// Also blacklist the refresh token JTI so it cannot be used to mint new access
	// tokens after logout — fixes AUDIT 1.6 (incomplete token revocation).
	refreshTokenStr := extractCookieToken(r, "refresh_jwt")
	if refreshTokenStr != "" {
		h.blacklistToken(refreshTokenStr, "refresh", h.cfg.RefreshTokenExpiry)
	}

	// Expire all auth cookies with SameSite: Lax (AUDIT 1.1, REVIEW #11).
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    "",
		MaxAge:   -1,
		HttpOnly: false,
		Secure:   h.cfg.TLS,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "jwt",
		Value:    "",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cfg.TLS,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_jwt",
		Value:    "",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   h.cfg.TLS,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"metadata": map[string]string{"message": "Successfully logged out"},
	})
}

func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if after, ok := strings.CutPrefix(authHeader, "Bearer "); ok {
		return after
	}
	return ""
}

// extractCookieToken extracts the JWT from the named cookie.
// Returns the token string, or empty string if absent.
func extractCookieToken(r *http.Request, name string) string {
	cookie, err := r.Cookie(name)
	if err != nil || cookie.Value == "" {
		return ""
	}
	return cookie.Value
}

// blacklistToken validates tokenStr as expectedType and adds its JTI to the blacklist.
// defaultExpiry is used when the token's exp claim is missing.
func (h *AuthHandler) blacklistToken(tokenStr, expectedType string, defaultExpiry time.Duration) {
	_, claims, err := h.jwtManager.ValidateToken(tokenStr, expectedType)
	if err != nil {
		return
	}
	jti, ok := claims["jti"].(string)
	if !ok || jti == "" {
		return
	}
	expiry := time.Now().Add(defaultExpiry)
	if exp, ok := claims["exp"].(*jwt.NumericDate); ok {
		expiry = exp.Time
	}
	h.jwtManager.AddToBlacklist(jti, expiry)
}
