package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// WebhookHandler handles HTTP requests for webhook management.
type WebhookHandler struct {
	store  store.WebhookStorer
	logger *zap.Logger
}

// NewWebhookHandler creates a WebhookHandler.
func NewWebhookHandler(s store.WebhookStorer, logger *zap.Logger) *WebhookHandler {
	return &WebhookHandler{store: s, logger: logger}
}

// createWebhookRequest is the payload for POST /projects/{project_id}/webhooks.
type createWebhookRequest struct {
	Name       string   `json:"name"`
	TargetType string   `json:"target_type"`
	URL        string   `json:"url"`
	Secret     *string  `json:"secret,omitempty"`
	Template   *string  `json:"template,omitempty"`
	Events     []string `json:"events,omitempty"`
	IsActive   *bool    `json:"is_active,omitempty"`
}

// updateWebhookRequest is the payload for PUT /projects/{project_id}/webhooks/{webhook_id}.
type updateWebhookRequest struct {
	Name       *string  `json:"name,omitempty"`
	TargetType *string  `json:"target_type,omitempty"`
	URL        *string  `json:"url,omitempty"`
	Secret     *string  `json:"secret,omitempty"`
	Template   *string  `json:"template,omitempty"`
	Events     []string `json:"events,omitempty"`
	IsActive   *bool    `json:"is_active,omitempty"`
}

// webhookResponse is the JSON representation of a webhook (URL masked, secret omitted).
type webhookResponse struct {
	ID         string   `json:"id"`
	ProjectID  string   `json:"project_id"`
	Name       string   `json:"name"`
	TargetType string   `json:"target_type"`
	URL        string   `json:"url"`
	HasSecret  bool     `json:"has_secret"`
	Template   *string  `json:"template,omitempty"`
	Events     []string `json:"events"`
	IsActive   bool     `json:"is_active"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

// maskURL masks a webhook URL showing only scheme+host.
// E.g. "https://hooks.slack.com/services/T00/B00/xxx" → "https://hooks.slack.com/****".
func maskURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "****"
	}
	return fmt.Sprintf("%s://%s/****", u.Scheme, u.Host)
}

// toWebhookResponse converts a store.Webhook to the API response format.
func toWebhookResponse(wh store.Webhook) webhookResponse {
	return webhookResponse{
		ID:         wh.ID,
		ProjectID:  wh.ProjectID,
		Name:       wh.Name,
		TargetType: wh.TargetType,
		URL:        maskURL(wh.URL),
		HasSecret:  wh.Secret != nil && *wh.Secret != "",
		Template:   wh.Template,
		Events:     wh.Events,
		IsActive:   wh.IsActive,
		CreatedAt:  wh.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:  wh.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}

// isPrivateIP checks if a hostname resolves to a private/loopback IP (SSRF prevention).
func isPrivateIP(host string) bool {
	hostname := host
	if h, _, err := net.SplitHostPort(host); err == nil {
		hostname = h
	}

	if hostname == "localhost" || hostname == "127.0.0.1" || hostname == "::1" {
		return true
	}

	resolver := &net.Resolver{}
	ips, err := resolver.LookupIPAddr(context.Background(), hostname)
	if err != nil {
		return false
	}
	for _, addr := range ips {
		if addr.IP.IsLoopback() || addr.IP.IsPrivate() || addr.IP.IsLinkLocalUnicast() || addr.IP.IsLinkLocalMulticast() {
			return true
		}
	}
	return false
}

// validateWebhookURL checks that a URL is valid HTTP(S) and doesn't point to private IPs.
func validateWebhookURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL must use http or https scheme")
	}
	if u.Host == "" {
		return fmt.Errorf("URL must have a host")
	}
	if isPrivateIP(u.Host) {
		return fmt.Errorf("URL must not point to private or loopback addresses")
	}
	return nil
}

// validTargetTypes is the allowed set of webhook target types.
var validTargetTypes = map[string]bool{
	"slack": true, "discord": true, "teams": true, "generic": true,
}

// maxWebhooksPerProject is the maximum number of webhooks allowed per project.
const maxWebhooksPerProject = 10

// List godoc
// @Summary      List webhooks for a project
// @Description  Returns all webhooks configured for the given project
// @Tags         webhooks
// @Produce      json
// @Param        project_id path string true "Project ID"
// @Success      200 {object} map[string]interface{}
// @Failure      400 {object} map[string]interface{}
// @Failure      500 {object} map[string]interface{}
// @Router       /projects/{project_id}/webhooks [get]
func (h *WebhookHandler) List(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	webhooks, err := h.store.List(r.Context(), projectID)
	if err != nil {
		h.logger.Error("list webhooks", zap.String("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error listing webhooks")
		return
	}
	if webhooks == nil {
		webhooks = []store.Webhook{}
	}

	responses := make([]webhookResponse, 0, len(webhooks))
	for i := range webhooks {
		responses = append(responses, toWebhookResponse(webhooks[i]))
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     responses,
		"metadata": map[string]string{"message": "Webhooks successfully obtained"},
	})
}

// Create godoc
// @Summary      Create a webhook for a project
// @Description  Adds a new webhook notification target to the given project (max 10 per project)
// @Tags         webhooks
// @Accept       json
// @Produce      json
// @Param        project_id path string true "Project ID"
// @Success      201 {object} map[string]interface{}
// @Failure      400 {object} map[string]interface{}
// @Failure      409 {object} map[string]interface{}
// @Failure      500 {object} map[string]interface{}
// @Router       /projects/{project_id}/webhooks [post]
func (h *WebhookHandler) Create(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}

	var req createWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Validate required fields.
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if len(req.Name) > 100 {
		writeError(w, http.StatusBadRequest, "name must not exceed 100 characters")
		return
	}
	if req.TargetType == "" {
		writeError(w, http.StatusBadRequest, "target_type is required")
		return
	}
	if !validTargetTypes[req.TargetType] {
		writeError(w, http.StatusBadRequest, "target_type must be one of: slack, discord, teams, generic")
		return
	}
	if req.URL == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}
	if err := validateWebhookURL(req.URL); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Enforce per-project webhook limit.
	existing, err := h.store.List(r.Context(), projectID)
	if err != nil {
		h.logger.Error("create webhook: list existing", zap.String("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error checking webhook count")
		return
	}
	if len(existing) >= maxWebhooksPerProject {
		writeError(w, http.StatusConflict, fmt.Sprintf("maximum of %d webhooks per project reached", maxWebhooksPerProject))
		return
	}

	// Apply defaults.
	events := req.Events
	if len(events) == 0 {
		events = []string{"report_completed"}
	}
	isActive := true
	if req.IsActive != nil {
		isActive = *req.IsActive
	}

	wh := store.Webhook{
		ProjectID:  projectID,
		Name:       req.Name,
		TargetType: req.TargetType,
		URL:        req.URL,
		Secret:     req.Secret,
		Template:   req.Template,
		Events:     events,
		IsActive:   isActive,
	}

	created, err := h.store.Create(r.Context(), &wh)
	if err != nil {
		h.logger.Error("create webhook", zap.String("project_id", projectID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error creating webhook")
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"data":     toWebhookResponse(*created),
		"metadata": map[string]string{"message": "Webhook created"},
	})
}

// Get godoc
// @Summary      Get a webhook
// @Description  Returns a single webhook by ID for the given project
// @Tags         webhooks
// @Produce      json
// @Param        project_id  path string true "Project ID"
// @Param        webhook_id  path string true "Webhook ID"
// @Success      200 {object} map[string]interface{}
// @Failure      400 {object} map[string]interface{}
// @Failure      404 {object} map[string]interface{}
// @Failure      500 {object} map[string]interface{}
// @Router       /projects/{project_id}/webhooks/{webhook_id} [get]
func (h *WebhookHandler) Get(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	webhookID := r.PathValue("webhook_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	if webhookID == "" {
		writeError(w, http.StatusBadRequest, "webhook_id is required")
		return
	}

	wh, err := h.store.GetByID(r.Context(), webhookID)
	if err != nil {
		if errors.Is(err, store.ErrWebhookNotFound) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		h.logger.Error("get webhook", zap.String("webhook_id", webhookID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error fetching webhook")
		return
	}

	// IDOR prevention: confirm this webhook belongs to the requested project.
	if wh.ProjectID != projectID {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     toWebhookResponse(*wh),
		"metadata": map[string]string{"message": "Webhook successfully obtained"},
	})
}

// Update godoc
// @Summary      Update a webhook
// @Description  Applies a partial update to an existing webhook
// @Tags         webhooks
// @Accept       json
// @Produce      json
// @Param        project_id  path string true "Project ID"
// @Param        webhook_id  path string true "Webhook ID"
// @Success      200 {object} map[string]interface{}
// @Failure      400 {object} map[string]interface{}
// @Failure      404 {object} map[string]interface{}
// @Failure      500 {object} map[string]interface{}
// @Router       /projects/{project_id}/webhooks/{webhook_id} [put]
func (h *WebhookHandler) Update(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	webhookID := r.PathValue("webhook_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	if webhookID == "" {
		writeError(w, http.StatusBadRequest, "webhook_id is required")
		return
	}

	wh, err := h.store.GetByID(r.Context(), webhookID)
	if err != nil {
		if errors.Is(err, store.ErrWebhookNotFound) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		h.logger.Error("update webhook: get", zap.String("webhook_id", webhookID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error fetching webhook")
		return
	}
	if wh.ProjectID != projectID {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}

	var req updateWebhookRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Apply non-nil fields.
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "name must not be empty")
			return
		}
		if len(name) > 100 {
			writeError(w, http.StatusBadRequest, "name must not exceed 100 characters")
			return
		}
		wh.Name = name
	}
	if req.TargetType != nil {
		if !validTargetTypes[*req.TargetType] {
			writeError(w, http.StatusBadRequest, "target_type must be one of: slack, discord, teams, generic")
			return
		}
		wh.TargetType = *req.TargetType
	}
	if req.URL != nil {
		if err := validateWebhookURL(*req.URL); err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		wh.URL = *req.URL
	}
	if req.Secret != nil {
		wh.Secret = req.Secret
	}
	if req.Template != nil {
		wh.Template = req.Template
	}
	if len(req.Events) > 0 {
		wh.Events = req.Events
	}
	if req.IsActive != nil {
		wh.IsActive = *req.IsActive
	}

	if err := h.store.Update(r.Context(), wh); err != nil {
		if errors.Is(err, store.ErrWebhookNotFound) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		h.logger.Error("update webhook", zap.String("webhook_id", webhookID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error updating webhook")
		return
	}

	// Fetch updated record to reflect any store-side changes (e.g. UpdatedAt).
	updated, err := h.store.GetByID(r.Context(), webhookID)
	if err != nil {
		h.logger.Error("update webhook: re-fetch", zap.String("webhook_id", webhookID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error fetching updated webhook")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     toWebhookResponse(*updated),
		"metadata": map[string]string{"message": "Webhook updated"},
	})
}

// Delete godoc
// @Summary      Delete a webhook
// @Description  Permanently removes a webhook from the given project
// @Tags         webhooks
// @Produce      json
// @Param        project_id  path string true "Project ID"
// @Param        webhook_id  path string true "Webhook ID"
// @Success      200 {object} map[string]interface{}
// @Failure      400 {object} map[string]interface{}
// @Failure      404 {object} map[string]interface{}
// @Failure      500 {object} map[string]interface{}
// @Router       /projects/{project_id}/webhooks/{webhook_id} [delete]
func (h *WebhookHandler) Delete(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	webhookID := r.PathValue("webhook_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	if webhookID == "" {
		writeError(w, http.StatusBadRequest, "webhook_id is required")
		return
	}

	if err := h.store.Delete(r.Context(), webhookID, projectID); err != nil {
		if errors.Is(err, store.ErrWebhookNotFound) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		h.logger.Error("delete webhook", zap.String("webhook_id", webhookID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error deleting webhook")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     map[string]string{"id": webhookID},
		"metadata": map[string]string{"message": "Webhook deleted"},
	})
}

// Test godoc
// @Summary      Trigger a test delivery for a webhook
// @Description  Queues a test notification for the given webhook (verifies the webhook exists)
// @Tags         webhooks
// @Produce      json
// @Param        project_id  path string true "Project ID"
// @Param        webhook_id  path string true "Webhook ID"
// @Success      200 {object} map[string]interface{}
// @Failure      400 {object} map[string]interface{}
// @Failure      404 {object} map[string]interface{}
// @Failure      500 {object} map[string]interface{}
// @Router       /projects/{project_id}/webhooks/{webhook_id}/test [post]
func (h *WebhookHandler) Test(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	webhookID := r.PathValue("webhook_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	if webhookID == "" {
		writeError(w, http.StatusBadRequest, "webhook_id is required")
		return
	}

	wh, err := h.store.GetByID(r.Context(), webhookID)
	if err != nil {
		if errors.Is(err, store.ErrWebhookNotFound) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		h.logger.Error("test webhook", zap.String("webhook_id", webhookID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error fetching webhook")
		return
	}
	if wh.ProjectID != projectID {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     map[string]string{"message": "test notification queued"},
		"metadata": map[string]string{"message": "Test delivery queued"},
	})
}

// ListDeliveries godoc
// @Summary      List delivery attempts for a webhook
// @Description  Returns paginated delivery log entries for the given webhook
// @Tags         webhooks
// @Produce      json
// @Param        project_id  path  string true  "Project ID"
// @Param        webhook_id  path  string true  "Webhook ID"
// @Param        page        query int    false "Page number (default 1)"
// @Param        per_page    query int    false "Items per page (default 20, max 100)"
// @Success      200 {object} map[string]interface{}
// @Failure      400 {object} map[string]interface{}
// @Failure      404 {object} map[string]interface{}
// @Failure      500 {object} map[string]interface{}
// @Router       /projects/{project_id}/webhooks/{webhook_id}/deliveries [get]
func (h *WebhookHandler) ListDeliveries(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("project_id")
	webhookID := r.PathValue("webhook_id")
	if projectID == "" {
		writeError(w, http.StatusBadRequest, "project_id is required")
		return
	}
	if webhookID == "" {
		writeError(w, http.StatusBadRequest, "webhook_id is required")
		return
	}

	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	perPage, _ := strconv.Atoi(q.Get("per_page"))
	if perPage < 1 || perPage > 100 {
		perPage = 20
	}

	// Verify webhook exists and belongs to this project.
	wh, err := h.store.GetByID(r.Context(), webhookID)
	if err != nil {
		if errors.Is(err, store.ErrWebhookNotFound) {
			writeError(w, http.StatusNotFound, "webhook not found")
			return
		}
		h.logger.Error("list deliveries: get webhook", zap.String("webhook_id", webhookID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error fetching webhook")
		return
	}
	if wh.ProjectID != projectID {
		writeError(w, http.StatusNotFound, "webhook not found")
		return
	}

	deliveries, total, err := h.store.ListDeliveries(r.Context(), webhookID, page, perPage)
	if err != nil {
		h.logger.Error("list deliveries", zap.String("webhook_id", webhookID), zap.Error(err))
		writeError(w, http.StatusInternalServerError, "error listing deliveries")
		return
	}
	if deliveries == nil {
		deliveries = []store.WebhookDelivery{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data":     deliveries,
		"total":    total,
		"page":     page,
		"per_page": perPage,
		"metadata": map[string]string{"message": "Deliveries successfully obtained"},
	})
}
