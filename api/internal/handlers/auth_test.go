package handlers

import (
	"bytes"
	"context"
	"encoding/json"
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

func testAuthConfig() *config.Config {
	cfg := &config.Config{
		SecurityEnabled:    true,
		AdminUser:          "admin",
		AdminPass:          "password",
		JWTSecret:          "test-secret",
		AccessTokenExpiry:  config.DurationSeconds(15 * time.Minute),
		RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
	}
	// Hash passwords so bcrypt comparison works in tests.
	if err := cfg.HashPasswords(); err != nil {
		panic("testAuthConfig: " + err.Error())
	}
	return cfg
}

// authPassword is the password of the fixture's local DB users.
const authPassword = "s3cret-password"

// authEmail assembles a fixture email at runtime so editors do not replace the
// literal with an anonymisation placeholder.
func authEmail(local string) string { return local + "@" + "test.local" }

// authFixture is an AuthHandler wired the way main.go wires it — user store,
// refresh-token families, audit logger — over env admin "admin"/"password",
// local DB users ed (active editor) and bob (inactive viewer), and an active
// refresh-token family for "alice".
type authFixture struct {
	h        *AuthHandler
	cfg      *config.Config
	jwt      *security.JWTManager
	users    *testutil.MemUserStore
	families *testutil.MemRefreshTokenFamilyStore
	audit    *testutil.MockAuditLogger
	ed       *store.User
	famID    string // alice's refresh-token family
	refresh  string // alice's current refresh token in famID
}

func newAuthFixture(t *testing.T) *authFixture {
	t.Helper()
	f := &authFixture{
		cfg:      testAuthConfig(),
		users:    testutil.NewMemUserStore(),
		families: testutil.NewMemRefreshTokenFamilyStore(),
		audit:    testutil.NewMockAuditLogger(),
	}
	f.jwt = security.NewJWTManager(f.cfg, testutil.NewMemBlacklist(), zap.NewNop())
	f.h = NewAuthHandler(f.cfg, f.jwt, f.families).WithUserStore(f.users).WithAuditLogger(f.audit)
	f.ed = seedLocalUserWithPassword(t, f.users, authEmail("ed"), authPassword, "editor", true)
	seedLocalUserWithPassword(t, f.users, authEmail("bob"), authPassword, "viewer", false)

	famID, err := security.NewFamilyID()
	if err != nil {
		t.Fatalf("NewFamilyID: %v", err)
	}
	f.famID = famID
	var jti string
	f.refresh, jti = f.mintRefresh(t, famID)
	if err := f.families.Create(context.Background(), store.RefreshTokenFamily{
		FamilyID: famID, UserID: "alice", Role: "admin", Provider: "local", CurrentJTI: jti,
		Status: store.RefreshTokenFamilyStatusActive, ExpiresAt: time.Now().Add(f.cfg.RefreshTokenExpiry.Duration()),
	}); err != nil {
		t.Fatalf("families.Create: %v", err)
	}
	return f
}

// seedLocalUserWithPassword registers a local user whose password_hash is a
// real bcrypt hash of password, so Login verifies it.
func seedLocalUserWithPassword(t *testing.T, users *testutil.MemUserStore, email, password, role string, active bool) *store.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	u, err := users.CreateLocal(context.Background(), email, "Seed "+email, string(hash), role)
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	if !active {
		if err := users.UpdateActive(context.Background(), u.ID, false); err != nil {
			t.Fatalf("UpdateActive: %v", err)
		}
	}
	return u
}

// mintRefresh returns a refresh token for alice in family famID ("" mints one
// without a fam claim, the legacy GenerateTokens shape) and its JTI.
func (f *authFixture) mintRefresh(t *testing.T, famID string) (token, jti string) {
	t.Helper()
	_, token, _, jti, err := f.jwt.GenerateTokensForFamily("alice", "admin", "local", famID)
	if err != nil {
		t.Fatalf("GenerateTokensForFamily: %v", err)
	}
	return token, jti
}

func (f *authFixture) login(t *testing.T, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(LoginRequest{Username: username, Password: password})
	req := httptest.NewRequest(http.MethodPost, "/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	f.h.Login(rr, req)
	return rr
}

// refreshWith posts to /auth/refresh with token as the refresh_jwt cookie;
// "" sends no cookie.
func (f *authFixture) refreshWith(token string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	if token != "" {
		req.AddCookie(&http.Cookie{Name: "refresh_jwt", Value: token})
	}
	rr := httptest.NewRecorder()
	f.h.Refresh(rr, req)
	return rr
}

// family returns alice's refresh-token family as stored.
func (f *authFixture) family(t *testing.T) *store.RefreshTokenFamily {
	t.Helper()
	fam, err := f.families.GetByID(context.Background(), f.famID)
	if err != nil || fam == nil {
		t.Fatalf("family %s = %v (err %v)", f.famID, fam, err)
	}
	return fam
}

// events asserts exactly n audit events of action were recorded and returns them.
func (f *authFixture) events(t *testing.T, action string, n int) []store.AuditEvent {
	t.Helper()
	evts := f.audit.EventsByAction(action)
	if len(evts) != n {
		t.Fatalf("%s events = %d, want %d", action, len(evts), n)
	}
	return evts
}

func authCookie(rr *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rr.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

func authData(t *testing.T, rr *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return resp.Data
}

// loginStep is one POST /login and the status it must get.
type loginStep struct {
	user, pass string
	want       int
}

func wrongAdminLogins(n int) []loginStep {
	steps := make([]loginStep, n)
	for i := range steps {
		steps[i] = loginStep{"admin", "wrong", http.StatusUnauthorized}
	}
	return steps
}

func TestAuthHandler_Login(t *testing.T) {
	t.Parallel()
	const newPassword34 = "new-password-34"
	rows := []struct {
		name     string
		throttle bool // wire an AccountThrottler that locks out at the 3rd failure
		setup    func(t *testing.T, f *authFixture)
		steps    []loginStep
		check    func(t *testing.T, f *authFixture, last *httptest.ResponseRecorder)
	}{
		// The env admin wins before the user store is consulted. Tokens travel
		// only in httpOnly cookies, never in the JSON body (M3 fix: dual-channel
		// exposure); the success is audited under the submitted username.
		{name: "env admin", steps: []loginStep{{"admin", "password", http.StatusOK}},
			check: func(t *testing.T, f *authFixture, rr *httptest.ResponseRecorder) {
				data := authData(t, rr)
				for _, k := range []string{"access_token", "refresh_token"} {
					if _, leaked := data[k]; leaked {
						t.Errorf("%s must not be in the JSON body", k)
					}
				}
				if data["csrf_token"] == nil || data["csrf_token"] == "" || data["expires_in"] == nil || data["roles"] == nil {
					t.Errorf("data = %v, want csrf_token, expires_in and roles", data)
				}
				if authCookie(rr, "jwt") == nil {
					t.Error("jwt cookie not set")
				}
				evt := f.events(t, store.AuditActionLoginSuccess, 1)[0]
				if evt.Outcome != store.AuditOutcomeSuccess || evt.ActorLabel != "admin" || evt.TargetType != store.AuditTargetUser {
					t.Errorf("login.success = %+v, want success by admin on a user", evt)
				}
				f.events(t, store.AuditActionLoginFailure, 0)
			}},
		{name: "env admin wrong password", steps: wrongAdminLogins(1),
			check: func(t *testing.T, f *authFixture, _ *httptest.ResponseRecorder) {
				evt := f.events(t, store.AuditActionLoginFailure, 1)[0]
				if evt.Outcome != store.AuditOutcomeFailure || evt.ActorID != nil || evt.ActorLabel != "admin" {
					t.Errorf("login.failure = %+v, want failure with no actor_id, labelled admin", evt)
				}
				f.events(t, store.AuditActionLoginSuccess, 0)
			}},
		// DB users log in by email: the token's sub is the numeric user ID, and
		// last_login is refreshed so the Profile page no longer shows "—".
		{name: "db user",
			setup: func(t *testing.T, f *authFixture) { _ = f.users.ClearLastLogin(context.Background(), f.ed.ID) },
			steps: []loginStep{{authEmail("ed"), authPassword, http.StatusOK}},
			check: func(t *testing.T, f *authFixture, rr *httptest.ResponseRecorder) {
				c := authCookie(rr, "jwt")
				if c == nil {
					t.Fatal("jwt cookie not set")
				}
				_, claims, err := f.jwt.ValidateToken(c.Value, "access")
				if err != nil || claims["sub"] != strconv.FormatInt(f.ed.ID, 10) || claims["role"] != "editor" {
					t.Errorf("claims = %v (err %v), want sub %d and role editor", claims, err, f.ed.ID)
				}
				if u, _ := f.users.GetByID(context.Background(), f.ed.ID); u.LastLogin == nil || time.Since(*u.LastLogin) > 5*time.Second {
					t.Errorf("last_login = %v, want now", u.LastLogin)
				}
			}},
		// Inactive, wrong-password and unknown users get the same answer, so
		// the response never reveals which it was.
		{name: "db user inactive", steps: []loginStep{{authEmail("bob"), authPassword, http.StatusUnauthorized}},
			check: func(t *testing.T, _ *authFixture, rr *httptest.ResponseRecorder) {
				if strings.Contains(rr.Body.String(), "inactive") {
					t.Errorf("response leaks the inactive state: %s", rr.Body.String())
				}
			}},
		{name: "db user wrong password", steps: []loginStep{{authEmail("ed"), "wrong-password", http.StatusUnauthorized}}},
		{name: "unknown user", steps: []loginStep{{authEmail("ghost"), "whatever", http.StatusUnauthorized}}},
		// Guards against store/handler drift: the hash ChangeMyPassword stores
		// is the one Login verifies.
		{name: "after a password change",
			setup: func(t *testing.T, f *authFixture) {
				body := `{"current_password":"` + authPassword + `","new_password":"` + newPassword34 + `"}`
				req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/password", strings.NewReader(body))
				claims := jwt.MapClaims{"sub": strconv.FormatInt(f.ed.ID, 10), "role": "editor"}
				req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, claims))
				rr := httptest.NewRecorder()
				NewUserHandler(f.users, zap.NewNop()).ChangeMyPassword(rr, req)
				if rr.Code != http.StatusNoContent {
					t.Fatalf("ChangeMyPassword: status = %d, want 204: %s", rr.Code, rr.Body.String())
				}
			},
			steps: []loginStep{{authEmail("ed"), authPassword, http.StatusUnauthorized}, {authEmail("ed"), newPassword34, http.StatusOK}}},
		// F-4: once three failures lock the account, the next attempt is refused
		// before any credential check, with Retry-After and lockout metadata.
		{name: "lockout", throttle: true, steps: append(wrongAdminLogins(3), loginStep{"admin", "wrong", http.StatusTooManyRequests}),
			check: func(t *testing.T, f *authFixture, rr *httptest.ResponseRecorder) {
				if rr.Header().Get("Retry-After") == "" {
					t.Error("429 without Retry-After")
				}
				evts := f.audit.EventsByAction(store.AuditActionLoginFailure)
				locked := false
				for _, evt := range evts {
					var meta map[string]any
					_ = json.Unmarshal(evt.Metadata, &meta)
					locked = locked || meta["lockout"] == true
				}
				if len(evts) < 4 || !locked {
					t.Errorf("login.failure events = %d (lockout metadata %v), want >= 4 with lockout=true", len(evts), locked)
				}
			}},
		{name: "success resets the failure count", throttle: true,
			steps: append(append(wrongAdminLogins(2), loginStep{"admin", "password", http.StatusOK}), wrongAdminLogins(2)...)},
		{name: "no throttler never locks out", steps: wrongAdminLogins(10)},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newAuthFixture(t)
			if tc.throttle {
				// Soft delay from the 1st failure is capped at 1ms to keep the suite fast.
				f.h.WithAccountThrottler(middleware.NewAccountThrottler(5*time.Minute, 1, 3, 100*time.Millisecond, time.Millisecond))
			}
			if tc.setup != nil {
				tc.setup(t, f)
			}
			var rr *httptest.ResponseRecorder
			for i, s := range tc.steps {
				if rr = f.login(t, s.user, s.pass); rr.Code != s.want {
					t.Fatalf("attempt %d (%s): status = %d, want %d: %s", i+1, s.user, rr.Code, s.want, rr.Body.String())
				}
			}
			if tc.check != nil {
				tc.check(t, f, rr)
			}
		})
	}
}

// IMPORTANT: jwt.Parse stores numeric claims as float64, not *jwt.NumericDate,
// so Session must read exp through claims.GetExpirationTime(). Rows that
// hand-build claims use float64 (or go through ValidateToken) to match the
// production shape — jwt.NewNumericDate() would silently disagree with jwt.Parse.
func TestAuthHandler_Session(t *testing.T) {
	t.Parallel()
	local := func(sub, role string) func(*testing.T, *authFixture) jwt.MapClaims {
		return func(*testing.T, *authFixture) jwt.MapClaims {
			return jwt.MapClaims{"sub": sub, "role": role, "provider": "local"}
		}
	}
	rows := []struct {
		name         string
		claims       func(t *testing.T, f *authFixture) jwt.MapClaims // nil sends none
		want         int
		wantUsername string // "{ed}" is the DB user's email
		checkExpiry  func(t *testing.T, f *authFixture, expiresIn float64)
	}{
		{name: "no claims", want: http.StatusUnauthorized},
		// An env user's non-numeric sub is echoed verbatim.
		{name: "env user", claims: local("admin", "admin"), want: http.StatusOK, wantUsername: "admin"},
		// A DB user's numeric sub resolves to the human-readable email.
		{name: "db user", want: http.StatusOK, wantUsername: "{ed}",
			claims: func(_ *testing.T, f *authFixture) jwt.MapClaims {
				return jwt.MapClaims{"sub": strconv.FormatInt(f.ed.ID, 10), "role": "editor", "provider": "local"}
			}},
		// expires_in is the token's real remaining lifetime, not the configured
		// TTL, so the client does not drift its expiry forward on every call.
		{name: "expires_in from the token", want: http.StatusOK, wantUsername: "alice",
			claims: func(t *testing.T, f *authFixture) jwt.MapClaims {
				token, _, _, _, err := f.jwt.GenerateTokensForFamily("alice", "admin", "local", "")
				if err != nil {
					t.Fatal(err)
				}
				_, claims, err := f.jwt.ValidateToken(token, "access")
				if err != nil {
					t.Fatal(err)
				}
				return claims
			},
			checkExpiry: func(t *testing.T, f *authFixture, expiresIn float64) {
				if configured := f.cfg.AccessTokenExpiry.Seconds(); expiresIn <= 0 || expiresIn > configured || configured-expiresIn > 5 {
					t.Errorf("expires_in = %.0f, want within 5s below %.0f", expiresIn, configured)
				}
			}},
		{name: "expired token", want: http.StatusOK, wantUsername: "admin",
			claims: func(*testing.T, *authFixture) jwt.MapClaims {
				return jwt.MapClaims{"sub": "admin", "role": "admin", "provider": "local", "type": "access", "exp": float64(time.Now().Add(-time.Minute).Unix())}
			},
			checkExpiry: func(t *testing.T, _ *authFixture, expiresIn float64) {
				if expiresIn != 0 {
					t.Errorf("expires_in = %.0f, want 0", expiresIn)
				}
			}},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newAuthFixture(t)
			req := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
			if tc.claims != nil {
				req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, tc.claims(t, f)))
			}
			rr := httptest.NewRecorder()
			f.h.Session(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.want != http.StatusOK {
				return
			}
			data := authData(t, rr)
			wantUsername := strings.ReplaceAll(tc.wantUsername, "{ed}", f.ed.Email)
			if data["username"] != wantUsername || data["provider"] != "local" {
				t.Errorf("username, provider = %v, %v, want %s, local", data["username"], data["provider"], wantUsername)
			}
			if tc.checkExpiry != nil {
				expiresIn, _ := data["expires_in"].(float64)
				tc.checkExpiry(t, f, expiresIn)
			}
		})
	}
}

func TestAuthHandler_Refresh(t *testing.T) {
	t.Parallel()
	current := func(_ *testing.T, f *authFixture) string { return f.refresh }
	rows := []struct {
		name    string
		present func(t *testing.T, f *authFixture) string // the refresh cookie; "" sends none
		want    int
		check   func(t *testing.T, f *authFixture, rr *httptest.ResponseRecorder)
	}{
		// A refresh rotates the family (previous_jti set, grace window open)
		// and the refresh cookie, and is audited against the family.
		{name: "rotates", present: current, want: http.StatusOK,
			check: func(t *testing.T, f *authFixture, rr *httptest.ResponseRecorder) {
				for _, name := range []string{"jwt", "refresh_jwt", "csrf_token"} {
					if c := authCookie(rr, name); c == nil || c.Value == "" {
						t.Errorf("%s cookie not set", name)
					}
				}
				if c := authCookie(rr, "refresh_jwt"); c != nil && c.Value == f.refresh {
					t.Error("refresh_jwt cookie not rotated")
				}
				if fam := f.family(t); fam.PreviousJTI == nil || fam.GraceUntil == nil || !fam.GraceUntil.After(time.Now()) || fam.Status != store.RefreshTokenFamilyStatusActive {
					t.Errorf("family = %+v, want active with previous_jti and a future grace_until", fam)
				}
				if data := authData(t, rr); data["username"] != "alice" || data["provider"] != "local" || data["expires_in"] == nil {
					t.Errorf("data = %v, want username alice, provider local and expires_in", data)
				}
				if evt := f.events(t, store.AuditActionRefreshSuccess, 1)[0]; evt.ActorLabel != "alice" || evt.TargetID != f.famID {
					t.Errorf("refresh.success = %+v, want alice on family %s", evt, f.famID)
				}
			}},
		// Within the grace window the previous token still works, tolerating
		// multi-tab races.
		{name: "previous token within grace window", want: http.StatusOK,
			present: func(t *testing.T, f *authFixture) string {
				if rr := f.refreshWith(f.refresh); rr.Code != http.StatusOK {
					t.Fatalf("first refresh: status = %d", rr.Code)
				}
				return f.refresh
			}},
		// Outside it a previous token is theft: the family is marked
		// compromised and the compromise is audited.
		{name: "reuse outside grace window", want: http.StatusUnauthorized,
			present: func(t *testing.T, f *authFixture) string {
				if err := f.families.Rotate(context.Background(), f.famID, "rotated-jti", 0); err != nil {
					t.Fatal(err)
				}
				return f.refresh
			},
			check: func(t *testing.T, f *authFixture, _ *httptest.ResponseRecorder) {
				if fam := f.family(t); fam.Status != store.RefreshTokenFamilyStatusCompromised {
					t.Errorf("family status = %q, want compromised", fam.Status)
				}
				if evt := f.events(t, store.AuditActionRefreshCompromise, 1)[0]; evt.Outcome != store.AuditOutcomeFailure || evt.TargetID != f.famID {
					t.Errorf("refresh.compromise = %+v, want failure on family %s", evt, f.famID)
				}
			}},
		{name: "revoked family", want: http.StatusUnauthorized,
			present: func(t *testing.T, f *authFixture) string {
				_ = f.families.Revoke(context.Background(), f.famID)
				return f.refresh
			}},
		{name: "compromised family", want: http.StatusUnauthorized,
			present: func(t *testing.T, f *authFixture) string {
				_ = f.families.MarkCompromised(context.Background(), f.famID)
				return f.refresh
			}},
		// A DB user deactivated after login cannot mint tokens even when the
		// deactivation's best-effort family revocation never happened: the
		// refresh is refused as a dead session, without revealing the account
		// state, and the family is revoked.
		{name: "db user deactivated since login", want: http.StatusUnauthorized,
			present: func(t *testing.T, f *authFixture) string {
				c := authCookie(f.login(t, f.ed.Email, authPassword), "refresh_jwt")
				if c == nil {
					t.Fatal("login set no refresh_jwt cookie")
				}
				if err := f.users.Deactivate(context.Background(), f.ed.ID); err != nil {
					t.Fatal(err)
				}
				return c.Value
			},
			check: func(t *testing.T, f *authFixture, rr *httptest.ResponseRecorder) {
				for _, name := range []string{"jwt", "refresh_jwt", "csrf_token"} {
					if authCookie(rr, name) != nil {
						t.Errorf("%s cookie set on a refused refresh", name)
					}
				}
				if strings.Contains(rr.Body.String(), "inactive") {
					t.Errorf("response leaks the inactive state: %s", rr.Body.String())
				}
				f.events(t, store.AuditActionRefreshSuccess, 0)
				// RevokeAllForUser counts the user's families still active.
				if n, _ := f.families.RevokeAllForUser(context.Background(), strconv.FormatInt(f.ed.ID, 10)); n != 0 {
					t.Errorf("%d family still active, want the refused refresh to revoke it", n)
				}
			}},
		{name: "missing cookie", want: http.StatusUnauthorized, present: func(*testing.T, *authFixture) string { return "" }},
		{name: "invalid token", want: http.StatusUnauthorized, present: func(*testing.T, *authFixture) string { return "not.a.valid.jwt" }},
		{name: "rotation disabled", want: http.StatusUnauthorized,
			present: func(_ *testing.T, f *authFixture) string {
				f.h.familyStore = nil
				return f.refresh
			}},
		{name: "token without fam claim", want: http.StatusUnauthorized,
			present: func(t *testing.T, f *authFixture) string {
				token, _ := f.mintRefresh(t, "")
				return token
			}},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newAuthFixture(t)
			rr := f.refreshWith(tc.present(t, f))
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.check != nil {
				tc.check(t, f, rr)
			}
		})
	}
}

// Logout revokes the refresh-token family, so the refresh token is dead at
// once, and is audited under the caller's subject.
func TestAuthHandler_Logout(t *testing.T) {
	t.Parallel()
	f := newAuthFixture(t)
	req := httptest.NewRequest(http.MethodDelete, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_jwt", Value: f.refresh})
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, jwt.MapClaims{"sub": "admin", "role": "admin"}))
	rr := httptest.NewRecorder()
	f.h.Logout(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	if fam := f.family(t); fam.Status != store.RefreshTokenFamilyStatusRevoked {
		t.Errorf("family status = %q, want revoked", fam.Status)
	}
	if evt := f.events(t, store.AuditActionLogout, 1)[0]; evt.ActorLabel != "admin" {
		t.Errorf("logout actor_label = %q, want admin", evt.ActorLabel)
	}
	if rr := f.refreshWith(f.refresh); rr.Code != http.StatusUnauthorized {
		t.Errorf("refresh after logout: status = %d, want 401", rr.Code)
	}
}
