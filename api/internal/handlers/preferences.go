package handlers

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// PreferenceHandler handles HTTP requests for user UI preferences.
type PreferenceHandler struct {
	store store.PreferenceStorer
}

// NewPreferenceHandler creates a new PreferenceHandler.
func NewPreferenceHandler(s store.PreferenceStorer) *PreferenceHandler {
	return &PreferenceHandler{store: s}
}

// preferencesResponse is the JSON shape returned by GET and PUT.
type preferencesResponse struct {
	Preferences json.RawMessage `json:"preferences"`
	UpdatedAt   string          `json:"updated_at"`
}

// GetPreferences returns the authenticated user's preferences.
func (h *PreferenceHandler) GetPreferences(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing claims")
		return
	}
	username, _ := claims["sub"].(string)

	prefs, err := h.store.GetPreferences(r.Context(), username)
	if err != nil {
		if errors.Is(err, store.ErrPreferencesNotFound) {
			writeSuccess(w, http.StatusOK, preferencesResponse{
				Preferences: json.RawMessage(`{}`),
				UpdatedAt:   "",
			}, "no preferences found")
			return
		}
		writeError(w, http.StatusInternalServerError, "error fetching preferences")
		return
	}

	writeSuccess(w, http.StatusOK, preferencesResponse{
		Preferences: prefs.Preferences,
		UpdatedAt:   prefs.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}, "ok")
}

// upsertPreferencesRequest is the expected JSON body for PUT.
type upsertPreferencesRequest struct {
	Preferences json.RawMessage `json:"preferences"`
}

// UpsertPreferences creates or updates the authenticated user's preferences.
func (h *PreferenceHandler) UpsertPreferences(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.ClaimsFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "missing claims")
		return
	}
	username, _ := claims["sub"].(string)

	var req upsertPreferencesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if len(req.Preferences) == 0 {
		writeError(w, http.StatusBadRequest, "preferences field is required")
		return
	}

	prefs, err := h.store.UpsertPreferences(r.Context(), username, req.Preferences)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error saving preferences")
		return
	}

	writeSuccess(w, http.StatusOK, preferencesResponse{
		Preferences: prefs.Preferences,
		UpdatedAt:   prefs.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}, "preferences saved")
}
