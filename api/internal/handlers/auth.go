package handlers

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	userStore   store.UserStorer               // optional; nil disables DB-backed local login fallback
	audit       store.AuditLogger              // optional; nil disables persistent audit emission
	throttler   *middleware.AccountThrottler   // optional; nil disables per-account brute-force throttle
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

// WithUserStore wires a UserStorer so Login can fall back to DB-backed local
// password authentication when the submitted username is not one of the
// env-configured admin/viewer users.
func (h *AuthHandler) WithUserStore(s store.UserStorer) *AuthHandler {
	h.userStore = s
	return h
}

// WithAuditLogger wires the persistent audit logger so login/logout/refresh
// events are recorded to the audit_log table. Nil is acceptable — audit emit
// becomes a no-op, matching the F-1 contract that audit must never fail the
// request it is auditing.
func (h *AuthHandler) WithAuditLogger(a store.AuditLogger) *AuthHandler {
	h.audit = a
	return h
}

// WithAccountThrottler wires a per-username brute-force throttle that
// complements the per-IP rate limit (the latter is the first-tier defence;
// this is the second tier that resists distributed credential stuffing
// where IP rotation defeats IP-keyed limits). Nil is acceptable — throttle
// becomes a no-op, preserving the pre-F-4 behaviour for callers that don't
// wire it (e.g. unit tests that don't care about lockout semantics).
func (h *AuthHandler) WithAccountThrottler(t *middleware.AccountThrottler) *AuthHandler {
	h.throttler = t
	return h
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

	// F-4: per-account brute-force throttle. The check runs BEFORE the
	// credential comparison so a locked-out account never reaches the
	// password compare (which is the expensive operation an attacker is
	// trying to time). On lockout we emit an audit event with
	// metadata.lockout=true so the SOC can correlate spraying campaigns,
	// then return 429 with Retry-After so honest clients back off.
	if h.throttler != nil {
		throttleResult := h.throttler.Check(username)
		if !throttleResult.Allowed {
			lockoutEvt := auditFromRequest(r)
			lockoutEvt.Action = store.AuditActionLoginFailure
			lockoutEvt.Outcome = store.AuditOutcomeFailure
			lockoutEvt.ActorLabel = username
			lockoutEvt.TargetType = store.AuditTargetUser
			lockoutEvt.Metadata = auditMetadata(map[string]any{
				"lockout":       true,
				"locked_until":  throttleResult.LockedUntil.UTC().Format(time.RFC3339),
				"failure_count": throttleResult.FailureCount,
			})
			auditRecord(r.Context(), h.audit, lockoutEvt)

			retryAfter := int(time.Until(throttleResult.LockedUntil).Seconds())
			if retryAfter < 1 {
				retryAfter = 1
			}
			w.Header().Set("Retry-After", strconv.Itoa(retryAfter))
			writeError(w, http.StatusTooManyRequests, "Too many failed attempts. Try again later.")
			return
		}
		// Soft/exponential delay: hold the goroutine to slow each attempt.
		// The cap is enforced by the throttler (backoffMax). Skip when zero
		// (no recorded failures yet).
		if throttleResult.Delay > 0 {
			time.Sleep(throttleResult.Delay)
		}
	}

	// Use constant-time comparison for usernames to prevent timing-based enumeration (AUDIT 1.5).
	// Use bcrypt for password verification (C3 fix: no more plaintext passwords).
	var roles []string
	valid := false
	// tokenSub is what we stamp into the JWT "sub" claim. For env-configured
	// admin/viewer it is the username (backward-compat). For DB-backed local
	// users it is the numeric user ID as string so the rest of the system can
	// look the user row up consistently.
	tokenSub := username

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

	// Fallback: DB-backed local users. The same error message is returned for
	// unknown-user, inactive, non-local-provider, and wrong-password so we do
	// not leak which of those conditions caused the rejection.
	// dbUserID is non-zero only when the DB-local branch validated the password;
	// it drives the best-effort UpdateLastLogin call below.
	var dbUserID int64
	if !valid && h.userStore != nil {
		u, lookupErr := h.userStore.GetByEmail(r.Context(), username)
		if lookupErr == nil && u != nil && u.Provider == "local" && u.IsActive && len(u.PasswordHash) > 0 {
			if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) == nil {
				roles = []string{u.Role}
				tokenSub = strconv.FormatInt(u.ID, 10)
				dbUserID = u.ID
				valid = true
			}
		} else if lookupErr != nil && !errors.Is(lookupErr, store.ErrUserNotFound) {
			logging.FromContext(r.Context()).Error("auth: user lookup failed", zap.Error(lookupErr))
		}
	}

	if !valid {
		// F-4: increment the per-username failure counter. If this failure
		// crossed the lockout threshold, the result reports Allowed=false and
		// we surface that on the audit metadata so the SOC can pinpoint the
		// transition. The 401 response stays identical regardless — we never
		// leak lockout state to the unauthenticated client (that would be a
		// signal an attacker could probe to map account states).
		failureMeta := map[string]any{}
		if h.throttler != nil {
			res := h.throttler.RecordFailure(username)
			failureMeta["failure_count"] = res.FailureCount
			if !res.Allowed {
				failureMeta["lockout"] = true
				failureMeta["locked_until"] = res.LockedUntil.UTC().Format(time.RFC3339)
			}
		}

		// Audit the failure BEFORE returning so a 401 response always corresponds
		// to a row in audit_log. ActorID stays nil because the credential
		// rejection happens before any user lookup succeeds.
		failureEvt := auditFromRequest(r)
		failureEvt.Action = store.AuditActionLoginFailure
		failureEvt.Outcome = store.AuditOutcomeFailure
		failureEvt.ActorLabel = username
		failureEvt.TargetType = store.AuditTargetUser
		if len(failureMeta) > 0 {
			failureEvt.Metadata = auditMetadata(failureMeta)
		}
		auditRecord(r.Context(), h.audit, failureEvt)

		writeError(w, http.StatusUnauthorized, "Invalid username/password")
		return
	}

	// F-4: credentials checked out — clear the per-username failure counter
	// so a legitimate user is not punished by their earlier typos.
	if h.throttler != nil {
		h.throttler.RecordSuccess(username)
	}

	// Best-effort: refresh last_login for DB-backed local users. Env admin/viewer
	// don't have a users row (dbUserID == 0) and OIDC users are handled via
	// UpsertByOIDC. A failure here must not fail the login — the credential
	// check already succeeded.
	if dbUserID != 0 && h.userStore != nil {
		if err := h.userStore.UpdateLastLogin(r.Context(), dbUserID); err != nil {
			logging.FromContext(r.Context()).Warn("auth: failed to update last_login",
				zap.Int64("user_id", dbUserID), zap.Error(err))
		}
	}

	familyID, err := security.NewFamilyID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate session ID")
		return
	}
	accessToken, refreshToken, _, refreshJTI, err := h.jwtManager.GenerateTokensForFamily(tokenSub, roles[0], "local", familyID)
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
			UserID:     tokenSub,
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

	// Audit the successful login. Emit AFTER tokens are set so a successful
	// audit row genuinely corresponds to a session that was handed out.
	successEvt := auditFromRequest(r)
	successEvt.Action = store.AuditActionLoginSuccess
	successEvt.Outcome = store.AuditOutcomeSuccess
	successEvt.ActorLabel = username
	successEvt.TargetType = store.AuditTargetUser
	successEvt.TargetID = tokenSub
	if dbUserID != 0 {
		id := dbUserID
		successEvt.ActorID = &id
	}
	successEvt.Metadata = auditMetadata(map[string]any{
		"family_id": familyID,
		"role":      roles[0],
	})
	auditRecord(r.Context(), h.audit, successEvt)

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
		blacklistAccessToken(h.jwtManager, tokenStr, "access", h.cfg.AccessTokenExpiry.Duration())
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
		blacklistAccessToken(h.jwtManager, refreshTokenStr, "refresh", h.cfg.RefreshTokenExpiry.Duration())
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

	// Audit the logout. ActorID is populated when the access token's sub is a
	// numeric users.id; for env-admin / env-viewer it stays nil and the
	// subject lives in actor_label.
	logoutEvt := auditFromRequest(r)
	logoutEvt.Action = store.AuditActionLogout
	logoutEvt.Outcome = store.AuditOutcomeSuccess
	logoutEvt.TargetType = store.AuditTargetSession
	if claims, ok := middleware.ClaimsFromContext(r.Context()); ok {
		if sub, _ := claims["sub"].(string); sub != "" {
			logoutEvt.ActorLabel = sub
			logoutEvt.TargetID = sub
			if id, parseErr := strconv.ParseInt(sub, 10, 64); parseErr == nil {
				logoutEvt.ActorID = &id
			}
		}
	}
	auditRecord(r.Context(), h.audit, logoutEvt)

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

		// Audit token-theft detection. This is THE single most important event in
		// the audit log — incident responders filter on this action first.
		compromiseEvt := auditFromRequest(r)
		compromiseEvt.Action = store.AuditActionRefreshCompromise
		compromiseEvt.Outcome = store.AuditOutcomeFailure
		compromiseEvt.ActorLabel = family.UserID
		compromiseEvt.TargetType = store.AuditTargetSession
		compromiseEvt.TargetID = famID
		if id, parseErr := strconv.ParseInt(family.UserID, 10, 64); parseErr == nil {
			compromiseEvt.ActorID = &id
		}
		compromiseEvt.Metadata = auditMetadata(map[string]any{
			"presented_jti": presentedJTI,
		})
		auditRecord(r.Context(), h.audit, compromiseEvt)

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

	// Audit the successful refresh. Volume here can be high (every UI tab
	// refreshes its access token periodically) but it is essential to detect
	// stolen-but-still-valid families.
	refreshEvt := auditFromRequest(r)
	refreshEvt.Action = store.AuditActionRefreshSuccess
	refreshEvt.Outcome = store.AuditOutcomeSuccess
	refreshEvt.ActorLabel = family.UserID
	refreshEvt.TargetType = store.AuditTargetSession
	refreshEvt.TargetID = famID
	if id, parseErr := strconv.ParseInt(family.UserID, 10, 64); parseErr == nil {
		refreshEvt.ActorID = &id
	}
	auditRecord(r.Context(), h.audit, refreshEvt)

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

	sub, _ := claims["sub"].(string)
	role, _ := claims["role"].(string)
	provider, _ := claims["provider"].(string)
	if provider == "" {
		provider = "local"
	}

	// Prefer a human-readable username for the UI. For DB-backed users (sub is
	// a numeric id) resolve the email so the header doesn't display "1". For
	// env admin/viewer (sub == "admin"/"viewer") and OIDC (sub == email) echo
	// the subject verbatim.
	username := sub
	if id, err := strconv.ParseInt(sub, 10, 64); err == nil && h.userStore != nil {
		if u, lookupErr := h.userStore.GetByID(r.Context(), id); lookupErr == nil && u != nil {
			username = u.Email
		} else if lookupErr != nil && !errors.Is(lookupErr, store.ErrUserNotFound) {
			logging.FromContext(r.Context()).Warn("auth: session user lookup failed",
				zap.Int64("user_id", id), zap.Error(lookupErr))
		}
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
