package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// newTestWebhookHandler returns a WebhookHandler backed by an in-memory store.
func newTestWebhookHandler(t *testing.T) (*WebhookHandler, *testutil.MemWebhookStore) {
	t.Helper()
	whs := testutil.NewMemWebhookStore()
	mocks := testutil.New()
	h := NewWebhookHandler(whs, mocks.Projects, zap.NewNop())
	return h, whs
}

// makeWebhook inserts a webhook into the store and returns it.
func makeWebhook(t *testing.T, whs *testutil.MemWebhookStore, projectID int64, name string) store.Webhook {
	t.Helper()
	isActive := true
	wh := store.Webhook{
		ProjectID:  projectID,
		Name:       name,
		TargetType: "generic",
		URL:        "https://example.com/hook",
		Events:     []string{"report_completed"},
		IsActive:   isActive,
	}
	created, err := whs.Create(context.Background(), &wh)
	if err != nil {
		t.Fatal(err)
	}
	return *created
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestWebhookHandler_List_Empty(t *testing.T) {
	t.Parallel()
	h, _ := newTestWebhookHandler(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj-1/webhooks", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].([]any)
	if len(data) != 0 {
		t.Fatalf("expected empty array, got %d items", len(data))
	}
}

func TestWebhookHandler_List_ReturnsMaskedURLs(t *testing.T) {
	t.Parallel()
	h, whs := newTestWebhookHandler(t)

	ctx := context.Background()
	_, err := whs.Create(ctx, &store.Webhook{
		ProjectID:  1,
		Name:       "slack-notify",
		TargetType: "slack",
		URL:        "https://hooks.slack.com/services/T00/B00/secret",
		Events:     []string{"report_completed"},
		IsActive:   true,
	})
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/v1/projects/proj-1/webhooks", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("expected 1 item, got %d", len(data))
	}
	item := data[0].(map[string]any)
	gotURL, _ := item["url"].(string)
	if gotURL != "https://hooks.slack.com/****" {
		t.Errorf("URL not masked: got %q", gotURL)
	}
	// Secret must not appear in response.
	if _, hasSecret := item["secret"]; hasSecret {
		t.Error("secret must not be present in response")
	}
}

func TestWebhookHandler_List_IsolatedByProject(t *testing.T) {
	t.Parallel()
	h, whs := newTestWebhookHandler(t)

	ctx := context.Background()
	makeWebhook(t, whs, 1, "wh-a")
	makeWebhook(t, whs, 1, "wh-b")
	makeWebhook(t, whs, 2, "wh-c")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/v1/projects/proj-1/webhooks", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.List(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("expected 2 items for proj-1, got %d", len(data))
	}
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestWebhookHandler_Create_Success(t *testing.T) {
	t.Parallel()
	h, _ := newTestWebhookHandler(t)

	body := map[string]any{
		"name":        "my-webhook",
		"target_type": "slack",
		"url":         "https://hooks.slack.com/services/T00/B00/abc",
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/projects/proj-1/webhooks", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data object")
	}
	if data["name"] != "my-webhook" {
		t.Errorf("name = %v, want %q", data["name"], "my-webhook")
	}
	if data["target_type"] != "slack" {
		t.Errorf("target_type = %v, want %q", data["target_type"], "slack")
	}
	// Default events applied.
	events, _ := data["events"].([]any)
	if len(events) != 1 || events[0] != "report_completed" {
		t.Errorf("default events not applied: %v", events)
	}
}

func TestWebhookHandler_Create_DefaultIsActive(t *testing.T) {
	t.Parallel()
	h, _ := newTestWebhookHandler(t)

	body := map[string]any{
		"name":        "active-webhook",
		"target_type": "generic",
		"url":         "https://example.com/hook",
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/projects/proj-1/webhooks", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["is_active"] != true {
		t.Errorf("is_active default should be true, got %v", data["is_active"])
	}
}

func TestWebhookHandler_Create_MissingName(t *testing.T) {
	t.Parallel()
	h, _ := newTestWebhookHandler(t)

	body := map[string]any{"target_type": "slack", "url": "https://example.com/hook"}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/projects/proj-1/webhooks", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestWebhookHandler_Create_InvalidTargetType(t *testing.T) {
	t.Parallel()
	h, _ := newTestWebhookHandler(t)

	body := map[string]any{
		"name":        "wh",
		"target_type": "unknown",
		"url":         "https://example.com/hook",
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/projects/proj-1/webhooks", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid target_type, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestWebhookHandler_Create_InvalidURL_NonHTTPS(t *testing.T) {
	t.Parallel()
	h, _ := newTestWebhookHandler(t)

	body := map[string]any{
		"name":        "wh",
		"target_type": "generic",
		"url":         "ftp://example.com/hook",
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/projects/proj-1/webhooks", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for ftp URL, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestWebhookHandler_Create_InvalidURL_Loopback(t *testing.T) {
	t.Parallel()
	h, _ := newTestWebhookHandler(t)

	body := map[string]any{
		"name":        "wh",
		"target_type": "generic",
		"url":         "http://127.0.0.1/hook",
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/projects/proj-1/webhooks", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for loopback URL, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestWebhookHandler_Create_MaxWebhooksLimit(t *testing.T) {
	t.Parallel()
	h, whs := newTestWebhookHandler(t)

	ctx := context.Background()
	for i := range maxWebhooksPerProject {
		makeWebhook(t, whs, 1, fmt.Sprintf("wh-%d", i))
	}

	body := map[string]any{
		"name":        "one-too-many",
		"target_type": "generic",
		"url":         "https://example.com/hook",
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/v1/projects/proj-1/webhooks", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("want 409 at webhook limit, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Get
// ---------------------------------------------------------------------------

func TestWebhookHandler_Get_Found(t *testing.T) {
	t.Parallel()
	h, whs := newTestWebhookHandler(t)

	wh := makeWebhook(t, whs, 1, "my-hook")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj-1/webhooks/"+wh.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")
	req.SetPathValue("webhook_id", wh.ID)

	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["id"] != wh.ID {
		t.Errorf("id = %v, want %q", data["id"], wh.ID)
	}
}

func TestWebhookHandler_Get_NotFound(t *testing.T) {
	t.Parallel()
	h, _ := newTestWebhookHandler(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj-1/webhooks/nonexistent", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")
	req.SetPathValue("webhook_id", "nonexistent")

	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestWebhookHandler_Get_IDOR(t *testing.T) {
	t.Parallel()
	h, whs := newTestWebhookHandler(t)

	// Webhook belongs to proj-1.
	wh := makeWebhook(t, whs, 1, "proj1-hook")

	// proj-2 requests it — should get 404.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj-2/webhooks/"+wh.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "2")
	req.SetPathValue("webhook_id", wh.ID)

	rr := httptest.NewRecorder()
	h.Get(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 for IDOR attempt, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestWebhookHandler_Update_PartialUpdate(t *testing.T) {
	t.Parallel()
	h, whs := newTestWebhookHandler(t)

	wh := makeWebhook(t, whs, 1, "original-name")

	newName := "updated-name"
	body := map[string]any{"name": newName}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut,
		"/api/v1/projects/proj-1/webhooks/"+wh.ID, bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "1")
	req.SetPathValue("webhook_id", wh.ID)

	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["name"] != newName {
		t.Errorf("name = %v, want %q", data["name"], newName)
	}
	// target_type unchanged.
	if data["target_type"] != "generic" {
		t.Errorf("target_type changed unexpectedly: %v", data["target_type"])
	}
}

func TestWebhookHandler_Update_InvalidURL(t *testing.T) {
	t.Parallel()
	h, whs := newTestWebhookHandler(t)

	wh := makeWebhook(t, whs, 1, "hook")

	body := map[string]any{"url": "http://localhost/evil"}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut,
		"/api/v1/projects/proj-1/webhooks/"+wh.ID, bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "1")
	req.SetPathValue("webhook_id", wh.ID)

	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for loopback URL update, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestWebhookHandler_Update_NotFound(t *testing.T) {
	t.Parallel()
	h, _ := newTestWebhookHandler(t)

	body := map[string]any{"name": "new"}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut,
		"/api/v1/projects/proj-1/webhooks/nonexistent", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "1")
	req.SetPathValue("webhook_id", "nonexistent")

	rr := httptest.NewRecorder()
	h.Update(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestWebhookHandler_Delete_Success(t *testing.T) {
	t.Parallel()
	h, whs := newTestWebhookHandler(t)

	wh := makeWebhook(t, whs, 1, "to-delete")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		"/api/v1/projects/proj-1/webhooks/"+wh.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")
	req.SetPathValue("webhook_id", wh.ID)

	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestWebhookHandler_Delete_NotFound(t *testing.T) {
	t.Parallel()
	h, _ := newTestWebhookHandler(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		"/api/v1/projects/proj-1/webhooks/nonexistent", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")
	req.SetPathValue("webhook_id", "nonexistent")

	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestWebhookHandler_Delete_WrongProject_IDOR(t *testing.T) {
	t.Parallel()
	h, whs := newTestWebhookHandler(t)

	wh := makeWebhook(t, whs, 1, "hook")

	// proj-2 tries to delete proj-1's webhook.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		"/api/v1/projects/proj-2/webhooks/"+wh.ID, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "2")
	req.SetPathValue("webhook_id", wh.ID)

	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 for IDOR delete, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Test (trigger test delivery)
// ---------------------------------------------------------------------------

func TestWebhookHandler_Test_Success(t *testing.T) {
	t.Parallel()
	h, whs := newTestWebhookHandler(t)

	wh := makeWebhook(t, whs, 1, "hook")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/projects/proj-1/webhooks/"+wh.ID+"/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")
	req.SetPathValue("webhook_id", wh.ID)

	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["message"] != "test notification queued" {
		t.Errorf("unexpected message: %v", data["message"])
	}
}

func TestWebhookHandler_Test_NotFound(t *testing.T) {
	t.Parallel()
	h, _ := newTestWebhookHandler(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/projects/proj-1/webhooks/nonexistent/test", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")
	req.SetPathValue("webhook_id", "nonexistent")

	rr := httptest.NewRecorder()
	h.Test(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// ListDeliveries
// ---------------------------------------------------------------------------

func TestWebhookHandler_ListDeliveries_Empty(t *testing.T) {
	t.Parallel()
	h, whs := newTestWebhookHandler(t)

	wh := makeWebhook(t, whs, 1, "hook")

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj-1/webhooks/"+wh.ID+"/deliveries", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")
	req.SetPathValue("webhook_id", wh.ID)

	rr := httptest.NewRecorder()
	h.ListDeliveries(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].([]any)
	if len(data) != 0 {
		t.Fatalf("expected 0 deliveries, got %d", len(data))
	}
}

func TestWebhookHandler_ListDeliveries_Pagination(t *testing.T) {
	t.Parallel()
	h, whs := newTestWebhookHandler(t)

	wh := makeWebhook(t, whs, 1, "hook")

	ctx := context.Background()
	// Insert 5 deliveries.
	for i := range 5 {
		d := &store.WebhookDelivery{
			ID:          fmt.Sprintf("dlv-%d", i),
			WebhookID:   wh.ID,
			Event:       "report_completed",
			Payload:     "{}",
			Attempt:     1,
			DeliveredAt: time.Now(),
		}
		if err := whs.InsertDelivery(ctx, d); err != nil {
			t.Fatal(err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/v1/projects/proj-1/webhooks/"+wh.ID+"/deliveries?page=1&per_page=3", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")
	req.SetPathValue("webhook_id", wh.ID)

	rr := httptest.NewRecorder()
	h.ListDeliveries(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].([]any)
	if len(data) != 3 {
		t.Fatalf("expected 3 items on page 1, got %d", len(data))
	}
	pg, _ := resp["pagination"].(map[string]any)
	total, _ := pg["total"].(float64)
	if int(total) != 5 {
		t.Errorf("total = %v, want 5", total)
	}
	pageNum, _ := pg["page"].(float64)
	if int(pageNum) != 1 {
		t.Errorf("page = %v, want 1", pageNum)
	}
}

func TestWebhookHandler_ListDeliveries_WebhookNotFound(t *testing.T) {
	t.Parallel()
	h, _ := newTestWebhookHandler(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj-1/webhooks/nonexistent/deliveries", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")
	req.SetPathValue("webhook_id", "nonexistent")

	rr := httptest.NewRecorder()
	h.ListDeliveries(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}
