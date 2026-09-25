package handlers

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/security"
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

// apiKeyDBEmail is the email of the fixture's DB-backed user. JWTs carry such
// users' numeric ID as sub, but their keys are stored under the email.
const apiKeyDBEmail = "admin" + "@" + "example.com"

// apiKeyFixture is an APIKeyHandler over in-memory stores holding two keys for
// env user alice, one for bob and one for the DB-backed user.
type apiKeyFixture struct {
	h        *APIKeyHandler
	mocks    *testutil.MockStores
	dbSub    string // the DB-backed user's JWT sub (numeric ID)
	aliceKey int64
	dbKey    int64
}

func newAPIKeyFixture(t *testing.T) *apiKeyFixture {
	t.Helper()
	ctx := context.Background()
	mocks := testutil.New()
	f := &apiKeyFixture{h: NewAPIKeyHandler(mocks.APIKeys, mocks.Users).WithAuditLogger(mocks.Audit), mocks: mocks}
	u, err := mocks.Users.UpsertByOIDC(ctx, "local", "sub-1", apiKeyDBEmail, "Admin", "admin")
	if err != nil {
		t.Fatal(err)
	}
	f.dbSub = strconv.FormatInt(u.ID, 10)
	seed := func(name, username string) int64 {
		k, err := mocks.APIKeys.Create(ctx, &store.APIKey{Name: name, Prefix: "ald_a1b2c3d4", KeyHash: "deadbeef" + name, Username: username, Role: "admin"})
		if err != nil {
			t.Fatal(err)
		}
		return k.ID
	}
	f.aliceKey = seed("key-a", "alice")
	seed("key-b", "alice")
	seed("key-bob", "bob")
	f.dbKey = seed("ci-key", apiKeyDBEmail)
	return f
}

// apiKeyRow is one request to an APIKeyHandler method. as is the JWT sub,
// "{db}" standing for the DB-backed user; id is the {id} path value, with
// "{alice}" and "{db}" standing for those users' seeded keys.
type apiKeyRow struct {
	name  string
	as    string
	id    string
	body  string
	setup func(t *testing.T, f *apiKeyFixture)
	want  int
	json  map[string]any
	check func(t *testing.T, f *apiKeyFixture, body any)
}

func runAPIKeyRows(t *testing.T, method string, endpoint func(*APIKeyHandler, http.ResponseWriter, *http.Request), rows []apiKeyRow) {
	t.Helper()
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newAPIKeyFixture(t)
			if tc.setup != nil {
				tc.setup(t, f)
			}
			sub := strings.ReplaceAll(tc.as, "{db}", f.dbSub)
			id := strings.NewReplacer("{alice}", strconv.FormatInt(f.aliceKey, 10), "{db}", strconv.FormatInt(f.dbKey, 10)).Replace(tc.id)
			fn := func(w http.ResponseWriter, r *http.Request) { endpoint(f.h, w, injectClaims(r, sub, "admin")) }
			code, body := serveJSON(t, fn, method, "/api/v1/api-keys", tc.body, "id", id)
			if code != tc.want {
				t.Fatalf("status = %d, want %d: %v", code, tc.want, body)
			}
			wantJSON(t, body, tc.json)
			if tc.check != nil {
				tc.check(t, f, body)
			}
		})
	}
}

// wantAPIKeyAudit asserts exactly one action event was recorded and returns it.
func wantAPIKeyAudit(t *testing.T, f *apiKeyFixture, action string) store.AuditEvent {
	t.Helper()
	events := f.mocks.Audit.EventsByAction(action)
	if len(events) != 1 {
		t.Fatalf("%s events = %d, want 1", action, len(events))
	}
	return events[0]
}

func TestAPIKeyHandler_List(t *testing.T) {
	t.Parallel()
	runAPIKeyRows(t, http.MethodGet, (*APIKeyHandler).List, []apiKeyRow{
		{name: "no keys", as: "carol", want: http.StatusOK, json: map[string]any{"data#": 0}},
		{name: "only the caller's keys", as: "alice", want: http.StatusOK, json: map[string]any{"data#": 2}},
		// A DB user's numeric sub resolves to the email the key is stored under.
		{name: "numeric sub", as: "{db}", want: http.StatusOK, json: map[string]any{"data#": 1, "data.0.name": "ci-key"}},
	})
}

func TestAPIKeyHandler_Create(t *testing.T) {
	t.Parallel()
	past := time.Now().Add(-time.Hour).Format(time.RFC3339)
	runAPIKeyRows(t, http.MethodPost, (*APIKeyHandler).Create, []apiKeyRow{
		{name: "creates key", as: "alice", body: `{"name":"my-ci-key"}`, want: http.StatusCreated,
			json: map[string]any{"data.name": "my-ci-key", "data.username": "alice"},
			check: func(t *testing.T, f *apiKeyFixture, body any) {
				if key, _ := jsonAt(body, "data.key").(string); !strings.HasPrefix(key, security.APIKeyPrefix) {
					t.Errorf("key = %q, want the %s prefix", key, security.APIKeyPrefix)
				}
				if evt := wantAPIKeyAudit(t, f, store.AuditActionAPIKeyCreate); evt.ActorLabel != "alice" {
					t.Errorf("actor_label = %q, want alice", evt.ActorLabel)
				}
			}},
		{name: "allow_mcp_writes persists", as: "carol", body: `{"name":"mcp-key","allow_mcp_writes":true}`, want: http.StatusCreated,
			json: map[string]any{"data.allow_mcp_writes": true},
			check: func(t *testing.T, f *apiKeyFixture, body any) {
				if keys, _ := f.mocks.APIKeys.ListByUsername(context.Background(), "carol"); len(keys) != 1 || !keys[0].AllowMCPWrites {
					t.Errorf("stored keys = %+v, want one with AllowMCPWrites", keys)
				}
			}},
		// Keys are stored under the DB user's email, not the numeric JWT sub,
		// and under an env user's literal sub.
		{name: "numeric sub stores email", as: "{db}", body: `{"name":"ci-key-2"}`, want: http.StatusCreated, json: map[string]any{"data.username": apiKeyDBEmail}},
		{name: "env user sub stores literal", as: "admin", body: `{"name":"env-key"}`, want: http.StatusCreated, json: map[string]any{"data.username": "admin"}},
		{name: "five key limit", as: "alice", body: `{"name":"key-6"}`, want: http.StatusConflict,
			setup: func(t *testing.T, f *apiKeyFixture) {
				for _, name := range []string{"key-c", "key-d", "key-e"} {
					if _, err := f.mocks.APIKeys.Create(context.Background(), &store.APIKey{Name: name, KeyHash: name, Username: "alice"}); err != nil {
						t.Fatal(err)
					}
				}
			}},
		{name: "missing name", as: "alice", body: `{}`, want: http.StatusBadRequest},
		{name: "past expires_at", as: "alice", body: `{"name":"expired-key","expires_at":"` + past + `"}`, want: http.StatusBadRequest},
	})
}

func TestAPIKeyHandler_Delete(t *testing.T) {
	t.Parallel()
	runAPIKeyRows(t, http.MethodDelete, (*APIKeyHandler).Delete, []apiKeyRow{
		{name: "own key", as: "alice", id: "{alice}", want: http.StatusOK,
			check: func(t *testing.T, f *apiKeyFixture, body any) {
				if evt := wantAPIKeyAudit(t, f, store.AuditActionAPIKeyDelete); evt.TargetID != strconv.FormatInt(f.aliceKey, 10) {
					t.Errorf("target_id = %q, want %d", evt.TargetID, f.aliceKey)
				}
			}},
		// Another user's key is indistinguishable from a missing one (IDOR).
		{name: "another user's key", as: "bob", id: "{alice}", want: http.StatusNotFound},
		{name: "numeric sub", as: "{db}", id: "{db}", want: http.StatusOK},
		{name: "non-existent key", as: "alice", id: "9999", want: http.StatusNotFound},
	})
}
