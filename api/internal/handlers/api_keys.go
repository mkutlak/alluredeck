package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// APIKeyHandler handles CRUD operations for API keys.
type APIKeyHandler struct {
	store     store.APIKeyStorer
	userStore store.UserStorer  // used at creation to resolve numeric sub → email
	audit     store.AuditLogger // optional; nil disables persistent audit emission
}

// NewAPIKeyHandler creates a new APIKeyHandler.
func NewAPIKeyHandler(s store.APIKeyStorer, us store.UserStorer) *APIKeyHandler {
	return &APIKeyHandler{store: s, userStore: us}
}

// WithAuditLogger wires the persistent audit logger so api_keys.create and
// api_keys.delete events are recorded to the audit_log table. Nil is
// acceptable — audit becomes a no-op.
func (h *APIKeyHandler) WithAuditLogger(a store.AuditLogger) *APIKeyHandler {
	h.audit = a
	return h
}

// apiKeyResponse is the JSON representation of an API key (without the hash).
type apiKeyResponse struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"`
	Prefix         string     `json:"prefix"`
	Username       string     `json:"username"`
	Role           string     `json:"role"`
	ExpiresAt      *time.Time `json:"expires_at"`
	LastUsed       *time.Time `json:"last_used"`
	CreatedAt      time.Time  `json:"created_at"`
	AllowMCPWrites bool       `json:"allow_mcp_writes"`
	ProjectIDs     []int64    `json:"project_ids,omitempty"`
}

// apiKeyCreateResponse extends apiKeyResponse with the full key shown once at creation.
type apiKeyCreateResponse struct {
	apiKeyResponse
	Key string `json:"key"`
}

func keyToResponse(k *store.APIKey) apiKeyResponse {
	return apiKeyResponse{
		ID:             k.ID,
		Name:           k.Name,
		Prefix:         k.Prefix,
		Username:       k.Username,
		Role:           k.Role,
		ExpiresAt:      k.ExpiresAt,
		LastUsed:       k.LastUsed,
		CreatedAt:      k.CreatedAt,
		AllowMCPWrites: k.AllowMCPWrites,
		ProjectIDs:     k.ProjectIDs,
	}
}

// resolveKeyUsername maps a JWT sub claim to the username stored on api_keys
// rows. DB-backed users have a numeric sub — resolve it to their email so it
// matches the username persisted at creation. Env users have a non-numeric
// literal sub — return it unchanged.
func (h *APIKeyHandler) resolveKeyUsername(ctx context.Context, sub string) (string, error) {
	id, err := strconv.ParseInt(sub, 10, 64)
	if err != nil {
		return sub, nil
	}
	u, err := h.userStore.GetByID(ctx, id)
	if err != nil {
		return "", err
	}
	return u.Email, nil
}

// List godoc
// @Summary      List API keys
// @Description  Returns all API keys for the authenticated user.
// @Tags         api-keys
// @Produce      json
// @Success      200  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Router       /api-keys [get]
func (h *APIKeyHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	claims, ok := middleware.ClaimsFromContext(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing claims")
		return
	}
	sub, _ := claims["sub"].(string)
	username, err := h.resolveKeyUsername(ctx, sub)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error resolving user")
		return
	}

	keys, err := h.store.ListByUsername(ctx, username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error listing API keys")
		return
	}

	resp := make([]apiKeyResponse, 0, len(keys))
	for i := range keys {
		resp = append(resp, keyToResponse(&keys[i]))
	}
	writeSuccess(w, http.StatusOK, resp, "API keys retrieved")
}

// Create godoc
// @Summary      Create an API key
// @Description  Generates a new API key for the authenticated user (max 5 per user). Set allow_mcp_writes to true to opt the key into MCP mutations.
// @Tags         api-keys
// @Accept       json
// @Produce      json
// @Success      201  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Failure      409  {object}  map[string]any
// @Router       /api-keys [post]
func (h *APIKeyHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	claims, ok := middleware.ClaimsFromContext(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing claims")
		return
	}
	username, _ := claims["sub"].(string)
	role, _ := claims["role"].(string)

	count, err := h.store.CountByUsername(ctx, username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error checking API key count")
		return
	}
	if count >= 5 {
		writeError(w, http.StatusConflict, "maximum of 5 API keys per user reached")
		return
	}

	var req struct {
		Name           string  `json:"name"`
		ExpiresAt      *string `json:"expires_at"`
		AllowMCPWrites bool    `json:"allow_mcp_writes"`
		ProjectIDs     []int64 `json:"project_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Name) > 64 {
		writeError(w, http.StatusBadRequest, "name must not exceed 64 characters")
		return
	}
	if len(req.ProjectIDs) > 100 {
		writeError(w, http.StatusBadRequest, "project_ids must not exceed 100 entries")
		return
	}
	for _, pid := range req.ProjectIDs {
		if pid <= 0 {
			writeError(w, http.StatusBadRequest, "each project_id must be a positive integer")
			return
		}
	}

	var expiresAt *time.Time
	if req.ExpiresAt != nil && *req.ExpiresAt != "" {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			writeError(w, http.StatusBadRequest, "expires_at must be a valid RFC3339 timestamp")
			return
		}
		if !t.After(time.Now()) {
			writeError(w, http.StatusBadRequest, "expires_at must be in the future")
			return
		}
		expiresAt = &t
	}

	// Resolve the username stored on the key. For DB-backed users the JWT sub
	// is a numeric user ID string; store the user's email instead so F-3 can
	// look it up via IsActiveByEmail. For env users (non-numeric sub) keep the
	// literal as-is — IsActiveByAPIKeyUsername handles those at runtime.
	keyUsername, err := h.resolveKeyUsername(ctx, username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error resolving user")
		return
	}

	fullKey, err := security.GenerateAPIKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error generating API key")
		return
	}
	hash := security.HashAPIKey(fullKey)
	prefix := security.DisplayPrefix(fullKey)

	key := &store.APIKey{
		Name:           req.Name,
		Prefix:         prefix,
		KeyHash:        hash,
		Username:       keyUsername,
		Role:           role,
		ExpiresAt:      expiresAt,
		AllowMCPWrites: req.AllowMCPWrites,
		ProjectIDs:     req.ProjectIDs,
	}

	created, err := h.store.Create(ctx, key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error creating API key")
		return
	}

	// Audit api-key creation. The actor is the JWT subject (the username); the
	// target is the new key's id. We deliberately do not log the key plaintext
	// or the prefix metadata in audit_log.metadata — those are handed back to
	// the caller in the HTTP response only.
	evt := auditFromRequest(r)
	evt.Action = store.AuditActionAPIKeyCreate
	evt.Outcome = store.AuditOutcomeSuccess
	evt.ActorLabel = username
	evt.TargetType = store.AuditTargetAPIKey
	evt.TargetID = strconv.FormatInt(created.ID, 10)
	if id, parseErr := strconv.ParseInt(username, 10, 64); parseErr == nil {
		evt.ActorID = &id
	}
	evt.Metadata = auditMetadata(map[string]any{
		"name":             created.Name,
		"prefix":           created.Prefix,
		"allow_mcp_writes": created.AllowMCPWrites,
	})
	auditRecord(ctx, h.audit, evt)

	resp := apiKeyCreateResponse{
		apiKeyResponse: keyToResponse(created),
		Key:            fullKey,
	}
	writeSuccess(w, http.StatusCreated, resp, "API key created. Copy it now — it won't be shown again.")
}

// Delete godoc
// @Summary      Delete an API key
// @Description  Permanently removes an API key owned by the authenticated user.
// @Tags         api-keys
// @Produce      json
// @Param        id  path  int  true  "API key ID"
// @Success      200  {object}  map[string]any
// @Failure      400  {object}  map[string]any
// @Failure      401  {object}  map[string]any
// @Failure      404  {object}  map[string]any
// @Router       /api-keys/{id} [delete]
func (h *APIKeyHandler) Delete(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	claims, ok := middleware.ClaimsFromContext(ctx)
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing claims")
		return
	}
	sub, _ := claims["sub"].(string)
	keyUsername, err := h.resolveKeyUsername(ctx, sub)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error resolving user")
		return
	}

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.store.Delete(ctx, id, keyUsername); err != nil {
		if errors.Is(err, store.ErrAPIKeyNotFound) {
			writeError(w, http.StatusNotFound, "API key not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "error deleting API key")
		return
	}

	evt := auditFromRequest(r)
	evt.Action = store.AuditActionAPIKeyDelete
	evt.Outcome = store.AuditOutcomeSuccess
	evt.ActorLabel = sub
	evt.TargetType = store.AuditTargetAPIKey
	evt.TargetID = strconv.FormatInt(id, 10)
	if uid, parseErr := strconv.ParseInt(sub, 10, 64); parseErr == nil {
		evt.ActorID = &uid
	}
	auditRecord(ctx, h.audit, evt)

	writeSuccess(w, http.StatusOK, map[string]any{"id": id}, "API key deleted")
}
