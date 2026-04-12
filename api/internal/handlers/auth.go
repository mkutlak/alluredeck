package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/logging"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// AuthHandler handles HTTP requests for authentication (login/logout/refresh).
type AuthHandler struct {
	cfg         *config.Config
	jwtManager  *security.JWTManager
	familyStore store.RefreshTokenFamilyStorer // optional; nil disables refresh-token rotation
}

// NewAuthHandler creates and returns a new AuthHandler. The familyStore parameter
// is optional — pass nil to disable refresh-token rotation (sessions then behave
// as before: a single 1h access token, no refresh).
func NewAuthHandler(cfg *config.Config, jwtManager *security.JWTManager, familyStore store.RefreshTokenFamilyStorer) *AuthHandler {
	return &AuthHandler{
		cfg:         cfg,
		jwtManager:  jwtManager,
		familyStore: familyStore,
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
	if !h.cfg.SecurityEnabled {
		writeError(w, http.StatusNotFound, "SECURITY is not enabled")
		return
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Missing JSON in body request")
		return
	}

	username := req.Username
	password := req.Password

	// Use constant-time comparison for usernames to prevent timing-based enumeration (AUDIT 1.5).
	// Use bcrypt for password verification (C3 fix: no more plaintext passwords).
	var roles []string
	valid := false

	adminUserMatch := subtle.ConstantTimeCompare([]byte(username), []byte(h.cfg.AdminUser)) == 1
	adminPassMatch := len(h.cfg.SecurityPassHash) > 0 &&
		bcrypt.CompareHashAndPassword(h.cfg.SecurityPassHash, []byte(password)) == nil
	if adminUserMatch && adminPassMatch {
		roles = []string{"admin"}
		valid = true
	}

	if !valid && h.cfg.ViewerUser != "" {
		viewerUserMatch := subtle.ConstantTimeCompare([]byte(username), []byte(h.cfg.ViewerUser)) == 1
		viewerPassMatch := len(h.cfg.ViewerPassHash) > 0 &&
			bcrypt.CompareHashAndPassword(h.cfg.ViewerPassHash, []byte(password)) == nil
		if viewerUserMatch && viewerPassMatch {
			roles = []string{"viewer"}
			valid = true
		}
	}

	if !valid {
		writeError(w, http.StatusUnauthorized, "Invalid username/password")
		return
	}

	familyID, err := security.NewFamilyID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate session ID")
		return
	}
	accessToken, refreshToken, _, refreshJTI, err := h.jwtManager.GenerateTokensForFamily(username, roles[0], "local", familyID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate tokens")
		return
	}

	// Persist the refresh-token family for rotation tracking. Failure here must not
	// block login — log and continue. Without a family record the user can still
	// authenticate, they just can't refresh (Refresh handler will reject and force
	// a re-login). This is no worse than today's behavior (no refresh at all).
	if h.familyStore != nil {
		if err := h.familyStore.Create(r.Context(), store.RefreshTokenFamily{
			FamilyID:   familyID,
			UserID:     username,
			Role:       roles[0],
			Provider:   "local",
			CurrentJTI: refreshJTI,
			Status:     "active",
			ExpiresAt:  time.Now().Add(h.cfg.RefreshTokenExpiry.Duration()),
		}); err != nil {
			logging.FromContext(r.Context()).Warn("auth: failed to persist refresh token family", zap.Error(err))
		}
	}

	// Generate CSRF token for double-submit cookie pattern (REVIEW #11).
	csrfToken, err := middleware.GenerateCSRFToken()
	if err != nil {
		logging.FromContext(r.Context()).Error("auth: failed to generate CSRF token", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "Failed to generate tokens")
		return
	}

	// Tokens delivered solely via httpOnly cookies — never in JSON body (M3 fix).
	response := map[string]any{
		"data": map[string]any{
			"csrf_token": csrfToken,
			"expires_in": int(h.cfg.AccessTokenExpiry.Seconds()),
			"roles":      roles,
		},
		"metadata": map[string]string{"message": "Successfully logged"},
	}

	// CSRF cookie is HttpOnly=false so JavaScript can read it (REVIEW #11).
	// jwt + refresh_jwt are HttpOnly. SameSite: Lax prevents CSRF while allowing
	// top-level navigation (AUDIT 1.1).
	setAuthCookies(w, h.cfg, accessToken, refreshToken, csrfToken)

	writeJSON(w, http.StatusOK, response)
}

// Logout godoc
// @Summary      Log out
// @Description  Revokes the current access and refresh tokens and expires auth cookies.
// @Tags         auth
// @Produce      json
// @Success      200  {object}  map[string]any
// @Router       /logout [delete]
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.SecurityEnabled {
		writeError(w, http.StatusNotFound, "SECURITY is not enabled")
		return
	}

	// Blacklist the access token JTI so it is immediately revoked (AUDIT 1.6).
	tokenStr := extractBearerToken(r)
	if tokenStr == "" {
		tokenStr = extractCookieToken(r, "jwt")
	}
	if tokenStr != "" {
		h.blacklistToken(tokenStr, "access", h.cfg.AccessTokenExpiry.Duration())
	}

	// Also blacklist the refresh token JTI so it cannot be used to mint new access
	// tokens after logout — fixes AUDIT 1.6 (incomplete token revocation).
	// If the refresh token has a `fam` claim, also revoke the family so any
	// concurrent refresh attempt from another tab fails immediately.
	refreshTokenStr := extractCookieToken(r, "refresh_jwt")
	if refreshTokenStr != "" {
		if h.familyStore != nil {
			if _, claims, err := h.jwtManager.ValidateToken(refreshTokenStr, "refresh"); err == nil {
				if famID, ok := claims["fam"].(string); ok && famID != "" {
					if revokeErr := h.familyStore.Revoke(r.Context(), famID); revokeErr != nil {
						logging.FromContext(r.Context()).Warn("auth: failed to revoke refresh token family on logout",
							zap.String("family", famID), zap.Error(revokeErr))
					}
				}
			}
		}
		h.blacklistToken(refreshTokenStr, "refresh", h.cfg.RefreshTokenExpiry.Duration())
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

	writeJSON(w, http.StatusOK, map[string]any{
		"metadata": map[string]string{"message": "Successfully logged out"},
	})
}

// Refresh godoc
// @Summary      Refresh access token
// @Description  Validates the refresh token, rotates it (OAuth 2.0 BCP), and issues a new access + refresh + csrf cookie set.
// @Tags         auth
// @Produce      json
// @Success      200  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Router       /auth/refresh [post]
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.SecurityEnabled {
		writeError(w, http.StatusNotFound, "SECURITY is not enabled")
		return
	}
	if h.familyStore == nil {
		// Refresh-token rotation not configured. The client should re-login.
		writeError(w, http.StatusUnauthorized, "Refresh not available")
		return
	}

	tokenStr := extractCookieToken(r, "refresh_jwt")
	if tokenStr == "" {
		writeError(w, http.StatusUnauthorized, "Missing refresh token")
		return
	}

	_, claims, err := h.jwtManager.ValidateToken(tokenStr, "refresh")
	if err != nil {
		writeError(w, http.StatusUnauthorized, "Invalid refresh token")
		return
	}

	famID, _ := claims["fam"].(string)
	presentedJTI, _ := claims["jti"].(string)
	if famID == "" || presentedJTI == "" {
		writeError(w, http.StatusUnauthorized, "Malformed refresh token")
		return
	}

	family, err := h.familyStore.GetByID(r.Context(), famID)
	if err != nil {
		logging.FromContext(r.Context()).Error("auth: refresh family lookup failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "Failed to load session")
		return
	}
	if family == nil {
		writeError(w, http.StatusUnauthorized, "Unknown session")
		return
	}
	if family.Status != store.RefreshTokenFamilyStatusActive {
		writeError(w, http.StatusUnauthorized, "Session no longer active")
		return
	}

	// Reuse detection: presented JTI must equal current_jti, or previous_jti
	// while still inside the grace window. Anything else is token theft.
	matchesCurrent := presentedJTI == family.CurrentJTI
	matchesPrevious := family.PreviousJTI != nil &&
		presentedJTI == *family.PreviousJTI &&
		family.GraceUntil != nil &&
		time.Now().Before(*family.GraceUntil)

	if !matchesCurrent && !matchesPrevious {
		if revokeErr := h.familyStore.MarkCompromised(r.Context(), famID); revokeErr != nil {
			logging.FromContext(r.Context()).Error("auth: failed to mark compromised family",
				zap.String("family", famID), zap.Error(revokeErr))
		}
		logging.FromContext(r.Context()).Warn("auth: refresh token reuse detected, family compromised",
			zap.String("user", family.UserID),
			zap.String("family", famID))
		writeError(w, http.StatusUnauthorized, "Session compromised")
		return
	}

	// Mint a new access + refresh pair against the same family.
	accessToken, refreshToken, _, newRefreshJTI, err := h.jwtManager.GenerateTokensForFamily(
		family.UserID, family.Role, family.Provider, famID)
	if err != nil {
		logging.FromContext(r.Context()).Error("auth: failed to mint refreshed tokens", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "Failed to generate tokens")
		return
	}

	// Atomically rotate: previous_jti = (old) current_jti, current_jti = newRefreshJTI,
	// grace_until = NOW() + 30s. The 30s grace window absorbs benign multi-tab
	// races where a second tab still holds the prior refresh cookie in flight.
	if rotErr := h.familyStore.Rotate(r.Context(), famID, newRefreshJTI, 30); rotErr != nil {
		logging.FromContext(r.Context()).Error("auth: failed to rotate refresh family",
			zap.String("family", famID), zap.Error(rotErr))
		writeError(w, http.StatusInternalServerError, "Failed to rotate session")
		return
	}

	csrfToken, err := middleware.GenerateCSRFToken()
	if err != nil {
		logging.FromContext(r.Context()).Error("auth: failed to generate CSRF token on refresh", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "Failed to generate CSRF token")
		return
	}

	setAuthCookies(w, h.cfg, accessToken, refreshToken, csrfToken)

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"csrf_token": csrfToken,
			"expires_in": int(h.cfg.AccessTokenExpiry.Seconds()),
			"roles":      []string{family.Role},
			"username":   family.UserID,
			"provider":   family.Provider,
		},
		"metadata": map[string]string{"message": "Session refreshed"},
	})
}

// setAuthCookies writes the jwt, refresh_jwt, and csrf_token cookies with the
// shared attributes used by Login, OIDC.Callback, and Refresh.
func setAuthCookies(w http.ResponseWriter, cfg *config.Config, accessToken, refreshToken, csrfToken string) {
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    csrfToken,
		HttpOnly: false, //nolint:gosec // intentionally readable by JS for double-submit pattern
		Secure:   cfg.TLS,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "jwt",
		Value:    accessToken,
		HttpOnly: true,
		Secure:   cfg.TLS,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
	http.SetCookie(w, &http.Cookie{
		Name:     "refresh_jwt",
		Value:    refreshToken,
		HttpOnly: true,
		Secure:   cfg.TLS,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
}

// Session godoc
// @Summary      Get current session info
// @Description  Returns the authenticated user's session information from JWT claims.
// @Tags         auth
// @Produce      json
// @Success      200  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Router       /auth/session [get]
func (h *AuthHandler) Session(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "No active session")
		return
	}

	username, _ := claims["sub"].(string)
	role, _ := claims["role"].(string)
	provider, _ := claims["provider"].(string)
	if provider == "" {
		provider = "local"
	}

	var roles []string
	if role != "" {
		roles = []string{role}
	}

	// Compute remaining seconds from the JWT exp claim — not the configured TTL —
	// so the client doesn't drift its expiresAt forward on each /auth/session call.
	//
	// MUST use claims.GetExpirationTime() (not a type assertion on claims["exp"])
	// because jwt.Parse stores numeric claims as float64, not *jwt.NumericDate.
	// GetExpirationTime() normalises across float64, *NumericDate, json.Number, int64.
	expiresIn := 0
	if expNum, err := claims.GetExpirationTime(); err == nil && expNum != nil {
		if remaining := time.Until(expNum.Time); remaining > 0 {
			expiresIn = int(remaining.Seconds())
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"username":   username,
			"roles":      roles,
			"expires_in": expiresIn,
			"provider":   provider,
		},
		"metadata": map[string]string{"message": "Session successfully obtained"},
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
