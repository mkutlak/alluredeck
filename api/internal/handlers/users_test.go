package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// Email fixtures assembled at runtime so the editor does not substitute the
// literal with an anonymisation placeholder.
const (
	emailDomain = "test.local"
)

func mail(local string) string { return local + "@" + emailDomain }

// Passwords for the cast's local accounts and for rotation requests.
const (
	oldPassword = "old-password-12"
	newPassword = "new-password-34"
)

// RBAC for these endpoints lives in the route wrappers and is covered by the
// route×role matrix in cmd/api; these tests call handler methods directly.

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

func idOf(u *store.User) string { return strconv.FormatInt(u.ID, 10) }

// userFixture is a UserHandler wired the way main.go wires it — audit logger,
// refresh-family store, API-key store, JWT manager and the auth middleware's
// is_active cache — over a seeded cast, with handles on every store so table
// rows can assert side effects.
type userFixture struct {
	h           *UserHandler
	users       *testutil.MemUserStore
	families    *testutil.MemRefreshTokenFamilyStore
	keys        *testutil.MemAPIKeyStore
	audit       *testutil.MockAuditLogger
	blacklist   *testutil.MemBlacklist
	jwt         *security.JWTManager
	activeCache *middleware.UserActiveCache

	admin *store.User // local admin; the caller of admin endpoints
	alice *store.User // local viewer; the caller of self-service endpoints
	bob   *store.User // local viewer; the target of admin endpoints
	dave  *store.User // local viewer, deactivated
	oidc  *store.User // OIDC-provisioned viewer without a local password

	aliceFamilies []store.RefreshTokenFamily // two active sessions
	bobFamilies   []store.RefreshTokenFamily // two active sessions; bob also owns two API keys
	oidcFamilies  []store.RefreshTokenFamily // two active sessions, keyed by email as the OIDC callback keys them
	otherFamily   store.RefreshTokenFamily   // an unrelated user's session; carol owns an unrelated API key

	aliceToken, aliceJTI string // a real signed access token for alice
	bobToken, bobKey     string // a real signed access token and a raw API key for bob
	oidcToken            string // a real signed OIDC access token, whose sub is the email
}

func newUserFixture(t *testing.T) *userFixture {
	t.Helper()
	mocks := testutil.New()
	f := &userFixture{
		users:     mocks.Users,
		families:  testutil.NewMemRefreshTokenFamilyStore(),
		keys:      mocks.APIKeys,
		audit:     mocks.Audit,
		blacklist: testutil.NewMemBlacklist(),
	}
	f.jwt = security.NewJWTManager(&config.Config{
		JWTSecret:         "test-secret",
		AccessTokenExpiry: config.DurationSeconds(15 * time.Minute),
	}, f.blacklist, zap.NewNop())
	// A TTL far beyond any test run, so only an explicit invalidation can
	// clear a cached is_active flag.
	f.activeCache = middleware.NewUserActiveCache(f.users, time.Hour, 0)
	f.h = NewUserHandler(f.users, zap.NewNop()).
		WithAuditLogger(f.audit).
		WithFamilyStore(f.families).
		WithAPIKeyStore(f.keys).
		WithJWTManager(f.jwt).
		WithUserActiveCache(f.activeCache)

	f.admin = seedUser(t, f.users, mail("admin"), "admin", true)
	f.alice = seedUser(t, f.users, mail("alice"), "viewer", true)
	f.bob = seedUser(t, f.users, mail("bob"), "viewer", true)
	f.dave = seedUser(t, f.users, mail("dave"), "viewer", false)
	oidc, err := f.users.UpsertByOIDC(context.Background(), "oidc", "oidc-sub-123", mail("oidcer"), "OIDC User", "viewer")
	if err != nil {
		t.Fatalf("UpsertByOIDC: %v", err)
	}
	f.oidc = oidc

	f.aliceFamilies = []store.RefreshTokenFamily{seedActiveFamily(t, f.families, idOf(f.alice)), seedActiveFamily(t, f.families, idOf(f.alice))}
	f.bobFamilies = []store.RefreshTokenFamily{seedActiveFamily(t, f.families, idOf(f.bob)), seedActiveFamily(t, f.families, idOf(f.bob))}
	f.oidcFamilies = []store.RefreshTokenFamily{seedActiveFamily(t, f.families, f.oidc.Email), seedActiveFamily(t, f.families, f.oidc.Email)}
	f.otherFamily = seedActiveFamily(t, f.families, "999")
	f.bobKey = seedAPIKey(t, f.keys, f.bob.Email, "ci-1")
	seedAPIKey(t, f.keys, f.bob.Email, "ci-2")
	seedAPIKey(t, f.keys, mail("carol"), "carol-key")

	f.aliceToken, _, f.aliceJTI, _, err = f.jwt.GenerateTokensForFamily(idOf(f.alice), "viewer", "local", "")
	if err != nil {
		t.Fatalf("GenerateTokensForFamily: %v", err)
	}
	f.bobToken, _, err = f.jwt.GenerateTokens(idOf(f.bob), "viewer", "local")
	if err != nil {
		t.Fatalf("GenerateTokens: %v", err)
	}
	f.oidcToken, _, err = f.jwt.GenerateTokens(f.oidc.Email, "viewer", "oidc")
	if err != nil {
		t.Fatalf("GenerateTokens: %v", err)
	}
	return f
}

// seedUser inserts a local user whose password_hash is a real bcrypt hash of
// oldPassword, so password rows exercise the handler's bcrypt comparison.
func seedUser(t *testing.T, s *testutil.MemUserStore, email, role string, active bool) *store.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(oldPassword), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt hash: %v", err)
	}
	u, err := s.CreateLocal(context.Background(), email, "Seed "+email, string(hash), role)
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

// seedActiveFamily inserts an active refresh-token family for userID into the
// in-memory store and returns the resulting family record.
func seedActiveFamily(t *testing.T, families *testutil.MemRefreshTokenFamilyStore, userID string) store.RefreshTokenFamily {
	t.Helper()
	famID, err := security.NewFamilyID()
	if err != nil {
		t.Fatalf("NewFamilyID: %v", err)
	}
	fam := store.RefreshTokenFamily{
		FamilyID:   famID,
		UserID:     userID,
		Role:       "viewer",
		Provider:   "local",
		CurrentJTI: "jti-" + famID,
		Status:     store.RefreshTokenFamilyStatusActive,
		ExpiresAt:  time.Now().Add(24 * time.Hour),
	}
	if err := families.Create(context.Background(), fam); err != nil {
		t.Fatalf("families.Create: %v", err)
	}
	return fam
}

// seedAPIKey inserts an API key owned by username into the in-memory store and
// returns the raw key, which authenticates through the auth middleware.
func seedAPIKey(t *testing.T, keys *testutil.MemAPIKeyStore, username, name string) string {
	t.Helper()
	raw := security.APIKeyPrefix + name + "-" + username
	if _, err := keys.Create(context.Background(), &store.APIKey{
		Name:     name,
		Prefix:   "ald_test",
		KeyHash:  security.HashAPIKey(raw),
		Username: username,
		Role:     "viewer",
	}); err != nil {
		t.Fatalf("apiKeys.Create: %v", err)
	}
	return raw
}

// errFamilyStore wraps the in-memory family store and returns a fixed error
// from RevokeAllForUser so we can assert the handler tolerates store failures.
type errFamilyStore struct {
	*testutil.MemRefreshTokenFamilyStore
	revokeErr error
}

func (e *errFamilyStore) RevokeAllForUser(_ context.Context, _ string) (int, error) {
	return 0, e.revokeErr
}

// cast maps the "{name}" placeholders rows use to the seeded users.
func (f *userFixture) cast() map[string]*store.User {
	return map[string]*store.User{
		"{admin}": f.admin, "{alice}": f.alice, "{bob}": f.bob, "{dave}": f.dave, "{oidc}": f.oidc,
	}
}

// userRow is one request to a UserHandler method and its expected outcome.
type userRow struct {
	name string
	// as is the caller: "" sends no claims; "{name}" is a cast member (numeric
	// sub, stored role); a bare word is an env-configured account, whose sub
	// is its role name ("admin").
	as     string
	id     string // {id} path value; "{name}" is that cast member's ID
	query  string
	body   string
	bearer bool                               // present alice's real access token
	setup  func(t *testing.T, f *userFixture) // rewire the handler or set up state before the request
	want   int
	check  func(t *testing.T, f *userFixture, rr *httptest.ResponseRecorder)
}

// runUserRows serves each row against a fresh fixture through endpoint,
// asserts the status, then runs the row's side-effect checks.
func runUserRows(t *testing.T, method string, endpoint func(*UserHandler, http.ResponseWriter, *http.Request), rows []userRow) {
	t.Helper()
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newUserFixture(t)
			if tc.setup != nil {
				tc.setup(t, f)
			}
			req := httptest.NewRequest(method, "/api/v1/users?"+tc.query, strings.NewReader(tc.body))
			if tc.id != "" {
				id := tc.id
				if u, ok := f.cast()[id]; ok {
					id = idOf(u)
				}
				req.SetPathValue("id", id)
			}
			if tc.bearer {
				req.Header.Set("Authorization", "Bearer "+f.aliceToken)
			}
			if u, ok := f.cast()[tc.as]; ok {
				req = injectUserClaims(req, idOf(u), u.Role)
			} else if tc.as != "" {
				req = injectUserClaims(req, tc.as, tc.as)
			}
			rr := httptest.NewRecorder()
			endpoint(f.h, rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.check != nil {
				tc.check(t, f, rr)
			}
		})
	}
}

// responseData decodes the "data" object of a JSON envelope response.
func responseData(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	return data
}

// passwordVerifies reports whether pwd matches the user's stored bcrypt hash.
func (f *userFixture) passwordVerifies(t *testing.T, u *store.User, pwd string) bool {
	t.Helper()
	got, err := f.users.GetByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	return bcrypt.CompareHashAndPassword([]byte(got.PasswordHash), []byte(pwd)) == nil
}

// wantAudit asserts exactly n events of action were recorded and returns them.
func (f *userFixture) wantAudit(t *testing.T, action string, n int) []store.AuditEvent {
	t.Helper()
	events := f.audit.EventsByAction(action)
	if len(events) != n {
		t.Fatalf("%s events = %d, want %d", action, len(events), n)
	}
	return events
}

// wantActor asserts evt was recorded with actor as its actor_id.
func wantActor(t *testing.T, evt store.AuditEvent, actor *store.User) {
	t.Helper()
	if evt.ActorID == nil || *evt.ActorID != actor.ID {
		t.Errorf("%s actor_id = %v, want %d", evt.Action, evt.ActorID, actor.ID)
	}
}

// wantSessionsRevoked asserts every family in fams is revoked, the unrelated
// user's session is untouched, and one session.revoke_all event targets sub.
func (f *userFixture) wantSessionsRevoked(t *testing.T, sub string, fams []store.RefreshTokenFamily) {
	t.Helper()
	for i := range fams {
		id := fams[i].FamilyID
		got, err := f.families.GetByID(context.Background(), id)
		if err != nil || got == nil || got.Status != store.RefreshTokenFamilyStatusRevoked {
			t.Errorf("family %s = %v (err %v), want revoked", id, got, err)
		}
	}
	if got, _ := f.families.GetByID(context.Background(), f.otherFamily.FamilyID); got == nil || got.Status != store.RefreshTokenFamilyStatusActive {
		t.Errorf("unrelated family = %v, want active", got)
	}
	if evt := f.wantAudit(t, store.AuditActionSessionRevokeAll, 1)[0]; evt.TargetID != sub {
		t.Errorf("session.revoke_all target_id = %q, want %q", evt.TargetID, sub)
	}
}

// wantKeysDeleted asserts every API key owned by username is gone, the
// unrelated user's key survives, and one cascade-delete event was recorded.
func (f *userFixture) wantKeysDeleted(t *testing.T, username string) {
	t.Helper()
	if n, _ := f.keys.CountByUsername(context.Background(), username); n != 0 {
		t.Errorf("%s API key count = %d, want 0", username, n)
	}
	if n, _ := f.keys.CountByUsername(context.Background(), mail("carol")); n != 1 {
		t.Errorf("unrelated API key count = %d, want 1", n)
	}
	f.wantAudit(t, store.AuditActionAPIKeyCascadeDelete, 1)
}

// wantBobAuth asserts the production AuthMiddleware — sharing the handler's
// JWT manager, API-key store and is_active cache — answers want to the next
// request bearing bob's access token and to the next bearing his API key.
func (f *userFixture) wantBobAuth(t *testing.T, want int) {
	t.Helper()
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	auth := middleware.AuthMiddleware(&config.Config{SecurityEnabled: true}, f.jwt, false, f.keys, f.activeCache)(ok)
	for _, c := range []struct{ label, credential string }{{"access token", f.bobToken}, {"API key", f.bobKey}} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
		req.Header.Set("Authorization", "Bearer "+c.credential)
		rr := httptest.NewRecorder()
		auth(rr, req)
		if rr.Code != want {
			t.Errorf("bob's %s: status = %d, want %d: %s", c.label, rr.Code, want, rr.Body.String())
		}
	}
}

// wantOIDCAuth is wantBobAuth for the OIDC user's access token, which names
// the user by email rather than by numeric ID.
func (f *userFixture) wantOIDCAuth(t *testing.T, want int) {
	t.Helper()
	ok := func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }
	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
	req.Header.Set("Authorization", "Bearer "+f.oidcToken)
	rr := httptest.NewRecorder()
	middleware.AuthMiddleware(&config.Config{SecurityEnabled: true}, f.jwt, false, f.keys, f.activeCache)(ok)(rr, req)
	if rr.Code != want {
		t.Errorf("OIDC access token: status = %d, want %d: %s", rr.Code, want, rr.Body.String())
	}
}

// wantOIDCAccessEnded asserts every email-keyed session of the OIDC user is
// revoked, the unrelated user's session is untouched, and the OIDC access
// token is refused on the next request.
func (f *userFixture) wantOIDCAccessEnded(t *testing.T) {
	t.Helper()
	for i := range f.oidcFamilies {
		id := f.oidcFamilies[i].FamilyID
		if got, err := f.families.GetByID(context.Background(), id); err != nil || got == nil || got.Status != store.RefreshTokenFamilyStatusRevoked {
			t.Errorf("OIDC family %s = %v (err %v), want revoked", id, got, err)
		}
	}
	if got, _ := f.families.GetByID(context.Background(), f.otherFamily.FamilyID); got == nil || got.Status != store.RefreshTokenFamilyStatusActive {
		t.Errorf("unrelated family = %v, want active", got)
	}
	f.wantOIDCAuth(t, http.StatusUnauthorized)
}

// wantUsers checks a List response holds n users and reports total n.
func wantUsers(n int) func(*testing.T, *userFixture, *httptest.ResponseRecorder) {
	return func(t *testing.T, _ *userFixture, rr *httptest.ResponseRecorder) {
		t.Helper()
		data := responseData(t, rr)
		if users, _ := data["users"].([]any); len(users) != n {
			t.Errorf("users length = %d, want %d", len(users), n)
		}
		if total, _ := data["total"].(float64); int(total) != n {
			t.Errorf("total = %v, want %d", data["total"], n)
		}
	}
}

func passwordBody(current, next string) string {
	return `{"current_password":"` + current + `","new_password":"` + next + `"}`
}

func TestUserHandler_Me(t *testing.T) {
	t.Parallel()
	runUserRows(t, http.MethodGet, (*UserHandler).Me, []userRow{
		{name: "db user", as: "{alice}", want: http.StatusOK,
			check: func(t *testing.T, f *userFixture, rr *httptest.ResponseRecorder) {
				data := responseData(t, rr)
				if data["email"] != f.alice.Email {
					t.Errorf("email = %v, want %s", data["email"], f.alice.Email)
				}
				if _, leaked := data["password_hash"]; leaked {
					t.Errorf("password_hash must not be in response body")
				}
			}},
		{name: "missing claims", want: http.StatusUnauthorized},
		// Env admin/viewer users have a non-numeric JWT sub (e.g. "admin"). Me()
		// must return a synthetic profile with provider="env" instead of
		// 401'ing, so the frontend AuthGuard can render the Profile page and
		// does not loop into a refresh→reset cycle.
		{name: "env user synthetic profile", as: "admin", want: http.StatusOK,
			check: func(t *testing.T, _ *userFixture, rr *httptest.ResponseRecorder) {
				data := responseData(t, rr)
				for field, want := range map[string]any{"provider": "env", "role": "admin", "is_active": true, "name": "admin"} {
					if data[field] != want {
						t.Errorf("%s = %v, want %v", field, data[field], want)
					}
				}
				// ID must be a sentinel value signalling "not a DB row".
				if id, ok := data["id"].(float64); !ok || id != 0 {
					t.Errorf("id = %v, want 0 sentinel", data["id"])
				}
			}},
	})
}

func TestUserHandler_UpdateMe(t *testing.T) {
	t.Parallel()
	runUserRows(t, http.MethodPatch, (*UserHandler).UpdateMe, []userRow{
		{name: "renames caller", as: "{alice}", body: `{"name":"New Name"}`, want: http.StatusOK,
			check: func(t *testing.T, _ *userFixture, rr *httptest.ResponseRecorder) {
				if data := responseData(t, rr); data["name"] != "New Name" {
					t.Errorf("name = %v, want New Name", data["name"])
				}
			}},
		// Env-configured accounts have no users row: refuse rather than let
		// callers silently "succeed" against a non-existent row.
		{name: "env user forbidden", as: "admin", body: `{"name":"New Name"}`, want: http.StatusForbidden},
		{name: "missing name", as: "{alice}", body: `{}`, want: http.StatusBadRequest},
		{name: "empty name", as: "{alice}", body: `{"name":"  "}`, want: http.StatusBadRequest},
		{name: "name too long", as: "{alice}", body: `{"name":"` + strings.Repeat("x", 121) + `"}`, want: http.StatusBadRequest},
		{name: "bad json", as: "{alice}", body: `{not-json`, want: http.StatusBadRequest},
	})
}

func TestUserHandler_List(t *testing.T) {
	t.Parallel()
	// The cast is admin, alice, bob, oidc (active viewers but admin) and dave
	// (inactive viewer). The filter rows prove role and active reach the store.
	runUserRows(t, http.MethodGet, (*UserHandler).List, []userRow{
		{name: "all users", as: "{admin}", query: "limit=10&offset=0", want: http.StatusOK, check: wantUsers(5)},
		{name: "role filter", as: "{admin}", query: "role=viewer", want: http.StatusOK, check: wantUsers(4)},
		{name: "role and active filters", as: "{admin}", query: "role=viewer&active=false", want: http.StatusOK, check: wantUsers(1)},
		{name: "invalid role", as: "{admin}", query: "role=owner", want: http.StatusBadRequest},
	})
}

func TestUserHandler_Get(t *testing.T) {
	t.Parallel()
	runUserRows(t, http.MethodGet, (*UserHandler).Get, []userRow{
		{name: "found", as: "{admin}", id: "{bob}", want: http.StatusOK},
		{name: "not found", as: "{admin}", id: "9999", want: http.StatusNotFound},
		{name: "invalid id", as: "{admin}", id: "not-a-number", want: http.StatusBadRequest},
	})
}

func TestUserHandler_Create(t *testing.T) {
	t.Parallel()
	validEmail := mail("ok")
	runUserRows(t, http.MethodPost, (*UserHandler).Create, []userRow{
		{name: "creates user with temp password", as: "{admin}",
			body: `{"email":"` + mail("new") + `","name":"New User","role":"viewer"}`, want: http.StatusCreated,
			check: func(t *testing.T, f *userFixture, rr *httptest.ResponseRecorder) {
				data := responseData(t, rr)
				if tempPassword, _ := data["temp_password"].(string); len(tempPassword) < minPasswordLen {
					t.Errorf("temp_password len = %d, want >= %d", len(tempPassword), minPasswordLen)
				}
				user, _ := data["user"].(map[string]any)
				if user["email"] != mail("new") || user["role"] != "viewer" {
					t.Errorf("user = %v, want email %s role viewer", user, mail("new"))
				}
				wantActor(t, f.wantAudit(t, store.AuditActionUserCreate, 1)[0], f.admin)
			}},
		{name: "bad email", as: "{admin}", body: `{"email":"not-an-email","name":"X","role":"viewer"}`, want: http.StatusBadRequest},
		{name: "empty name", as: "{admin}", body: `{"email":"` + validEmail + `","name":"  ","role":"viewer"}`, want: http.StatusBadRequest},
		{name: "bad role", as: "{admin}", body: `{"email":"` + validEmail + `","name":"X","role":"owner"}`, want: http.StatusBadRequest},
		{name: "missing role", as: "{admin}", body: `{"email":"` + validEmail + `","name":"X"}`, want: http.StatusBadRequest},
		{name: "bad json", as: "{admin}", body: `{not-json`, want: http.StatusBadRequest},
		{name: "duplicate email", as: "{admin}", body: `{"email":"` + mail("bob") + `","name":"Dup","role":"viewer"}`, want: http.StatusConflict},
	})
}

func TestUserHandler_Update(t *testing.T) {
	t.Parallel()
	runUserRows(t, http.MethodPatch, (*UserHandler).Update, []userRow{
		{name: "role", as: "{admin}", id: "{bob}", body: `{"role":"editor"}`, want: http.StatusOK,
			check: func(t *testing.T, f *userFixture, rr *httptest.ResponseRecorder) {
				if data := responseData(t, rr); data["role"] != "editor" {
					t.Errorf("role = %v, want editor", data["role"])
				}
				f.wantAudit(t, store.AuditActionUserUpdateRole, 1)
			}},
		// Deactivation ends access at once, as DELETE does: every session is
		// revoked and, although earlier requests cached bob as active, his
		// access token and API key are refused on the very next request. The
		// keys themselves are kept so that reactivation restores them.
		{name: "deactivate revokes access", as: "{admin}", id: "{bob}", body: `{"active":false}`,
			setup: func(t *testing.T, f *userFixture) { f.wantBobAuth(t, http.StatusOK) },
			want:  http.StatusOK,
			check: func(t *testing.T, f *userFixture, rr *httptest.ResponseRecorder) {
				if data := responseData(t, rr); data["is_active"] != false {
					t.Errorf("is_active = %v, want false", data["is_active"])
				}
				f.wantAudit(t, store.AuditActionUserUpdateActive, 1)
				f.wantSessionsRevoked(t, idOf(f.bob), f.bobFamilies)
				f.wantBobAuth(t, http.StatusUnauthorized)
				if n, _ := f.keys.CountByUsername(context.Background(), f.bob.Email); n != 2 {
					t.Errorf("bob API key count = %d, want 2 kept for reactivation", n)
				}
			}},
		// Revocation is best-effort: a store hiccup must never fail the
		// deactivation, and access still ends on the next request.
		{name: "revocation failure does not fail handler", as: "{admin}", id: "{bob}", body: `{"active":false}`,
			setup: func(t *testing.T, f *userFixture) {
				f.wantBobAuth(t, http.StatusOK)
				f.h.WithFamilyStore(&errFamilyStore{MemRefreshTokenFamilyStore: f.families, revokeErr: errors.New("synthetic revoke failure")})
			},
			want: http.StatusOK,
			check: func(t *testing.T, f *userFixture, _ *httptest.ResponseRecorder) {
				f.wantAudit(t, store.AuditActionSessionRevokeAll, 0)
				f.wantBobAuth(t, http.StatusUnauthorized)
			}},
		// Reactivation undoes it on the next request too, not after the cache
		// TTL, and revokes nothing.
		{name: "reactivate restores access", as: "{admin}", id: "{bob}", body: `{"active":true}`,
			setup: func(t *testing.T, f *userFixture) {
				if err := f.users.UpdateActive(context.Background(), f.bob.ID, false); err != nil {
					t.Fatalf("UpdateActive: %v", err)
				}
				f.wantBobAuth(t, http.StatusUnauthorized)
			},
			want: http.StatusOK,
			check: func(t *testing.T, f *userFixture, _ *httptest.ResponseRecorder) {
				f.wantBobAuth(t, http.StatusOK)
				f.wantAudit(t, store.AuditActionSessionRevokeAll, 0)
			}},
		// An OIDC user's sessions and access token name them by email, not
		// ID; deactivation ends those just the same.
		{name: "deactivate revokes OIDC access", as: "{admin}", id: "{oidc}", body: `{"active":false}`,
			setup: func(t *testing.T, f *userFixture) { f.wantOIDCAuth(t, http.StatusOK) },
			want:  http.StatusOK,
			check: func(t *testing.T, f *userFixture, _ *httptest.ResponseRecorder) { f.wantOIDCAccessEnded(t) }},
		{name: "self-deactivate", as: "{admin}", id: "{admin}", body: `{"active":false}`, want: http.StatusUnprocessableEntity},
		{name: "not found", as: "{admin}", id: "9999", body: `{"role":"editor"}`, want: http.StatusNotFound},
		{name: "empty body", as: "{admin}", id: "{bob}", body: `{}`, want: http.StatusBadRequest},
	})
}

func TestUserHandler_Delete(t *testing.T) {
	t.Parallel()
	runUserRows(t, http.MethodDelete, (*UserHandler).Delete, []userRow{
		// The canonical "remove access on departure" workflow: deactivate,
		// revoke every session and cascade-delete every API key (F-2); bob's
		// access token, cached as active by earlier requests, is refused on
		// the very next request.
		{name: "deactivates and revokes access", as: "{admin}", id: "{bob}",
			setup: func(t *testing.T, f *userFixture) { f.wantBobAuth(t, http.StatusOK) },
			want:  http.StatusNoContent,
			check: func(t *testing.T, f *userFixture, _ *httptest.ResponseRecorder) {
				got, err := f.users.GetByID(context.Background(), f.bob.ID)
				if err != nil {
					t.Fatalf("GetByID after delete: %v", err)
				}
				if got.IsActive {
					t.Errorf("user remained active after DELETE")
				}
				if evt := f.wantAudit(t, store.AuditActionUserDelete, 1)[0]; evt.TargetID != idOf(f.bob) {
					t.Errorf("users.delete target_id = %q, want %s", evt.TargetID, idOf(f.bob))
				}
				f.wantSessionsRevoked(t, idOf(f.bob), f.bobFamilies)
				f.wantKeysDeleted(t, f.bob.Email)
				f.wantBobAuth(t, http.StatusUnauthorized)
			}},
		{name: "revokes OIDC access", as: "{admin}", id: "{oidc}",
			setup: func(t *testing.T, f *userFixture) { f.wantOIDCAuth(t, http.StatusOK) },
			want:  http.StatusNoContent,
			check: func(t *testing.T, f *userFixture, _ *httptest.ResponseRecorder) { f.wantOIDCAccessEnded(t) }},
		{name: "self", as: "{admin}", id: "{admin}", want: http.StatusUnprocessableEntity},
		{name: "not found", as: "{admin}", id: "9999", want: http.StatusNotFound},
	})
}

func TestUserHandler_ChangeMyPassword(t *testing.T) {
	t.Parallel()
	runUserRows(t, http.MethodPost, (*UserHandler).ChangeMyPassword, []userRow{
		// F-2: a rotation revokes every refresh family of the caller and
		// blacklists the access JTI presented in the request.
		{name: "rotates password and revokes sessions", as: "{alice}", bearer: true,
			body: passwordBody(oldPassword, newPassword), want: http.StatusNoContent,
			check: func(t *testing.T, f *userFixture, _ *httptest.ResponseRecorder) {
				if !f.passwordVerifies(t, f.alice, newPassword) {
					t.Errorf("new password does not verify against stored hash")
				}
				if f.passwordVerifies(t, f.alice, oldPassword) {
					t.Errorf("old password still verifies after change")
				}
				evt := f.wantAudit(t, store.AuditActionPasswordChange, 1)[0]
				if evt.Outcome != store.AuditOutcomeSuccess || evt.TargetID != idOf(f.alice) {
					t.Errorf("password_change outcome/target = %q/%q, want success/%s", evt.Outcome, evt.TargetID, idOf(f.alice))
				}
				wantActor(t, evt, f.alice)
				f.wantSessionsRevoked(t, idOf(f.alice), f.aliceFamilies)
				if blacklisted, _ := f.blacklist.IsBlacklisted(context.Background(), f.aliceJTI); !blacklisted {
					t.Errorf("access JTI %q is not blacklisted after password change", f.aliceJTI)
				}
			}},
		// Revocation is best-effort: a store hiccup must never block the
		// password change itself, and no revoke audit is emitted for it.
		{name: "revocation failure does not fail handler", as: "{alice}",
			setup: func(_ *testing.T, f *userFixture) {
				f.h.WithFamilyStore(&errFamilyStore{MemRefreshTokenFamilyStore: f.families, revokeErr: errors.New("synthetic revoke failure")})
			},
			body: passwordBody(oldPassword, newPassword), want: http.StatusNoContent,
			check: func(t *testing.T, f *userFixture, _ *httptest.ResponseRecorder) {
				if !f.passwordVerifies(t, f.alice, newPassword) {
					t.Errorf("password not rotated despite handler succeeding")
				}
				f.wantAudit(t, store.AuditActionSessionRevokeAll, 0)
			}},
		// 400, not 401: the session is authenticated; a 401 would log the user out.
		{name: "wrong current", as: "{alice}", body: passwordBody("wrong-password-0", "brand-new-password"), want: http.StatusBadRequest,
			check: func(t *testing.T, _ *userFixture, rr *httptest.ResponseRecorder) {
				if strings.Contains(rr.Body.String(), "hash") {
					t.Errorf("response body leaks 'hash': %s", rr.Body.String())
				}
			}},
		{name: "too short", as: "{alice}", body: passwordBody(oldPassword, "short"), want: http.StatusBadRequest},
		{name: "same as current", as: "{alice}", body: passwordBody(oldPassword, oldPassword), want: http.StatusBadRequest},
		{name: "env user forbidden", as: "admin", body: passwordBody("anything-12345", "brand-new-pass-1"), want: http.StatusForbidden},
		{name: "non-local provider", as: "{oidc}", body: passwordBody("anything-12345", "brand-new-pass-1"), want: http.StatusUnprocessableEntity},
		{name: "bad json", as: "{alice}", body: `{not-json`, want: http.StatusBadRequest},
	})
}

func TestUserHandler_ResetUserPassword(t *testing.T) {
	t.Parallel()
	runUserRows(t, http.MethodPost, (*UserHandler).ResetUserPassword, []userRow{
		// F-2: the stolen session and API keys of the target must stop
		// authenticating immediately.
		{name: "issues temp password and revokes access", as: "{admin}", id: "{bob}", want: http.StatusOK,
			check: func(t *testing.T, f *userFixture, rr *httptest.ResponseRecorder) {
				tempPassword, _ := responseData(t, rr)["temp_password"].(string)
				if len(tempPassword) < minPasswordLen {
					t.Errorf("temp_password len = %d, want >= %d", len(tempPassword), minPasswordLen)
				}
				if !f.passwordVerifies(t, f.bob, tempPassword) {
					t.Errorf("returned temp_password does not verify against stored hash")
				}
				if f.passwordVerifies(t, f.bob, oldPassword) {
					t.Errorf("old password still verifies after reset")
				}
				evt := f.wantAudit(t, store.AuditActionPasswordReset, 1)[0]
				wantActor(t, evt, f.admin)
				if evt.TargetID != idOf(f.bob) {
					t.Errorf("password_reset target_id = %q, want %s", evt.TargetID, idOf(f.bob))
				}
				f.wantSessionsRevoked(t, idOf(f.bob), f.bobFamilies)
				f.wantKeysDeleted(t, f.bob.Email)
			}},
		// Admins rescue deactivated accounts by resetting them.
		{name: "inactive target allowed", as: "{admin}", id: "{dave}", want: http.StatusOK,
			check: func(t *testing.T, f *userFixture, _ *httptest.ResponseRecorder) {
				if f.passwordVerifies(t, f.dave, oldPassword) {
					t.Errorf("hash was not updated for inactive target")
				}
			}},
		{name: "not found", as: "{admin}", id: "9999", want: http.StatusNotFound},
		{name: "non-local target", as: "{admin}", id: "{oidc}", want: http.StatusUnprocessableEntity},
		{name: "invalid id", as: "{admin}", id: "not-a-number", want: http.StatusBadRequest},
	})
}
