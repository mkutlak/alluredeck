package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// mockOIDCExchanger implements security.OIDCExchanger for testing.
type mockOIDCExchanger struct {
	userInfo *security.OIDCUserInfo
}

func (m *mockOIDCExchanger) AuthCodeURL(state, _, _ string) string {
	return "https://idp.example.com/auth?state=" + state
}

func (m *mockOIDCExchanger) Exchange(context.Context, string, string, string) (*security.OIDCUserInfo, error) {
	return m.userInfo, nil
}

func testOIDCConfig() *config.Config {
	return &config.Config{
		SecurityEnabled:    true,
		JWTSecret:          "test-secret-32-bytes-minimum-len",
		AccessTokenExpiry:  config.DurationSeconds(3600 * time.Second),
		RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
		OIDC: config.OIDCConfig{
			Enabled:           true,
			IssuerURL:         "https://accounts.example.com",
			ClientID:          "test-client",
			ClientSecret:      "test-secret",
			RedirectURL:       "https://app.example.com/callback",
			Scopes:            []string{"openid", "profile", "email"},
			GroupsClaim:       "groups",
			DefaultRole:       "viewer",
			StateCookieSecret: "12345678901234567890123456789012", // 32 bytes
			PostLoginRedirect: "/",
		},
	}
}

func newTestOIDCHandler(cfg *config.Config, users store.UserAuthStorer, info *security.OIDCUserInfo, audit store.AuditLogger) *OIDCHandler {
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	return NewOIDCHandler(cfg, &mockOIDCExchanger{userInfo: info}, jwtManager, users, nil, zap.NewNop()).WithAuditLogger(audit)
}

// Login redirects to the IdP and parks state, nonce and PKCE verifier in a
// short-lived HttpOnly cookie.
func TestOIDCHandler_Login(t *testing.T) {
	t.Parallel()
	rr := httptest.NewRecorder()
	newTestOIDCHandler(testOIDCConfig(), testutil.NewMemUserStore(), nil, nil).Login(rr, httptest.NewRequest(http.MethodGet, "/auth/oidc/login", nil))
	if rr.Code != http.StatusFound || rr.Header().Get("Location") == "" {
		t.Fatalf("status = %d, Location = %q, want 302 to the IdP", rr.Code, rr.Header().Get("Location"))
	}
	if c := authCookie(rr, "oidc_state"); c == nil || !c.HttpOnly || c.MaxAge != 300 {
		t.Errorf("oidc_state cookie = %+v, want HttpOnly with MaxAge 300", c)
	}
}

// TestOIDCHandler_Callback runs sequentially: a row shortens the package-wide
// state-cookie TTL.
func TestOIDCHandler_Callback(t *testing.T) {
	const ok = "state=mystate&code=authcode"
	alice := func(sub string, verified bool) *security.OIDCUserInfo {
		return &security.OIDCUserInfo{Subject: sub, Email: "alice@example.com", EmailVerified: verified, Name: "Alice", Groups: []string{}}
	}
	// collision is a user store where alice's email already belongs to an
	// okta account; RelinkOIDC records "<id> <sub>".
	collision := func(relinked *string) store.UserAuthStorer {
		return &testutil.MockUserStore{
			UpsertByOIDCFn: func(context.Context, string, string, string, string, string) (*store.User, error) {
				return nil, store.ErrEmailAlreadyLinked
			},
			GetByEmailFn: func(context.Context, string) (*store.User, error) {
				return &store.User{ID: 42, Email: "alice@example.com", Name: "Alice", Provider: "okta", ProviderSub: "okta|orig", Role: "viewer", IsActive: true}, nil
			},
			RelinkOIDCFn: func(_ context.Context, id int64, _, sub string) error {
				*relinked = fmt.Sprintf("%d %s", id, sub)
				return nil
			},
		}
	}
	deactivated := func(*string) store.UserAuthStorer {
		users := testutil.NewMemUserStore()
		u, _ := users.UpsertByOIDC(context.Background(), "oidc", "sub123", "user@example.com", "User Name", "viewer")
		_ = users.Deactivate(context.Background(), u.ID)
		return users
	}
	rows := []struct {
		name        string
		cookieState string // state sealed in the oidc_state cookie; "" sends none
		expired     bool   // seal an already expired cookie
		query       string
		autoLink    bool
		users       func(relinked *string) store.UserAuthStorer // nil: empty in-memory store
		info        *security.OIDCUserInfo
		want        int
		auditAction string   // the one audit event of this action must carry auditMeta
		auditMeta   []string // metadata JSON fragments
		wantRelink  string   // "<id> <sub>" RelinkOIDC must receive; "" for no relink
	}{
		{name: "missing state cookie", query: "state=abc&code=xyz", want: http.StatusBadRequest},
		{name: "state mismatch", cookieState: "correct-state", query: "state=wrong-state&code=xyz", want: http.StatusBadRequest},
		{name: "IdP error", cookieState: "mystate", query: "state=mystate&error=access_denied", want: http.StatusBadRequest},
		{name: "expired state cookie", cookieState: "mystate", expired: true, query: ok, want: http.StatusBadRequest},
		{name: "deactivated user", cookieState: "mystate", query: ok, users: deactivated,
			info: &security.OIDCUserInfo{Subject: "sub123", Email: "user@example.com", Name: "User Name", Groups: []string{}}, want: http.StatusForbidden},
		{name: "new user", cookieState: "mystate", query: ok,
			info: &security.OIDCUserInfo{Subject: "sub456", Email: "newuser@example.com", Name: "New User", Groups: []string{}}, want: http.StatusFound},
		// F-5: an email owned by another identity is refused by default...
		{name: "email collision", cookieState: "mystate", query: ok, users: collision, info: alice("kc|attacker", true),
			want: http.StatusConflict, auditAction: store.AuditActionLoginFailure, auditMeta: []string{`"reason":"oidc_email_collision"`}},
		// ...relinked when auto-link is on and the IdP verified the email...
		{name: "email collision auto-linked", cookieState: "mystate", query: ok, autoLink: true, users: collision, info: alice("kc|verified", true),
			want: http.StatusFound, auditAction: store.AuditActionLoginSuccess,
			auditMeta: []string{`"oidc_link":"auto"`, `"verified":true`, `"previous_provider":"okta"`}, wantRelink: "42 kc|verified"},
		// ...and an unverified email never takes over an existing account.
		{name: "email collision unverified", cookieState: "mystate", query: ok, autoLink: true, users: collision, info: alice("kc|unverified", false),
			want: http.StatusConflict, auditAction: store.AuditActionLoginFailure, auditMeta: []string{`"reason":"oidc_email_collision_unverified"`}},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testOIDCConfig()
			cfg.OIDC.AutoLinkByEmail = tc.autoLink
			var relinked string
			var users store.UserAuthStorer = testutil.NewMemUserStore()
			if tc.users != nil {
				users = tc.users(&relinked)
			}
			req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?"+tc.query, nil)
			if tc.cookieState != "" {
				if tc.expired {
					orig := security.StateCookieTTL()
					security.SetStateCookieTTL(-time.Second)
					defer security.SetStateCookieTTL(orig)
				}
				val, err := security.EncodeStateCookie([]byte(cfg.OIDC.StateCookieSecret), tc.cookieState, "nonce", "verifier")
				if err != nil {
					t.Fatal(err)
				}
				req.AddCookie(&http.Cookie{Name: "oidc_state", Value: val})
			}
			audit := testutil.NewMockAuditLogger()
			rr := httptest.NewRecorder()
			newTestOIDCHandler(cfg, users, tc.info, audit).Callback(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if relinked != tc.wantRelink {
				t.Errorf("RelinkOIDC got %q, want %q", relinked, tc.wantRelink)
			}
			if tc.auditAction != "" {
				evts := audit.EventsByAction(tc.auditAction)
				if len(evts) != 1 {
					t.Fatalf("%s events = %d, want 1", tc.auditAction, len(evts))
				}
				for _, frag := range tc.auditMeta {
					if !strings.Contains(string(evts[0].Metadata), frag) {
						t.Errorf("audit metadata = %s, want %s", evts[0].Metadata, frag)
					}
				}
			}
			if tc.want != http.StatusFound {
				return
			}
			// A session is handed out, the state cookie cleared, and the
			// browser sent to the post-login page with ?oidc=success.
			if loc, _ := url.Parse(rr.Header().Get("Location")); loc == nil || loc.Query().Get("oidc") != "success" {
				t.Errorf("Location = %q, want ?oidc=success", rr.Header().Get("Location"))
			}
			if c := authCookie(rr, "oidc_state"); authCookie(rr, "jwt") == nil || c == nil || c.MaxAge != -1 {
				t.Errorf("cookies = %v, want jwt set and oidc_state cleared", rr.Result().Cookies())
			}
		})
	}
}
