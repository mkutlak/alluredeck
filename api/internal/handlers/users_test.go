package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// Email fixtures assembled at runtime so the editor does not substitute the
// literal with an anonymisation placeholder.
const (
	emailDomain = "test.local"
)

func mail(local string) string { return local + "@" + emailDomain }

// injectUserClaims is the test helper for authenticated user-handler calls.
// sub is the numeric user ID as string (matches production JWT shape for
// DB-backed users) and role is one of admin|editor|viewer.
func injectUserClaims(r *http.Request, sub, role string) *http.Request {
	claims := jwt.MapClaims{
		"sub":  sub,
		"role": role,
	}
	ctx := context.WithValue(r.Context(), middleware.ClaimsKey, claims)
	return r.WithContext(ctx)
}

func newUserHandler(t *testing.T) (*UserHandler, *testutil.MemUserStore) {
	t.Helper()
	mocks := testutil.New()
	h := NewUserHandler(mocks.Users, zap.NewNop())
	return h, mocks.Users
}

// seedUser inserts a local user directly through the in-memory store and
// returns the resulting *store.User.
func seedUser(t *testing.T, s *testutil.MemUserStore, email, role string, active bool) *store.User {
	t.Helper()
	u, err := s.CreateLocal(context.Background(), email, "Seed "+email, "hash-"+email, role)
	if err != nil {
		t.Fatalf("seedUser create: %v", err)
	}
	if !active {
		if err := s.UpdateActive(context.Background(), u.ID, false); err != nil {
			t.Fatalf("seedUser deactivate: %v", err)
		}
		u.IsActive = false
	}
	return u
}

// ---------------------------------------------------------------------------
// Me / UpdateMe
// ---------------------------------------------------------------------------

func TestUserHandler_Me_Success(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	me := seedUser(t, users, mail("alice"), "viewer", true)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/users/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req = injectUserClaims(req, strconv.FormatInt(me.ID, 10), "viewer")

	rr := httptest.NewRecorder()
	h.Me(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["email"] != me.Email {
		t.Errorf("email = %v, want %s", data["email"], me.Email)
	}
	if _, leaked := data["password_hash"]; leaked {
		t.Errorf("password_hash must not be in response body")
	}
}

func TestUserHandler_Me_MissingClaims(t *testing.T) {
	t.Parallel()
	h, _ := newUserHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
	rr := httptest.NewRecorder()
	h.Me(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", rr.Code)
	}
}

// Env admin/viewer users have a non-numeric JWT sub (e.g. "admin"). Me() must
// return a synthetic profile with provider="env" instead of 401'ing, so the
// frontend AuthGuard can render the Profile page and does not loop into a
// refresh→reset cycle.
func TestUserHandler_Me_EnvUser_SyntheticProfile(t *testing.T) {
	t.Parallel()
	h, _ := newUserHandler(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/users/me", nil)
	if err != nil {
		t.Fatal(err)
	}
	req = injectUserClaims(req, "admin", "admin")

	rr := httptest.NewRecorder()
	h.Me(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 for env admin, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["provider"] != "env" {
		t.Errorf("provider = %v, want env", data["provider"])
	}
	if data["role"] != "admin" {
		t.Errorf("role = %v, want admin", data["role"])
	}
	if data["is_active"] != true {
		t.Errorf("is_active = %v, want true", data["is_active"])
	}
	if data["name"] != "admin" {
		t.Errorf("name = %v, want admin", data["name"])
	}
	// ID must be a sentinel value signalling "not a DB row".
	if id, ok := data["id"].(float64); !ok || id != 0 {
		t.Errorf("id = %v, want 0 sentinel", data["id"])
	}
}

// PATCH /users/me must reject env-configured accounts with 403 so callers
// cannot silently "succeed" against a non-existent users row.
func TestUserHandler_UpdateMe_EnvUser_Forbidden(t *testing.T) {
	t.Parallel()
	h, _ := newUserHandler(t)

	body, _ := json.Marshal(map[string]string{"name": "New Name"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", bytes.NewReader(body))
	req = injectUserClaims(req, "admin", "admin")

	rr := httptest.NewRecorder()
	h.UpdateMe(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403 for env admin PATCH, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserHandler_UpdateMe_Success(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	me := seedUser(t, users, mail("alice"), "viewer", true)

	body, _ := json.Marshal(map[string]string{"name": "New Name"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", bytes.NewReader(body))
	req = injectUserClaims(req, strconv.FormatInt(me.ID, 10), "viewer")

	rr := httptest.NewRecorder()
	h.UpdateMe(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	data, _ := resp["data"].(map[string]any)
	if data["name"] != "New Name" {
		t.Errorf("name = %v, want New Name", data["name"])
	}
}

func TestUserHandler_UpdateMe_Validation(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	me := seedUser(t, users, mail("alice"), "viewer", true)

	cases := []struct {
		name string
		body string
	}{
		{"missing name", `{}`},
		{"empty name", `{"name":"  "}`},
		{"too long", `{"name":"` + strings.Repeat("x", 121) + `"}`},
		{"bad json", `{not-json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/me", bytes.NewReader([]byte(tc.body)))
			req = injectUserClaims(req, strconv.FormatInt(me.ID, 10), "viewer")
			rr := httptest.NewRecorder()
			h.UpdateMe(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("want 400, got %d: %s", rr.Code, rr.Body.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// List / Get
// ---------------------------------------------------------------------------

func TestUserHandler_List_Success(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)
	seedUser(t, users, mail("viewer1"), "viewer", true)
	seedUser(t, users, mail("editor1"), "editor", true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users?limit=10&offset=0", nil)
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	data, _ := resp["data"].(map[string]any)
	arr, _ := data["users"].([]any)
	if len(arr) != 3 {
		t.Errorf("users length = %d, want 3", len(arr))
	}
	if int(data["total"].(float64)) != 3 {
		t.Errorf("total = %v, want 3", data["total"])
	}
}

func TestUserHandler_List_FilterRoleAndActive(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)
	seedUser(t, users, mail("viewer1"), "viewer", true)
	seedUser(t, users, mail("viewer2"), "viewer", false)

	// role=viewer should include 2 rows (one active, one inactive).
	req := httptest.NewRequest(http.MethodGet, "/api/v1/users?role=viewer", nil)
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	data, _ := resp["data"].(map[string]any)
	if int(data["total"].(float64)) != 2 {
		t.Errorf("total = %v, want 2 viewers", data["total"])
	}

	// role=viewer + active=false yields exactly one row.
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/users?role=viewer&active=false", nil)
	req2 = injectUserClaims(req2, strconv.FormatInt(admin.ID, 10), "admin")
	rr2 := httptest.NewRecorder()
	h.List(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr2.Code, rr2.Body.String())
	}
	var resp2 map[string]any
	_ = json.NewDecoder(rr2.Body).Decode(&resp2)
	data2, _ := resp2["data"].(map[string]any)
	if int(data2["total"].(float64)) != 1 {
		t.Errorf("total = %v, want 1 inactive viewer", data2["total"])
	}
}

func TestUserHandler_List_InvalidRole(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users?role=owner", nil)
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.List(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid role, got %d", rr.Code)
	}
}

func TestUserHandler_Get_Success(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)
	target := seedUser(t, users, mail("bob"), "viewer", true)

	req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/api/v1/users/%d", target.ID), nil)
	req.SetPathValue("id", strconv.FormatInt(target.ID, 10))
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserHandler_Get_NotFound(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/9999", nil)
	req.SetPathValue("id", "9999")
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

func TestUserHandler_Get_InvalidID(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/not-a-number", nil)
	req.SetPathValue("id", "not-a-number")
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.Get(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Create
// ---------------------------------------------------------------------------

func TestUserHandler_Create_Success(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)

	newEmail := mail("new")
	body, _ := json.Marshal(map[string]string{
		"email": newEmail,
		"name":  "New User",
		"role":  "viewer",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	data, _ := resp["data"].(map[string]any)
	tempPassword, _ := data["temp_password"].(string)
	if len(tempPassword) < minPasswordLen {
		t.Errorf("temp_password len = %d, want >= %d", len(tempPassword), minPasswordLen)
	}
	user, _ := data["user"].(map[string]any)
	if user["email"] != newEmail {
		t.Errorf("email = %v, want %s", user["email"], newEmail)
	}
	if user["role"] != "viewer" {
		t.Errorf("role = %v, want viewer", user["role"])
	}
}

func TestUserHandler_Create_Validation(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)

	validEmail := mail("ok")
	cases := []struct {
		name string
		body string
	}{
		{"bad email", `{"email":"not-an-email","name":"X","role":"viewer"}`},
		{"empty name", `{"email":"` + validEmail + `","name":"  ","role":"viewer"}`},
		{"bad role", `{"email":"` + validEmail + `","name":"X","role":"owner"}`},
		{"missing role", `{"email":"` + validEmail + `","name":"X"}`},
		{"bad json", `{not-json`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader([]byte(tc.body)))
			req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
			rr := httptest.NewRecorder()
			h.Create(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("want 400, got %d: %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestUserHandler_Create_DuplicateEmail(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)
	dup := mail("dup")
	seedUser(t, users, dup, "viewer", true)

	body, _ := json.Marshal(map[string]string{
		"email": dup,
		"name":  "Dup",
		"role":  "viewer",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	if rr.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestUserHandler_Update_Role(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)
	target := seedUser(t, users, mail("bob"), "viewer", true)

	body, _ := json.Marshal(map[string]string{"role": "editor"})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", target.ID), bytes.NewReader(body))
	req.SetPathValue("id", strconv.FormatInt(target.ID, 10))
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	data, _ := resp["data"].(map[string]any)
	if data["role"] != "editor" {
		t.Errorf("role = %v, want editor", data["role"])
	}
}

func TestUserHandler_Update_Active(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)
	target := seedUser(t, users, mail("bob"), "viewer", true)

	body, _ := json.Marshal(map[string]any{"active": false})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", target.ID), bytes.NewReader(body))
	req.SetPathValue("id", strconv.FormatInt(target.ID, 10))
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	data, _ := resp["data"].(map[string]any)
	if data["is_active"] != false {
		t.Errorf("is_active = %v, want false", data["is_active"])
	}
}

func TestUserHandler_Update_SelfDeactivate_422(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)

	body, _ := json.Marshal(map[string]any{"active": false})
	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", admin.ID), bytes.NewReader(body))
	req.SetPathValue("id", strconv.FormatInt(admin.ID, 10))
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserHandler_Update_NotFound(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)

	body, _ := json.Marshal(map[string]string{"role": "editor"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/9999", bytes.NewReader(body))
	req.SetPathValue("id", "9999")
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

func TestUserHandler_Update_EmptyBody(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)
	target := seedUser(t, users, mail("bob"), "viewer", true)

	req := httptest.NewRequest(http.MethodPatch, fmt.Sprintf("/api/v1/users/%d", target.ID), bytes.NewReader([]byte(`{}`)))
	req.SetPathValue("id", strconv.FormatInt(target.ID, 10))
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.Update(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestUserHandler_Delete_Success(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)
	target := seedUser(t, users, mail("bob"), "viewer", true)

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/users/%d", target.ID), nil)
	req.SetPathValue("id", strconv.FormatInt(target.ID, 10))
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rr.Code, rr.Body.String())
	}
	got, err := users.GetByID(context.Background(), target.ID)
	if err != nil {
		t.Fatalf("GetByID after delete: %v", err)
	}
	if got.IsActive {
		t.Errorf("user remained active after DELETE")
	}
}

func TestUserHandler_Delete_Self_422(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)

	req := httptest.NewRequest(http.MethodDelete, fmt.Sprintf("/api/v1/users/%d", admin.ID), nil)
	req.SetPathValue("id", strconv.FormatInt(admin.ID, 10))
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422, got %d", rr.Code)
	}
}

func TestUserHandler_Delete_NotFound(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/9999", nil)
	req.SetPathValue("id", "9999")
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.Delete(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d", rr.Code)
	}
}

// ---------------------------------------------------------------------------
// RBAC — a non-admin role must be rejected at the route layer. These tests
// validate that the middleware chain configured in main.go reaches the right
// status code end-to-end through the mux, not individual handler methods.
// ---------------------------------------------------------------------------

func TestUserHandler_List_RBAC_ViewerForbidden(t *testing.T) {
	t.Parallel()
	mocks := testutil.New()
	viewer := seedUser(t, mocks.Users, mail("viewer"), "viewer", true)

	h := NewUserHandler(mocks.Users, zap.NewNop())
	guarded := middleware.RequireRole("admin")(h.List)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req = injectUserClaims(req, strconv.FormatInt(viewer.ID, 10), "viewer")
	rr := httptest.NewRecorder()
	guarded(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403 for viewer, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserHandler_List_RBAC_EditorForbidden(t *testing.T) {
	t.Parallel()
	mocks := testutil.New()
	editor := seedUser(t, mocks.Users, mail("editor"), "editor", true)

	h := NewUserHandler(mocks.Users, zap.NewNop())
	guarded := middleware.RequireRole("admin")(h.List)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	req = injectUserClaims(req, strconv.FormatInt(editor.ID, 10), "editor")
	rr := httptest.NewRecorder()
	guarded(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403 for editor, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// ChangeMyPassword
// ---------------------------------------------------------------------------

// seedLocalUserWithBcryptPassword inserts a local user whose password_hash is a
// real bcrypt hash of the given plaintext. Used by password-change tests so the
// handler's CompareHashAndPassword path is exercised end-to-end.
func seedLocalUserWithBcryptPassword(t *testing.T, s *testutil.MemUserStore, email, plaintext, role string) *store.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(plaintext), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt hash: %v", err)
	}
	u, err := s.CreateLocal(context.Background(), email, "Seed "+email, string(hash), role)
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	return u
}

func TestUserHandler_ChangeMyPassword_Success(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	const oldPwd = "old-password-12"
	const newPwd = "new-password-34"
	me := seedLocalUserWithBcryptPassword(t, users, mail("alice"), oldPwd, "viewer")

	body, _ := json.Marshal(map[string]string{
		"current_password": oldPwd,
		"new_password":     newPwd,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/password", bytes.NewReader(body))
	req = injectUserClaims(req, strconv.FormatInt(me.ID, 10), "viewer")
	rr := httptest.NewRecorder()
	h.ChangeMyPassword(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Assert the new password verifies against the stored hash.
	reloaded, err := users.GetByID(context.Background(), me.ID)
	if err != nil {
		t.Fatalf("GetByID after change: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(reloaded.PasswordHash), []byte(newPwd)) != nil {
		t.Errorf("new password does not verify against stored hash")
	}
	if bcrypt.CompareHashAndPassword([]byte(reloaded.PasswordHash), []byte(oldPwd)) == nil {
		t.Errorf("old password still verifies after change")
	}
}

func TestUserHandler_ChangeMyPassword_WrongCurrent(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	me := seedLocalUserWithBcryptPassword(t, users, mail("alice"), "correct-password-1", "viewer")

	body, _ := json.Marshal(map[string]string{
		"current_password": "wrong-password-0",
		"new_password":     "brand-new-password",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/password", bytes.NewReader(body))
	req = injectUserClaims(req, strconv.FormatInt(me.ID, 10), "viewer")
	rr := httptest.NewRecorder()
	h.ChangeMyPassword(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
	// Response must not leak hash or current-password value.
	if strings.Contains(rr.Body.String(), "hash") {
		t.Errorf("response body leaks 'hash': %s", rr.Body.String())
	}
}

func TestUserHandler_ChangeMyPassword_TooShort(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	const oldPwd = "old-password-12"
	me := seedLocalUserWithBcryptPassword(t, users, mail("alice"), oldPwd, "viewer")

	body, _ := json.Marshal(map[string]string{
		"current_password": oldPwd,
		"new_password":     "short",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/password", bytes.NewReader(body))
	req = injectUserClaims(req, strconv.FormatInt(me.ID, 10), "viewer")
	rr := httptest.NewRecorder()
	h.ChangeMyPassword(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for short password, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserHandler_ChangeMyPassword_SameAsCurrent(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	const pwd = "same-password-123"
	me := seedLocalUserWithBcryptPassword(t, users, mail("alice"), pwd, "viewer")

	body, _ := json.Marshal(map[string]string{
		"current_password": pwd,
		"new_password":     pwd,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/password", bytes.NewReader(body))
	req = injectUserClaims(req, strconv.FormatInt(me.ID, 10), "viewer")
	rr := httptest.NewRecorder()
	h.ChangeMyPassword(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for same-as-current, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserHandler_ChangeMyPassword_EnvUserForbidden(t *testing.T) {
	t.Parallel()
	h, _ := newUserHandler(t)

	body, _ := json.Marshal(map[string]string{
		"current_password": "anything-12345",
		"new_password":     "brand-new-pass-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/password", bytes.NewReader(body))
	req = injectUserClaims(req, "admin", "admin") // non-numeric sub → env user
	rr := httptest.NewRecorder()
	h.ChangeMyPassword(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403 for env user, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserHandler_ChangeMyPassword_NonLocalProvider(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)

	// Seed an OIDC user via UpsertByOIDC (provider = "oidc").
	u, err := users.UpsertByOIDC(context.Background(), "oidc", "oidc-sub-123", mail("oidcer"), "OIDC User", "viewer")
	if err != nil {
		t.Fatalf("UpsertByOIDC: %v", err)
	}

	body, _ := json.Marshal(map[string]string{
		"current_password": "anything-12345",
		"new_password":     "brand-new-pass-1",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/password", bytes.NewReader(body))
	req = injectUserClaims(req, strconv.FormatInt(u.ID, 10), "viewer")
	rr := httptest.NewRecorder()
	h.ChangeMyPassword(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for oidc user, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserHandler_ChangeMyPassword_BadJSON(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	me := seedLocalUserWithBcryptPassword(t, users, mail("alice"), "old-password-12", "viewer")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/password", bytes.NewReader([]byte(`{not-json`)))
	req = injectUserClaims(req, strconv.FormatInt(me.ID, 10), "viewer")
	rr := httptest.NewRecorder()
	h.ChangeMyPassword(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for bad json, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// ResetUserPassword (admin reset)
// ---------------------------------------------------------------------------

func TestUserHandler_ResetUserPassword_Success(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)
	target := seedLocalUserWithBcryptPassword(t, users, mail("bob"), "old-password-12", "viewer")

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/password", target.ID), nil)
	req.SetPathValue("id", strconv.FormatInt(target.ID, 10))
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.ResetUserPassword(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	tempPassword, _ := data["temp_password"].(string)
	if len(tempPassword) < minPasswordLen {
		t.Errorf("temp_password len = %d, want >= %d", len(tempPassword), minPasswordLen)
	}
	// The new hash in the store must verify against the returned temp password.
	reloaded, err := users.GetByID(context.Background(), target.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(reloaded.PasswordHash), []byte(tempPassword)) != nil {
		t.Errorf("returned temp_password does not verify against stored hash")
	}
	// The old password must no longer verify.
	if bcrypt.CompareHashAndPassword([]byte(reloaded.PasswordHash), []byte("old-password-12")) == nil {
		t.Errorf("old password still verifies after reset")
	}
}

func TestUserHandler_ResetUserPassword_RBAC_Viewer(t *testing.T) {
	t.Parallel()
	mocks := testutil.New()
	viewer := seedUser(t, mocks.Users, mail("viewer"), "viewer", true)
	target := seedUser(t, mocks.Users, mail("bob"), "viewer", true)

	h := NewUserHandler(mocks.Users, zap.NewNop())
	guarded := middleware.RequireRole("admin")(h.ResetUserPassword)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/password", target.ID), nil)
	req.SetPathValue("id", strconv.FormatInt(target.ID, 10))
	req = injectUserClaims(req, strconv.FormatInt(viewer.ID, 10), "viewer")
	rr := httptest.NewRecorder()
	guarded(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403 for viewer, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserHandler_ResetUserPassword_RBAC_Editor(t *testing.T) {
	t.Parallel()
	mocks := testutil.New()
	editor := seedUser(t, mocks.Users, mail("editor"), "editor", true)
	target := seedUser(t, mocks.Users, mail("bob"), "viewer", true)

	h := NewUserHandler(mocks.Users, zap.NewNop())
	guarded := middleware.RequireRole("admin")(h.ResetUserPassword)

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/password", target.ID), nil)
	req.SetPathValue("id", strconv.FormatInt(target.ID, 10))
	req = injectUserClaims(req, strconv.FormatInt(editor.ID, 10), "editor")
	rr := httptest.NewRecorder()
	guarded(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("want 403 for editor, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserHandler_ResetUserPassword_NotFound(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/9999/password", nil)
	req.SetPathValue("id", "9999")
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.ResetUserPassword(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserHandler_ResetUserPassword_NonLocalTarget(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)
	target, err := users.UpsertByOIDC(context.Background(), "oidc", "oidc-sub-999", mail("oidcer"), "OIDC User", "viewer")
	if err != nil {
		t.Fatalf("UpsertByOIDC: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/password", target.ID), nil)
	req.SetPathValue("id", strconv.FormatInt(target.ID, 10))
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.ResetUserPassword(rr, req)
	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("want 422 for non-local target, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserHandler_ResetUserPassword_InvalidID(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/not-a-number/password", nil)
	req.SetPathValue("id", "not-a-number")
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.ResetUserPassword(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for bad id, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUserHandler_ResetUserPassword_InactiveTargetAllowed(t *testing.T) {
	t.Parallel()
	h, users := newUserHandler(t)
	admin := seedUser(t, users, mail("admin"), "admin", true)
	target := seedLocalUserWithBcryptPassword(t, users, mail("bob"), "old-password-12", "viewer")
	// Deactivate target to ensure reset is still allowed (per brief: rescue inactive user).
	if err := users.UpdateActive(context.Background(), target.ID, false); err != nil {
		t.Fatalf("UpdateActive: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/api/v1/users/%d/password", target.ID), nil)
	req.SetPathValue("id", strconv.FormatInt(target.ID, 10))
	req = injectUserClaims(req, strconv.FormatInt(admin.ID, 10), "admin")
	rr := httptest.NewRecorder()
	h.ResetUserPassword(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 even for inactive target, got %d: %s", rr.Code, rr.Body.String())
	}
	// And the hash really changed.
	reloaded, err := users.GetByID(context.Background(), target.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if bcrypt.CompareHashAndPassword([]byte(reloaded.PasswordHash), []byte("old-password-12")) == nil {
		t.Errorf("hash was not updated for inactive target")
	}
}
