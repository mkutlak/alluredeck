package handlers

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// bcryptCostUsers matches the cost used by existing admin/viewer credential
// hashing (bcrypt.DefaultCost == 10). The plan references cost 12 but the rest
// of the codebase uses DefaultCost; the trade-off is resource usage during
// tests where 12 noticeably slows down large suites. Using 12 explicitly as
// requested by the plan.
const bcryptCostUsers = 12

const (
	maxUserNameLen   = 120
	minPasswordLen   = 12
	tempPasswordSize = 18 // bytes -> 24 chars base64 (>= 12-char policy)
)

// validRoles lists the roles acceptable via API input.
var validRoles = map[string]struct{}{
	"admin":  {},
	"editor": {},
	"viewer": {},
}

// UserHandler handles CRUD operations for user accounts.
type UserHandler struct {
	store      store.UserStorer
	logger     *zap.Logger
	audit      store.AuditLogger              // optional; nil disables persistent audit emission
	families   store.RefreshTokenFamilyStorer // optional; nil disables session revocation on password change/reset/deactivate
	apiKeys    store.APIKeyStorer             // optional; nil disables API-key cascade-delete on password reset/deactivate
	jwtManager *security.JWTManager           // optional; nil disables access-JTI blacklisting on self password change
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(s store.UserStorer, logger *zap.Logger) *UserHandler {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &UserHandler{store: s, logger: logger}
}

// WithAuditLogger wires the persistent audit logger so user-lifecycle events
// (create, role/active updates, delete, password change/reset) are recorded
// to the audit_log table. Nil is acceptable — audit becomes a no-op.
func (h *UserHandler) WithAuditLogger(a store.AuditLogger) *UserHandler {
	h.audit = a
	return h
}

// WithFamilyStore wires the refresh-token family store so password change,
// password reset, and account deactivation can revoke every live session for
// the affected user (F-2). Nil is acceptable — bulk revocation becomes a
// no-op and the handler logs that revocation was skipped.
func (h *UserHandler) WithFamilyStore(f store.RefreshTokenFamilyStorer) *UserHandler {
	h.families = f
	return h
}

// WithAPIKeyStore wires the API-key store so password reset and account
// deactivation can cascade-delete every API key owned by the affected user
// (F-2). Nil is acceptable — cascade becomes a no-op.
func (h *UserHandler) WithAPIKeyStore(k store.APIKeyStorer) *UserHandler {
	h.apiKeys = k
	return h
}

// WithJWTManager wires the JWT manager so ChangeMyPassword can blacklist the
// actor's current access JTI on the same request that rotated their password
// (F-2). Nil is acceptable — blacklisting becomes a no-op and the access
// token continues to be honoured until natural expiry.
func (h *UserHandler) WithJWTManager(m *security.JWTManager) *UserHandler {
	h.jwtManager = m
	return h
}

// userResponse mirrors the public JSON representation (PasswordHash excluded
// via json:"-" on the store type — but we still project explicitly to
// insulate the wire shape from future struct additions).
type userResponse struct {
	ID        int64   `json:"id"`
	Email     string  `json:"email"`
	Name      string  `json:"name"`
	Provider  string  `json:"provider"`
	Role      string  `json:"role"`
	IsActive  bool    `json:"is_active"`
	LastLogin *string `json:"last_login"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

func userToResponse(u *store.User) userResponse {
	resp := userResponse{
		ID:        u.ID,
		Email:     u.Email,
		Name:      u.Name,
		Provider:  u.Provider,
		Role:      u.Role,
		IsActive:  u.IsActive,
		CreatedAt: u.CreatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
		UpdatedAt: u.UpdatedAt.UTC().Format("2006-01-02T15:04:05.000Z"),
	}
	if u.LastLogin != nil {
		s := u.LastLogin.UTC().Format("2006-01-02T15:04:05.000Z")
		resp.LastLogin = &s
	}
	return resp
}

// userListResponse is the shape returned by GET /users.
type userListResponse struct {
	Users  []userResponse `json:"users"`
	Total  int            `json:"total"`
	Limit  int            `json:"limit"`
	Offset int            `json:"offset"`
}

// createUserResponse is returned by POST /users and carries the one-time
// generated temporary password.
type createUserResponse struct {
	User         userResponse `json:"user"`
	TempPassword string       `json:"temp_password"`
}

func claimsUserID(r *http.Request) (int64, bool) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		return 0, false
	}
	sub, _ := claims["sub"].(string)
	if sub == "" {
		return 0, false
	}
	id, err := strconv.ParseInt(sub, 10, 64)
	if err != nil {
		return 0, false
	}
	return id, true
}

// claimsSubject returns the JWT sub and role claims. The bool is false when
// claims are missing entirely (unauthenticated request).
func claimsSubject(r *http.Request) (sub, role string, ok bool) {
	claims, found := middleware.ClaimsFromContext(r.Context())
	if !found {
		return "", "", false
	}
	sub, _ = claims["sub"].(string)
	role, _ = claims["role"].(string)
	return sub, role, sub != ""
}

// envUserProfile builds a synthetic store.User for env-configured admin/viewer
// accounts whose JWT sub is not a numeric DB id (e.g. "admin", "viewer").
// Provider is set to "env" so the UI can distinguish these from "local"
// password-managed users (which support profile updates) and avoid surfacing
// DB-only affordances.
func envUserProfile(sub, role string) *store.User {
	return &store.User{
		ID:       0, // sentinel: not a DB row
		Email:    "",
		Name:     sub,
		Provider: "env",
		Role:     role,
		IsActive: true,
	}
}

// isValidEmail applies a pragmatic validation: non-empty, bounded length,
// exactly one '@', at least one '.' in the domain part, and no whitespace.
// We deliberately avoid net/mail.ParseAddress because it accepts display-name
// forms ("Name <addr>") that we don't want on this surface.
func isValidEmail(email string) bool {
	if email == "" || len(email) > 254 {
		return false
	}
	if strings.ContainsAny(email, " \t\n\r") {
		return false
	}
	at := strings.IndexByte(email, '@')
	if at <= 0 || at == len(email)-1 {
		return false
	}
	// Only one '@' allowed.
	if strings.IndexByte(email[at+1:], '@') != -1 {
		return false
	}
	domain := email[at+1:]
	if !strings.Contains(domain, ".") {
		return false
	}
	// Domain must not start/end with a dot and the final TLD must be >=2 chars.
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return false
	}
	lastDot := strings.LastIndex(domain, ".")
	if lastDot == -1 || len(domain)-lastDot-1 < 2 {
		return false
	}
	return true
}

func generateTempPassword() (string, error) {
	b := make([]byte, tempPasswordSize)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// URL-safe base64 without padding yields ~24 printable chars for 18 bytes.
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Me godoc
// @Summary      Get the authenticated user's profile
// @Description  Returns the current user's profile loaded from the users table.
// @Tags         users
// @Produce      json
// @Success      200  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /users/me [get]
func (h *UserHandler) Me(w http.ResponseWriter, r *http.Request) {
	sub, role, ok := claimsSubject(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing or invalid claims")
		return
	}
	// Env-configured admin/viewer (and any legacy JWT whose sub is not numeric)
	// do not have a users row. Return a synthetic, safe-to-render profile so the
	// UI AuthGuard can render Profile without triggering a 401→refresh loop.
	id, parseErr := strconv.ParseInt(sub, 10, 64)
	if parseErr != nil {
		writeSuccess(w, http.StatusOK, userToResponse(envUserProfile(sub, role)), "profile retrieved")
		return
	}
	u, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.logger.Error("users: get self failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error loading user")
		return
	}
	writeSuccess(w, http.StatusOK, userToResponse(u), "profile retrieved")
}

// UpdateMe godoc
// @Summary      Update the authenticated user's profile
// @Description  Updates the display name for the current user.
// @Tags         users
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /users/me [patch]
func (h *UserHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	sub, _, ok := claimsSubject(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing or invalid claims")
		return
	}
	id, parseErr := strconv.ParseInt(sub, 10, 64)
	if parseErr != nil {
		// Env-configured admin/viewer have no users row to update; refuse with a
		// clear message instead of silently succeeding.
		writeError(w, http.StatusForbidden, "Environment-configured accounts cannot update profile")
		return
	}
	var req struct {
		Name *string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == nil {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	name := strings.TrimSpace(*req.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, "name must not be empty")
		return
	}
	if len(name) > maxUserNameLen {
		writeError(w, http.StatusBadRequest, "name must not exceed 120 characters")
		return
	}
	if err := h.store.UpdateProfile(r.Context(), id, name); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.logger.Error("users: update self profile failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error updating profile")
		return
	}
	u, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.logger.Error("users: reload after update failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error loading user")
		return
	}
	writeSuccess(w, http.StatusOK, userToResponse(u), "profile updated")
}

// List godoc
// @Summary      List users
// @Description  Admin-only paginated list of users with search and role/active filters.
// @Tags         users
// @Produce      json
// @Param        limit   query  int     false  "Page size (default 20, max 100)"
// @Param        offset  query  int     false  "Offset (default 0)"
// @Param        search  query  string  false  "Substring match on email or name"
// @Param        role    query  string  false  "Filter by role (admin|editor|viewer)"
// @Param        active  query  bool    false  "Filter by is_active"
// @Success      200  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Failure      403  {object}  map[string]any
// @Router       /users [get]
func (h *UserHandler) List(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	limit := queryInt(r, "limit", 20)
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	offset := max(queryInt(r, "offset", 0), 0)

	params := store.ListUsersParams{
		Limit:  limit,
		Offset: offset,
		Search: q.Get("search"),
		Role:   q.Get("role"),
	}
	if role := strings.TrimSpace(params.Role); role != "" {
		if _, ok := validRoles[role]; !ok {
			writeError(w, http.StatusBadRequest, "role must be one of admin, editor, viewer")
			return
		}
	}
	if v := q.Get("active"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "active must be a boolean")
			return
		}
		params.Active = &b
	}

	users, total, err := h.store.ListPaginated(r.Context(), params)
	if err != nil {
		h.logger.Error("users: list paginated failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error listing users")
		return
	}
	resp := userListResponse{
		Users:  make([]userResponse, 0, len(users)),
		Total:  total,
		Limit:  limit,
		Offset: offset,
	}
	for i := range users {
		resp.Users = append(resp.Users, userToResponse(&users[i]))
	}
	writeSuccess(w, http.StatusOK, resp, "users retrieved")
}

// Get godoc
// @Summary      Get a user by ID
// @Description  Admin-only lookup by primary key.
// @Tags         users
// @Produce      json
// @Param        id  path  int  true  "User ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Failure      403  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /users/{id} [get]
func (h *UserHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	u, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.logger.Error("users: get by id failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error loading user")
		return
	}
	writeSuccess(w, http.StatusOK, userToResponse(u), "user retrieved")
}

// Create godoc
// @Summary      Create a local user
// @Description  Admin-only. Generates a one-time temporary password returned in the response.
// @Tags         users
// @Accept       json
// @Produce      json
// @Success      201  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Failure      403  {object}  map[string]any
// @Failure      409  {object}  map[string]any
// @Router       /users [post]
func (h *UserHandler) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
		Name  string `json:"name"`
		Role  string `json:"role"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	email := strings.TrimSpace(strings.ToLower(req.Email))
	name := strings.TrimSpace(req.Name)
	role := strings.TrimSpace(req.Role)

	if !isValidEmail(email) {
		writeError(w, http.StatusBadRequest, "email must be a valid RFC 5322 address")
		return
	}
	if name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(name) > maxUserNameLen {
		writeError(w, http.StatusBadRequest, "name must not exceed 120 characters")
		return
	}
	if _, ok := validRoles[role]; !ok {
		writeError(w, http.StatusBadRequest, "role must be one of admin, editor, viewer")
		return
	}

	tempPassword, err := generateTempPassword()
	if err != nil {
		h.logger.Error("users: generate temp password failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error generating password")
		return
	}
	// Defence-in-depth: ensure the generated password satisfies the same
	// minimum-length policy we enforce on user-supplied values.
	if len(tempPassword) < minPasswordLen {
		writeError(w, http.StatusInternalServerError, "error generating password")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(tempPassword), bcryptCostUsers)
	if err != nil {
		h.logger.Error("users: bcrypt hash failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error hashing password")
		return
	}
	u, err := h.store.CreateLocal(r.Context(), email, name, string(hash), role)
	if err != nil {
		if errors.Is(err, store.ErrDuplicateEntry) {
			writeError(w, http.StatusConflict, "user with this email already exists")
			return
		}
		h.logger.Error("users: create local failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error creating user")
		return
	}

	// Audit user creation. Actor identity comes from the calling admin's JWT
	// claims; the new user's id is the audit target.
	evt := auditFromRequest(r)
	evt.Action = store.AuditActionUserCreate
	evt.Outcome = store.AuditOutcomeSuccess
	evt.TargetType = store.AuditTargetUser
	evt.TargetID = strconv.FormatInt(u.ID, 10)
	if actorID, ok := claimsUserID(r); ok {
		id := actorID
		evt.ActorID = &id
	}
	if actorSub, _, ok := claimsSubject(r); ok {
		evt.ActorLabel = actorSub
	}
	evt.Metadata = auditMetadata(map[string]any{
		"new_user_email": email,
		"role":           role,
	})
	auditRecord(r.Context(), h.audit, evt)

	writeSuccess(w, http.StatusCreated, createUserResponse{
		User:         userToResponse(u),
		TempPassword: tempPassword,
	}, "user created. Copy the temporary password — it won't be shown again.")
}

// patchUserRequest is the PATCH /users/{id} body. Both fields are optional.
type patchUserRequest struct {
	Role   *string `json:"role,omitempty"`
	Active *bool   `json:"active,omitempty"`
}

// Update godoc
// @Summary      Update a user's role or active state
// @Description  Admin-only. Allows changing role and/or is_active. Rejects
//
//	self-deactivation to prevent admin lockout.
//
// @Tags         users
// @Accept       json
// @Produce      json
// @Param        id  path  int  true  "User ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Failure      403  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Failure      422  {object}  map[string]any
// @Router       /users/{id} [patch]
func (h *UserHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req patchUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Role == nil && req.Active == nil {
		writeError(w, http.StatusBadRequest, "at least one of role or active is required")
		return
	}

	if req.Role != nil {
		if _, ok := validRoles[*req.Role]; !ok {
			writeError(w, http.StatusBadRequest, "role must be one of admin, editor, viewer")
			return
		}
		if err := h.store.UpdateRole(r.Context(), id, *req.Role); err != nil {
			if errors.Is(err, store.ErrUserNotFound) {
				writeError(w, http.StatusNotFound, "user not found")
				return
			}
			h.logger.Error("users: update role failed", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "error updating role")
			return
		}
		// Emit role-change audit immediately after the storage write so a
		// successful audit row genuinely reflects a persisted role change.
		evt := auditFromRequest(r)
		evt.Action = store.AuditActionUserUpdateRole
		evt.Outcome = store.AuditOutcomeSuccess
		evt.TargetType = store.AuditTargetUser
		evt.TargetID = strconv.FormatInt(id, 10)
		if actorID, ok := claimsUserID(r); ok {
			a := actorID
			evt.ActorID = &a
		}
		if actorSub, _, ok := claimsSubject(r); ok {
			evt.ActorLabel = actorSub
		}
		evt.Metadata = auditMetadata(map[string]any{"new_role": *req.Role})
		auditRecord(r.Context(), h.audit, evt)
	}

	if req.Active != nil {
		// Self-deactivation guard: compare id to JWT sub.
		if !*req.Active {
			if selfID, ok := claimsUserID(r); ok && selfID == id {
				writeError(w, http.StatusUnprocessableEntity, "cannot deactivate your own account")
				return
			}
		}
		if err := h.store.UpdateActive(r.Context(), id, *req.Active); err != nil {
			if errors.Is(err, store.ErrUserNotFound) {
				writeError(w, http.StatusNotFound, "user not found")
				return
			}
			h.logger.Error("users: update active failed", zap.Error(err))
			writeError(w, http.StatusInternalServerError, "error updating active")
			return
		}
		evt := auditFromRequest(r)
		evt.Action = store.AuditActionUserUpdateActive
		evt.Outcome = store.AuditOutcomeSuccess
		evt.TargetType = store.AuditTargetUser
		evt.TargetID = strconv.FormatInt(id, 10)
		if actorID, ok := claimsUserID(r); ok {
			a := actorID
			evt.ActorID = &a
		}
		if actorSub, _, ok := claimsSubject(r); ok {
			evt.ActorLabel = actorSub
		}
		evt.Metadata = auditMetadata(map[string]any{"active": *req.Active})
		auditRecord(r.Context(), h.audit, evt)
	}

	u, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.logger.Error("users: reload after update failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error loading user")
		return
	}
	writeSuccess(w, http.StatusOK, userToResponse(u), "user updated")
}

// revokeAllFamilies is a best-effort wrapper that revokes every active refresh
// family for sub. The trigger and target (when distinct from the actor) are
// included in the audit metadata so incident response can correlate the event
// to the originating request. Failures are logged at warn level and never
// propagated — the surrounding handler must complete its successful response.
func (h *UserHandler) revokeAllFamilies(ctx context.Context, r *http.Request, sub, trigger string, targetID int64) {
	if h.families == nil {
		return
	}
	revoked, err := h.families.RevokeAllForUser(ctx, sub)
	if err != nil {
		h.logger.Warn("users: revoke refresh families failed",
			zap.String("trigger", trigger), zap.String("sub", sub), zap.Error(err))
		return
	}
	if h.audit == nil {
		return
	}
	evt := auditFromRequest(r)
	evt.Action = store.AuditActionSessionRevokeAll
	evt.Outcome = store.AuditOutcomeSuccess
	evt.TargetType = store.AuditTargetSession
	evt.TargetID = sub
	if actorID, ok := claimsUserID(r); ok {
		a := actorID
		evt.ActorID = &a
	}
	if actorSub, _, ok := claimsSubject(r); ok {
		evt.ActorLabel = actorSub
	}
	meta := map[string]any{
		"trigger": trigger,
		"revoked": revoked,
	}
	if targetID != 0 {
		meta["target_id"] = targetID
	}
	evt.Metadata = auditMetadata(meta)
	auditRecord(ctx, h.audit, evt)
}

// cascadeDeleteAPIKeys is a best-effort wrapper that hard-deletes every API
// key owned by username. Failures are logged at warn level and never
// propagated. Trigger is recorded in the audit metadata for forensics.
func (h *UserHandler) cascadeDeleteAPIKeys(ctx context.Context, r *http.Request, username, trigger string, targetID int64) {
	if h.apiKeys == nil || username == "" {
		return
	}
	deleted, err := h.apiKeys.DeleteAllForUser(ctx, username)
	if err != nil {
		h.logger.Warn("users: cascade-delete api keys failed",
			zap.String("trigger", trigger), zap.String("username", username), zap.Error(err))
		return
	}
	if h.audit == nil {
		return
	}
	evt := auditFromRequest(r)
	evt.Action = store.AuditActionAPIKeyCascadeDelete
	evt.Outcome = store.AuditOutcomeSuccess
	evt.TargetType = store.AuditTargetAPIKey
	evt.TargetID = strconv.FormatInt(targetID, 10)
	if actorID, ok := claimsUserID(r); ok {
		a := actorID
		evt.ActorID = &a
	}
	if actorSub, _, ok := claimsSubject(r); ok {
		evt.ActorLabel = actorSub
	}
	evt.Metadata = auditMetadata(map[string]any{
		"trigger":  trigger,
		"username": username,
		"deleted":  deleted,
	})
	auditRecord(ctx, h.audit, evt)
}

// blacklistCurrentAccessToken extracts the access token from the current
// request (Bearer header preferred, jwt cookie fallback) and adds its JTI to
// the persistent blacklist. Best-effort: silently no-ops when the JWT manager
// is unwired or the token cannot be parsed.
func (h *UserHandler) blacklistCurrentAccessToken(r *http.Request) {
	if h.jwtManager == nil {
		return
	}
	tokenStr := extractBearerToken(r)
	if tokenStr == "" {
		tokenStr = extractCookieToken(r, "jwt")
	}
	blacklistAccessToken(h.jwtManager, tokenStr, "access", 0)
}

// ChangeMyPassword godoc
// @Summary      Change the authenticated user's password
// @Description  Self-service password rotation for local (password-based)
//
//	accounts. Requires the current password for re-authentication.
//	Environment-configured admin/viewer and OIDC users are rejected.
//
// @Tags         users
// @Accept       json
// @Produce      json
// @Success      204  "no content"
// @Failure      400  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Failure      403  {object}  map[string]any
// @Failure      422  {object}  map[string]any
// @Router       /users/me/password [post]
func (h *UserHandler) ChangeMyPassword(w http.ResponseWriter, r *http.Request) {
	sub, _, ok := claimsSubject(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing or invalid claims")
		return
	}
	id, parseErr := strconv.ParseInt(sub, 10, 64)
	if parseErr != nil {
		// Env-configured admin/viewer have no users row → no password to rotate.
		writeError(w, http.StatusForbidden, "Environment-configured accounts cannot change password")
		return
	}
	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	u, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.logger.Error("users: change password lookup failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error loading user")
		return
	}
	if u.Provider != "local" {
		writeError(w, http.StatusUnprocessableEntity, "Password change is only available for local accounts")
		return
	}
	if !u.IsActive {
		// Defence-in-depth: auth middleware already gates this, but refuse
		// explicitly rather than silently rotating a disabled account.
		writeError(w, http.StatusForbidden, "account is inactive")
		return
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(req.CurrentPassword)) != nil {
		// Use 400, not 401: the session IS authenticated; this is a
		// domain-level credential challenge failure. 401 would trigger the
		// client's session-refresh pipeline and log the user out.
		writeError(w, http.StatusBadRequest, "Invalid current password")
		return
	}
	if len(req.NewPassword) < minPasswordLen {
		writeError(w, http.StatusBadRequest, "password must be at least 12 characters")
		return
	}
	if req.NewPassword == req.CurrentPassword {
		writeError(w, http.StatusBadRequest, "new password must differ from current")
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcryptCostUsers)
	if err != nil {
		h.logger.Error("users: bcrypt hash (change password) failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error hashing password")
		return
	}
	if err := h.store.UpdatePasswordHash(r.Context(), id, string(hash)); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.logger.Error("users: update password hash failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error updating password")
		return
	}
	h.logger.Info("users: password changed",
		zap.Int64("user_id", id), zap.String("provider", u.Provider))

	// Audit the self-service password change. Actor and target are the same
	// user — this is a credential rotation initiated by its owner.
	evt := auditFromRequest(r)
	evt.Action = store.AuditActionPasswordChange
	evt.Outcome = store.AuditOutcomeSuccess
	evt.TargetType = store.AuditTargetUser
	evt.TargetID = strconv.FormatInt(id, 10)
	actorID := id
	evt.ActorID = &actorID
	evt.ActorLabel = u.Email
	auditRecord(r.Context(), h.audit, evt)

	// F-2: revoke every other refresh-token family for this user and blacklist
	// the access JTI presented in this request so the stolen-credential window
	// closes immediately on rotation. Best-effort: failures are logged but do
	// not fail the password change itself — the password is already rotated.
	userSub := strconv.FormatInt(id, 10)
	h.revokeAllFamilies(r.Context(), r, userSub, "password_change", 0)
	h.blacklistCurrentAccessToken(r)

	w.WriteHeader(http.StatusNoContent)
}

// ResetUserPassword godoc
// @Summary      Reset another user's password (admin only)
// @Description  Generates a one-time temporary password for the target user and
//
//	returns it in the response. Only local (password-based) accounts
//	may be reset. OIDC/env accounts are rejected.
//
// @Tags         users
// @Produce      json
// @Param        id  path  int  true  "User ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Failure      403  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Failure      422  {object}  map[string]any
// @Router       /users/{id}/password [post]
func (h *UserHandler) ResetUserPassword(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	u, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.logger.Error("users: reset password lookup failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error loading user")
		return
	}
	if u.Provider != "local" {
		writeError(w, http.StatusUnprocessableEntity, "cannot reset password for non-local accounts")
		return
	}

	tempPassword, err := generateTempPassword()
	if err != nil {
		h.logger.Error("users: generate temp password (reset) failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error generating password")
		return
	}
	if len(tempPassword) < minPasswordLen {
		writeError(w, http.StatusInternalServerError, "error generating password")
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(tempPassword), bcryptCostUsers)
	if err != nil {
		h.logger.Error("users: bcrypt hash (reset) failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error hashing password")
		return
	}
	if err := h.store.UpdatePasswordHash(r.Context(), id, string(hash)); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.logger.Error("users: update password hash (reset) failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error updating password")
		return
	}
	actorID, _ := claimsUserID(r) // 0 for env admin — still useful as a marker.
	h.logger.Info("users: password reset by admin",
		zap.Int64("actor_id", actorID),
		zap.Int64("target_id", id),
		zap.Bool("target_active", u.IsActive))

	// Audit the admin-driven password reset. Actor is the admin invoking the
	// endpoint; target is the user whose password was rotated.
	evt := auditFromRequest(r)
	evt.Action = store.AuditActionPasswordReset
	evt.Outcome = store.AuditOutcomeSuccess
	evt.TargetType = store.AuditTargetUser
	evt.TargetID = strconv.FormatInt(id, 10)
	if actorID != 0 {
		a := actorID
		evt.ActorID = &a
	}
	if actorSub, _, ok := claimsSubject(r); ok {
		evt.ActorLabel = actorSub
	}
	evt.Metadata = auditMetadata(map[string]any{"target_active": u.IsActive})
	auditRecord(r.Context(), h.audit, evt)

	// F-2: invalidate every active refresh-token family for the target and
	// hard-delete every API key they own. The original session, wherever it
	// was stolen to, must stop authenticating immediately. Best-effort.
	sub := strconv.FormatInt(id, 10)
	h.revokeAllFamilies(r.Context(), r, sub, "password_reset", id)
	h.cascadeDeleteAPIKeys(r.Context(), r, u.Email, "password_reset", id)

	writeSuccess(w, http.StatusOK, map[string]string{
		"temp_password": tempPassword,
	}, "password reset. Copy the temporary password — it won't be shown again.")
}

// Delete godoc
// @Summary      Deactivate a user
// @Description  Admin-only soft delete. The target user's is_active flag is
//
//	set to false. Rejects self-deactivation.
//
// @Tags         users
// @Produce      json
// @Param        id  path  int  true  "User ID"
// @Success      204  {string}  string  "no content"
// @Failure      400  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Failure      403  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Failure      422  {object}  map[string]any
// @Router       /users/{id} [delete]
func (h *UserHandler) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if selfID, ok := claimsUserID(r); ok && selfID == id {
		writeError(w, http.StatusUnprocessableEntity, "cannot deactivate your own account")
		return
	}
	// Load the target before deactivating so the post-deactivate cascade has
	// the user's email (used as api_keys.username for DB-backed users). The
	// loaded user is also useful for the audit row.
	target, err := h.store.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.logger.Error("users: deactivate lookup failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error loading user")
		return
	}
	if err := h.store.Deactivate(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		h.logger.Error("users: deactivate failed", zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error deactivating user")
		return
	}

	// Audit the (soft) delete. We use the deletes action even though the
	// underlying operation is a deactivation — that is the user-facing intent
	// and the audit log records intent, not implementation detail.
	evt := auditFromRequest(r)
	evt.Action = store.AuditActionUserDelete
	evt.Outcome = store.AuditOutcomeSuccess
	evt.TargetType = store.AuditTargetUser
	evt.TargetID = strconv.FormatInt(id, 10)
	if actorID, ok := claimsUserID(r); ok {
		a := actorID
		evt.ActorID = &a
	}
	if actorSub, _, ok := claimsSubject(r); ok {
		evt.ActorLabel = actorSub
	}
	auditRecord(r.Context(), h.audit, evt)

	// F-2: revoke every active session and delete every API key belonging to
	// the deactivated user. Best-effort: failures here log but do not change
	// the response — the account is already disabled and follow-up F-3 work
	// (per-request is_active recheck) will close any residual access window.
	sub := strconv.FormatInt(id, 10)
	h.revokeAllFamilies(r.Context(), r, sub, "user_deactivate", id)
	h.cascadeDeleteAPIKeys(r.Context(), r, target.Email, "user_deactivate", id)

	w.WriteHeader(http.StatusNoContent)
}
