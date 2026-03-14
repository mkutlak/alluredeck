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

	"github.com/golang-jwt/jwt/v5"

	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// injectClaims returns a request with JWT claims injected into the context,
// simulating what AuthMiddleware does for authenticated requests.
func injectClaims(r *http.Request, username, role string) *http.Request {
	claims := jwt.MapClaims{
		"sub":  username,
		"role": role,
	}
	ctx := context.WithValue(r.Context(), middleware.ClaimsKey, claims)
	return r.WithContext(ctx)
}

func newTestAPIKeyHandler(t *testing.T) *APIKeyHandler {
	t.Helper()
	mocks := testutil.New()
	return NewAPIKeyHandler(mocks.APIKeys)
}

// makeAPIKey builds a minimal store.APIKey for seeding the in-memory store.
func makeAPIKey(name, username, role string) store.APIKey {
	return store.APIKey{
		Name:     name,
		Prefix:   "ald_a1b2c3d4",
		KeyHash:  "deadbeef" + name,
		Username: username,
		Role:     role,
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestAPIKeyHandler_List_Empty(t *testing.T) {
	t.Parallel()
	h := newTestAPIKeyHandler(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/api-keys", nil)
	if err != nil {
		t.Fatal(err)
	}
	req = injectClaims(req, "alice", "admin")

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

func TestAPIKeyHandler_List_ReturnsUserKeys(t *testing.T) {
	t.Parallel()
	mocks := testutil.New()
	h := NewAPIKeyHandler(mocks.APIKeys)

	ctx := context.Background()
	for _, name := range []string{"key-a", "key-b"} {
		k := makeAPIKey(name, "alice", "admin")
		if _, err := mocks.APIKeys.Create(ctx, &k); err != nil {
			t.Fatal(err)
		}
	}
	k := makeAPIKey("key-bob", "bob", "viewer")
	if _, err := mocks.APIKeys.Create(ctx, &k); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/api-keys", nil)
	if err != nil {
		t.Fatal(err)
	}
	req = injectClaims(req, "alice", "admin")

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
		t.Fatalf("expected 2 items for alice, got %d", len(data))
	}
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestAPIKeyHandler_Create_Success(t *testing.T) {
	t.Parallel()
	h := newTestAPIKeyHandler(t)

	body := map[string]any{"name": "my-ci-key"}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/api-keys", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req = injectClaims(req, "alice", "admin")

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
	key, _ := data["key"].(string)
	if len(key) < 4 || key[:4] != "ald_" {
		t.Errorf("expected key starting with ald_, got %q", key)
	}
	if data["name"] != "my-ci-key" {
		t.Errorf("name = %v, want %q", data["name"], "my-ci-key")
	}
}

func TestAPIKeyHandler_Create_FiveKeyLimit(t *testing.T) {
	t.Parallel()
	mocks := testutil.New()
	h := NewAPIKeyHandler(mocks.APIKeys)

	ctx := context.Background()
	for i := range 5 {
		k := makeAPIKey(fmt.Sprintf("key-%d", i), "alice", "admin")
		if _, err := mocks.APIKeys.Create(ctx, &k); err != nil {
			t.Fatal(err)
		}
	}

	body := map[string]any{"name": "key-6"}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/v1/api-keys", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req = injectClaims(req, "alice", "admin")

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("want 409 at key limit, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAPIKeyHandler_Create_MissingName(t *testing.T) {
	t.Parallel()
	h := newTestAPIKeyHandler(t)

	body := map[string]any{}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/api-keys", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req = injectClaims(req, "alice", "admin")

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for missing name, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAPIKeyHandler_Create_PastExpiresAt(t *testing.T) {
	t.Parallel()
	h := newTestAPIKeyHandler(t)

	past := time.Now().Add(-time.Hour).Format(time.RFC3339)
	body := map[string]any{"name": "expired-key", "expires_at": past}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/api-keys", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req = injectClaims(req, "alice", "admin")

	rr := httptest.NewRecorder()
	h.Create(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for past expires_at, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestAPIKeyHandler_Delete_OwnKey(t *testing.T) {
	t.Parallel()
	mocks := testutil.New()
	h := NewAPIKeyHandler(mocks.APIKeys)

	ctx := context.Background()
	k := makeAPIKey("my-key", "alice", "admin")
	created, err := mocks.APIKeys.Create(ctx, &k)
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v1/api-keys/%d", created.ID), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("id", fmt.Sprintf("%d", created.ID))
	req = injectClaims(req, "alice", "admin")

	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAPIKeyHandler_Delete_WrongUsername_IDOR(t *testing.T) {
	t.Parallel()
	mocks := testutil.New()
	h := NewAPIKeyHandler(mocks.APIKeys)

	ctx := context.Background()
	k := makeAPIKey("alice-key", "alice", "admin")
	created, err := mocks.APIKeys.Create(ctx, &k)
	if err != nil {
		t.Fatal(err)
	}

	// bob tries to delete alice's key
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		fmt.Sprintf("/api/v1/api-keys/%d", created.ID), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("id", fmt.Sprintf("%d", created.ID))
	req = injectClaims(req, "bob", "admin")

	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 for IDOR attempt, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAPIKeyHandler_Delete_NonExistent(t *testing.T) {
	t.Parallel()
	h := newTestAPIKeyHandler(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		"/api/v1/api-keys/9999", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("id", "9999")
	req = injectClaims(req, "alice", "admin")

	rr := httptest.NewRecorder()
	h.Delete(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 for non-existent key, got %d: %s", rr.Code, rr.Body.String())
	}
}
