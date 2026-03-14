package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// OIDCHandler handles OIDC SSO login and callback flows.
type OIDCHandler struct {
	cfg        *config.Config
	oidcProv   security.OIDCExchanger
	jwtManager *security.JWTManager
	userStore  store.UserStorer
	logger     *zap.Logger
}

// NewOIDCHandler creates and returns a new OIDCHandler.
func NewOIDCHandler(cfg *config.Config, oidcProv security.OIDCExchanger, jwtManager *security.JWTManager, userStore store.UserStorer, logger *zap.Logger) *OIDCHandler {
	return &OIDCHandler{
		cfg:        cfg,
		oidcProv:   oidcProv,
		jwtManager: jwtManager,
		userStore:  userStore,
		logger:     logger,
	}
}

// Login initiates the OIDC authorization code + PKCE flow.
// GET /api/v1/auth/oidc/login
func (h *OIDCHandler) Login(w http.ResponseWriter, r *http.Request) {
	// Generate cryptographically random state (32 bytes -> hex)
	state, err := randomHex(32)
	if err != nil {
		h.logger.Error("oidc: failed to generate state", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "Failed to initiate SSO")
		return
	}

	// Generate nonce (32 bytes -> hex)
	nonce, err := randomHex(32)
	if err != nil {
		h.logger.Error("oidc: failed to generate nonce", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "Failed to initiate SSO")
		return
	}

	// Generate PKCE code verifier (32 bytes -> base64url) and S256 challenge
	codeVerifier, err := randomBase64URL(32)
	if err != nil {
		h.logger.Error("oidc: failed to generate PKCE verifier", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "Failed to initiate SSO")
		return
	}
	codeChallenge := security.PKCEChallenge(codeVerifier)

	// Encrypt state, nonce, and code verifier into a single state cookie
	cookieValue, err := security.EncodeStateCookie([]byte(h.cfg.OIDC.StateCookieSecret), state, nonce, codeVerifier)
	if err != nil {
		h.logger.Error("oidc: failed to encode state cookie", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "Failed to initiate SSO")
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "oidc_state",
		Value:    cookieValue,
		HttpOnly: true,
		Secure:   h.cfg.TLS,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300, // 5 minutes
		Path:     "/",
	})

	authURL := h.oidcProv.AuthCodeURL(state, nonce, codeChallenge)
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback handles the OIDC provider redirect after successful authentication.
// GET /api/v1/auth/oidc/callback
func (h *OIDCHandler) Callback(w http.ResponseWriter, r *http.Request) {
	// 1. Read and decrypt the state cookie
	stateCookie, err := r.Cookie("oidc_state")
	if err != nil {
		writeError(w, http.StatusBadRequest, "Missing OIDC state cookie")
		return
	}

	cookieState, nonce, codeVerifier, err := security.DecodeStateCookie([]byte(h.cfg.OIDC.StateCookieSecret), stateCookie.Value)
	if err != nil {
		h.logger.Warn("oidc: invalid state cookie", zap.Error(err))
		writeError(w, http.StatusBadRequest, "Invalid or expired OIDC state")
		return
	}

	// 2. Constant-time compare state parameter vs cookie state
	queryState := r.URL.Query().Get("state")
	if !secureCompare(queryState, cookieState) {
		h.logger.Warn("oidc: state mismatch", zap.String("query_state", queryState))
		writeError(w, http.StatusBadRequest, "OIDC state mismatch")
		return
	}

	// 3. Check for IdP error
	if errParam := r.URL.Query().Get("error"); errParam != "" {
		errDesc := r.URL.Query().Get("error_description")
		h.logger.Warn("oidc: IdP returned error", zap.String("error", errParam), zap.String("description", errDesc))
		writeError(w, http.StatusBadRequest, fmt.Sprintf("SSO error: %s", errParam))
		return
	}

	// 4. Exchange authorization code (also validates nonce and extracts claims)
	code := r.URL.Query().Get("code")
	userInfo, err := h.oidcProv.Exchange(r.Context(), code, codeVerifier, nonce)
	if err != nil {
		h.logger.Error("oidc: token exchange failed", zap.Error(err))
		writeError(w, http.StatusBadGateway, "Failed to exchange OIDC token")
		return
	}

	// 5. Resolve role from groups
	role := security.ResolveRole(userInfo.Groups, &h.cfg.OIDC)

	// 6. JIT user provisioning via UpsertByOIDC
	user, err := h.userStore.UpsertByOIDC(r.Context(), "oidc", userInfo.Subject, userInfo.Email, userInfo.Name, role)
	if err != nil {
		h.logger.Error("oidc: user upsert failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "Failed to provision user")
		return
	}

	// 7. Check if user is active
	if !user.IsActive {
		h.logger.Warn("oidc: deactivated user attempted login", zap.String("email", user.Email))
		writeError(w, http.StatusForbidden, "Account is deactivated")
		return
	}

	// 8. Generate JWT tokens with OIDC provider claim
	accessToken, refreshToken, err := h.jwtManager.GenerateTokens(user.Email, user.Role, "oidc")
	if err != nil {
		h.logger.Error("oidc: failed to generate tokens", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "Failed to generate session tokens")
		return
	}

	// 9. Generate CSRF token
	csrfToken, err := middleware.GenerateCSRFToken()
	if err != nil {
		h.logger.Error("oidc: failed to generate CSRF token", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "Failed to generate session tokens")
		return
	}

	// 10. Set auth cookies (same pattern as local login)
	http.SetCookie(w, &http.Cookie{
		Name:     "csrf_token",
		Value:    csrfToken,
		HttpOnly: false, //nolint:gosec // intentionally readable by JS for double-submit pattern
		Secure:   h.cfg.TLS,
		SameSite: http.SameSiteLaxMode,
		Path:     "/",
	})
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

	// 11. Clear the state cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oidc_state",
		Value:    "",
		HttpOnly: true,
		Secure:   h.cfg.TLS,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Path:     "/",
	})

	// 12. Redirect to post-login URL
	postLoginURL := h.cfg.OIDC.PostLoginRedirect
	if postLoginURL == "" {
		postLoginURL = "/"
	}
	http.Redirect(w, r, postLoginURL+"?oidc=success", http.StatusFound)
}

// randomHex returns n cryptographically random bytes as a lowercase hex string.
func randomHex(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// randomBase64URL returns n cryptographically random bytes as a base64url-encoded string (no padding).
func randomBase64URL(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// secureCompare compares two strings in constant time to prevent timing attacks.
func secureCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	result := 0
	for i := 0; i < len(a); i++ {
		result |= int(a[i]) ^ int(b[i])
	}
	return result == 0
}
