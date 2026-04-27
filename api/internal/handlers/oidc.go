package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// OIDCHandler handles OIDC SSO login and callback flows.
type OIDCHandler struct {
	cfg         *config.Config
	oidcProv    security.OIDCExchanger
	jwtManager  *security.JWTManager
	userStore   store.UserStorer
	familyStore store.RefreshTokenFamilyStorer // optional; nil disables refresh-token rotation
	audit       store.AuditLogger              // optional; nil disables persistent audit emission
	logger      *zap.Logger
}

// NewOIDCHandler creates and returns a new OIDCHandler. The familyStore parameter
// is optional — pass nil to disable refresh-token rotation for OIDC sessions.
func NewOIDCHandler(cfg *config.Config, oidcProv security.OIDCExchanger, jwtManager *security.JWTManager, userStore store.UserStorer, familyStore store.RefreshTokenFamilyStorer, logger *zap.Logger) *OIDCHandler {
	return &OIDCHandler{
		cfg:         cfg,
		oidcProv:    oidcProv,
		jwtManager:  jwtManager,
		userStore:   userStore,
		familyStore: familyStore,
		logger:      logger,
	}
}

// WithAuditLogger wires the persistent audit logger so OIDC success/failure
// events are recorded to the audit_log table. Nil is acceptable — emit becomes
// a no-op, matching the F-1 contract that audit must never fail the request
// it is auditing.
func (h *OIDCHandler) WithAuditLogger(a store.AuditLogger) *OIDCHandler {
	h.audit = a
	return h
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

	// 6. JIT user provisioning via UpsertByOIDC. F-5: when the partial-unique
	// email index rejects the insert because a different identity already owns
	// this email, UpsertByOIDC returns ErrEmailAlreadyLinked. We resolve that
	// either by rejecting (default) or, when the operator opted in via
	// OIDC_AUTO_LINK_BY_EMAIL AND the IdP attests email_verified, by relinking
	// the existing row to the new (provider, provider_sub).
	//
	// relinkPrev tracks the previous provider in the audit metadata of the
	// auto-link success path; it stays empty on the non-link branches.
	var relinkPrev string
	user, err := h.userStore.UpsertByOIDC(r.Context(), "oidc", userInfo.Subject, userInfo.Email, userInfo.Name, role)
	if err != nil {
		if errors.Is(err, store.ErrEmailAlreadyLinked) {
			user, relinkPrev = h.handleEmailCollision(w, r, userInfo, role)
			if user == nil {
				return
			}
		} else {
			h.logger.Error("oidc: user upsert failed", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "Failed to provision user")
			return
		}
	}

	// 7. Check if user is active
	if !user.IsActive {
		h.logger.Warn("oidc: deactivated user attempted login", zap.String("email", user.Email))
		writeError(w, http.StatusForbidden, "Account is deactivated")
		return
	}

	// 8. Generate refresh-token family ID + JWT tokens with OIDC provider claim
	familyID, err := security.NewFamilyID()
	if err != nil {
		h.logger.Error("oidc: failed to generate session ID", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "Failed to generate session tokens")
		return
	}
	accessToken, refreshToken, _, refreshJTI, err := h.jwtManager.GenerateTokensForFamily(user.Email, user.Role, "oidc", familyID)
	if err != nil {
		h.logger.Error("oidc: failed to generate tokens", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "Failed to generate session tokens")
		return
	}

	// 8b. Persist the refresh-token family for rotation tracking. Failure here
	// must not block login — log and continue. Without a family record the user
	// can still authenticate, they just can't refresh.
	if h.familyStore != nil {
		if famErr := h.familyStore.Create(r.Context(), store.RefreshTokenFamily{
			FamilyID:   familyID,
			UserID:     user.Email,
			Role:       user.Role,
			Provider:   "oidc",
			CurrentJTI: refreshJTI,
			Status:     store.RefreshTokenFamilyStatusActive,
			ExpiresAt:  time.Now().Add(h.cfg.RefreshTokenExpiry.Duration()),
		}); famErr != nil {
			h.logger.Warn("oidc: failed to persist refresh token family", zap.Error(famErr))
		}
	}

	// 9. Generate CSRF token
	csrfToken, err := middleware.GenerateCSRFToken()
	if err != nil {
		h.logger.Error("oidc: failed to generate CSRF token", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "Failed to generate session tokens")
		return
	}

	// 10. Set auth cookies (shared with local Login + Refresh).
	setAuthCookies(w, h.cfg, accessToken, refreshToken, csrfToken)

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

	// 12. Audit the successful login. Emit AFTER tokens are set so a successful
	// audit row genuinely corresponds to a session that was handed out. The
	// metadata distinguishes the normal upsert path from the F-5 auto-link path
	// so incident response can spot every cross-provider rebinding.
	successEvt := auditFromRequest(r)
	successEvt.Action = store.AuditActionLoginSuccess
	successEvt.Outcome = store.AuditOutcomeSuccess
	successEvt.ActorLabel = user.Email
	successEvt.TargetType = store.AuditTargetUser
	successEvt.TargetID = fmt.Sprintf("%d", user.ID)
	uid := user.ID
	successEvt.ActorID = &uid
	successMeta := map[string]any{
		"family_id": familyID,
		"role":      user.Role,
		"provider":  "oidc",
	}
	if relinkPrev != "" {
		successMeta["oidc_link"] = "auto"
		successMeta["verified"] = true
		successMeta["previous_provider"] = relinkPrev
		successMeta["new_provider"] = "oidc"
	}
	successEvt.Metadata = auditMetadata(successMeta)
	auditRecord(r.Context(), h.audit, successEvt)

	// 13. Redirect to post-login URL
	postLoginURL := h.cfg.OIDC.PostLoginRedirect
	if postLoginURL == "" {
		postLoginURL = "/"
	}
	http.Redirect(w, r, postLoginURL+"?oidc=success", http.StatusFound)
}

// handleEmailCollision resolves an ErrEmailAlreadyLinked from UpsertByOIDC.
//
// Default (cfg.OIDC.AutoLinkByEmail=false): respond 409 with a clear message,
// emit auth.login.failure with metadata.reason="oidc_email_collision", and
// return (nil, "") to signal the caller to bail.
//
// Auto-link enabled AND IdP marked email_verified=true: load the existing row
// by email, rebind it to the new (provider, provider_sub) via RelinkOIDC, and
// return the refreshed user plus the previous provider for audit metadata.
//
// Auto-link enabled BUT email_verified=false: same 409 + audit-failure as the
// default path. An attacker controlling a federated IdP must NOT be able to
// claim an arbitrary email and take over an account.
//
// On any internal failure (GetByEmail/RelinkOIDC error) the request is rejected
// with 409 + audit-failure to keep the operator-facing failure surface
// uniform — internal details belong in the structured log, not the response.
func (h *OIDCHandler) handleEmailCollision(w http.ResponseWriter, r *http.Request, userInfo *security.OIDCUserInfo, role string) (*store.User, string) {
	emailLower := strings.ToLower(strings.TrimSpace(userInfo.Email))

	rejectFn := func(reason string) {
		evt := auditFromRequest(r)
		evt.Action = store.AuditActionLoginFailure
		evt.Outcome = store.AuditOutcomeFailure
		evt.ActorLabel = emailLower
		evt.TargetType = store.AuditTargetUser
		evt.Metadata = auditMetadata(map[string]any{
			"reason":          reason,
			"attempted_email": emailLower,
			"provider":        "oidc",
			"provider_sub":    userInfo.Subject,
		})
		auditRecord(r.Context(), h.audit, evt)
		writeError(w, http.StatusConflict, "An account with this email already exists under a different identity. Contact an administrator to link them.")
	}

	if !h.cfg.OIDC.AutoLinkByEmail {
		h.logger.Warn("oidc: email collision rejected (auto-link disabled)",
			zap.String("email", emailLower), zap.String("subject", userInfo.Subject))
		rejectFn("oidc_email_collision")
		return nil, ""
	}

	// Auto-link path. The IdP MUST attest the email is verified, otherwise
	// any IdP that accepts arbitrary email input could claim ownership of
	// an existing account.
	if !userInfo.EmailVerified {
		h.logger.Warn("oidc: email collision rejected (auto-link enabled but email_verified=false)",
			zap.String("email", emailLower), zap.String("subject", userInfo.Subject))
		rejectFn("oidc_email_collision_unverified")
		return nil, ""
	}

	existing, err := h.userStore.GetByEmail(r.Context(), userInfo.Email)
	if err != nil || existing == nil {
		h.logger.Error("oidc: auto-link failed to load existing row",
			zap.String("email", emailLower), zap.Error(err))
		rejectFn("oidc_email_collision_lookup_failed")
		return nil, ""
	}
	prevProvider := existing.Provider
	if err := h.userStore.RelinkOIDC(r.Context(), existing.ID, "oidc", userInfo.Subject); err != nil {
		h.logger.Error("oidc: auto-link relink failed",
			zap.String("email", emailLower), zap.Int64("user_id", existing.ID), zap.Error(err))
		rejectFn("oidc_email_collision_relink_failed")
		return nil, ""
	}
	// Refresh the user view so callers see the new provider/sub and bumped
	// last_login. RelinkOIDC's UPDATE has already mutated the row server-side;
	// we only need to reflect that here.
	existing.Provider = "oidc"
	existing.ProviderSub = userInfo.Subject
	existing.Role = role
	now := time.Now()
	existing.LastLogin = &now
	existing.UpdatedAt = now
	return existing, prevProvider
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
