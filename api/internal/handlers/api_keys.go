package handlers

import (
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
	store store.APIKeyStorer
}

// NewAPIKeyHandler creates a new APIKeyHandler.
func NewAPIKeyHandler(s store.APIKeyStorer) *APIKeyHandler {
	return &APIKeyHandler{store: s}
}

// apiKeyResponse is the JSON representation of an API key (without the hash).
type apiKeyResponse struct {
	ID        int64      `json:"id"`
	Name      string     `json:"name"`
	Prefix    string     `json:"prefix"`
	Username  string     `json:"username"`
	Role      string     `json:"role"`
	ExpiresAt *time.Time `json:"expires_at"`
	LastUsed  *time.Time `json:"last_used"`
	CreatedAt time.Time  `json:"created_at"`
}

// apiKeyCreateResponse extends apiKeyResponse with the full key shown once at creation.
type apiKeyCreateResponse struct {
	apiKeyResponse
	Key string `json:"key"`
}

func keyToResponse(k *store.APIKey) apiKeyResponse {
	return apiKeyResponse{
		ID:        k.ID,
		Name:      k.Name,
		Prefix:    k.Prefix,
		Username:  k.Username,
		Role:      k.Role,
		ExpiresAt: k.ExpiresAt,
		LastUsed:  k.LastUsed,
		CreatedAt: k.CreatedAt,
	}
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
	username, _ := claims["sub"].(string)

	keys, err := h.store.ListByUsername(ctx, username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error listing API keys")
		return
	}

	resp := make([]apiKeyResponse, 0, len(keys))
	for i := range keys {
		resp = append(resp, keyToResponse(&keys[i]))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data":     resp,
		"metadata": map[string]string{"message": "API keys retrieved"},
	})
}

// Create godoc
// @Summary      Create an API key
// @Description  Generates a new API key for the authenticated user (max 5 per user).
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
		Name      string  `json:"name"`
		ExpiresAt *string `json:"expires_at"`
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

	fullKey, err := security.GenerateAPIKey()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error generating API key")
		return
	}
	hash := security.HashAPIKey(fullKey)
	prefix := security.DisplayPrefix(fullKey)

	key := &store.APIKey{
		Name:      req.Name,
		Prefix:    prefix,
		KeyHash:   hash,
		Username:  username,
		Role:      role,
		ExpiresAt: expiresAt,
	}

	created, err := h.store.Create(ctx, key)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error creating API key")
		return
	}

	resp := apiKeyCreateResponse{
		apiKeyResponse: keyToResponse(created),
		Key:            fullKey,
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"data":     resp,
		"metadata": map[string]string{"message": "API key created. Copy it now — it won't be shown again."},
	})
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
	username, _ := claims["sub"].(string)

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	if err := h.store.Delete(ctx, id, username); err != nil {
		if errors.Is(err, store.ErrAPIKeyNotFound) {
			writeError(w, http.StatusNotFound, "API key not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "error deleting API key")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     map[string]any{"id": id},
		"metadata": map[string]string{"message": "API key deleted"},
	})
}
