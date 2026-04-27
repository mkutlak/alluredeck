package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
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
	authCodeURL string
	userInfo    *security.OIDCUserInfo
	exchangeErr error
}

func (m *mockOIDCExchanger) AuthCodeURL(state, nonce, codeChallenge string) string {
	if m.authCodeURL != "" {
		return m.authCodeURL + "?state=" + state
	}
	return "https://idp.example.com/auth?state=" + state
}

func (m *mockOIDCExchanger) Exchange(_ context.Context, _, _, _ string) (*security.OIDCUserInfo, error) {
	return m.userInfo, m.exchangeErr
}

func testOIDCConfig() *config.Config {
	return &config.Config{
		SecurityEnabled:    true,
		JWTSecret:          "test-secret-32-bytes-minimum-len",
		AccessTokenExpiry:  config.DurationSeconds(3600 * time.Second),
		RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
		TLS:                false,
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

func TestOIDCHandler_Login_Redirects(t *testing.T) {
	cfg := testOIDCConfig()
	mocks := testutil.New()
	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())
	userStore := testutil.NewMemUserStore()
	oidcProv := &mockOIDCExchanger{}
	handler := NewOIDCHandler(cfg, oidcProv, jwtManager, userStore, nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/login", nil)
	rr := httptest.NewRecorder()
	handler.Login(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("Login() status = %d, want %d", rr.Code, http.StatusFound)
	}
	// Check redirect to IdP
	location := rr.Header().Get("Location")
	if location == "" {
		t.Error("Login() expected Location header")
	}
	// Check state cookie set
	var stateCookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == "oidc_state" {
			stateCookie = c
			break
		}
	}
	if stateCookie == nil {
		t.Fatal("Login() expected oidc_state cookie")
		return // unreachable, but satisfies staticcheck SA5011
	}
	if !stateCookie.HttpOnly {
		t.Error("Login() oidc_state cookie must be HttpOnly")
	}
	if stateCookie.MaxAge != 300 {
		t.Errorf("Login() oidc_state MaxAge = %d, want 300", stateCookie.MaxAge)
	}
}

func TestOIDCHandler_Callback_MissingStateCookie(t *testing.T) {
	cfg := testOIDCConfig()
	mocks := testutil.New()
	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())
	handler := NewOIDCHandler(cfg, &mockOIDCExchanger{}, jwtManager, testutil.NewMemUserStore(), nil, zap.NewNop())

	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state=abc&code=xyz", nil)
	rr := httptest.NewRecorder()
	handler.Callback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Callback() without state cookie status = %d, want 400", rr.Code)
	}
}

func TestOIDCHandler_Callback_StateMismatch(t *testing.T) {
	cfg := testOIDCConfig()
	mocks := testutil.New()
	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())
	handler := NewOIDCHandler(cfg, &mockOIDCExchanger{}, jwtManager, testutil.NewMemUserStore(), nil, zap.NewNop())

	// Create a valid state cookie with state="correct-state"
	cookieVal, _ := security.EncodeStateCookie([]byte(cfg.OIDC.StateCookieSecret), "correct-state", "nonce", "verifier")
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state=wrong-state&code=xyz", nil)
	req.AddCookie(&http.Cookie{Name: "oidc_state", Value: cookieVal})
	rr := httptest.NewRecorder()
	handler.Callback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Callback() with state mismatch status = %d, want 400", rr.Code)
	}
}

func TestOIDCHandler_Callback_IdPError(t *testing.T) {
	cfg := testOIDCConfig()
	mocks := testutil.New()
	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())
	handler := NewOIDCHandler(cfg, &mockOIDCExchanger{}, jwtManager, testutil.NewMemUserStore(), nil, zap.NewNop())

	cookieVal, _ := security.EncodeStateCookie([]byte(cfg.OIDC.StateCookieSecret), "mystate", "nonce", "verifier")
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state=mystate&error=access_denied", nil)
	req.AddCookie(&http.Cookie{Name: "oidc_state", Value: cookieVal})
	rr := httptest.NewRecorder()
	handler.Callback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Callback() with IdP error status = %d, want 400", rr.Code)
	}
}

func TestOIDCHandler_Callback_ExpiredStateCookie(t *testing.T) {
	cfg := testOIDCConfig()
	mocks := testutil.New()
	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())
	handler := NewOIDCHandler(cfg, &mockOIDCExchanger{}, jwtManager, testutil.NewMemUserStore(), nil, zap.NewNop())

	// Override TTL to force expiry
	orig := security.StateCookieTTL()
	security.SetStateCookieTTL(-1 * time.Second)
	defer security.SetStateCookieTTL(orig)

	cookieVal, _ := security.EncodeStateCookie([]byte(cfg.OIDC.StateCookieSecret), "mystate", "nonce", "verifier")
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state=mystate&code=xyz", nil)
	req.AddCookie(&http.Cookie{Name: "oidc_state", Value: cookieVal})
	rr := httptest.NewRecorder()
	handler.Callback(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Callback() with expired cookie status = %d, want 400", rr.Code)
	}
}

func TestOIDCHandler_Callback_DeactivatedUser(t *testing.T) {
	cfg := testOIDCConfig()
	mocks := testutil.New()
	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())
	userStore := testutil.NewMemUserStore()

	// Pre-insert a deactivated user
	user, _ := userStore.UpsertByOIDC(context.Background(), "oidc", "sub123", "user@example.com", "User Name", "viewer")
	_ = userStore.Deactivate(context.Background(), user.ID)

	exchanger := &mockOIDCExchanger{
		userInfo: &security.OIDCUserInfo{
			Subject: "sub123",
			Email:   "user@example.com",
			Name:    "User Name",
			Groups:  []string{},
		},
	}

	handler := NewOIDCHandler(cfg, exchanger, jwtManager, userStore, nil, zap.NewNop())

	cookieVal, _ := security.EncodeStateCookie([]byte(cfg.OIDC.StateCookieSecret), "mystate", "nonce", "verifier")
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state=mystate&code=authcode", nil)
	req.AddCookie(&http.Cookie{Name: "oidc_state", Value: cookieVal})
	rr := httptest.NewRecorder()
	handler.Callback(rr, req)

	if rr.Code != http.StatusForbidden {
		t.Errorf("Callback() deactivated user status = %d, want 403", rr.Code)
	}
}

func TestOIDCHandler_Callback_Success(t *testing.T) {
	cfg := testOIDCConfig()
	mocks := testutil.New()
	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())
	userStore := testutil.NewMemUserStore()

	exchanger := &mockOIDCExchanger{
		userInfo: &security.OIDCUserInfo{
			Subject: "sub456",
			Email:   "newuser@example.com",
			Name:    "New User",
			Groups:  []string{},
		},
	}

	handler := NewOIDCHandler(cfg, exchanger, jwtManager, userStore, nil, zap.NewNop())

	cookieVal, _ := security.EncodeStateCookie([]byte(cfg.OIDC.StateCookieSecret), "mystate", "nonce", "verifier")
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state=mystate&code=authcode", nil)
	req.AddCookie(&http.Cookie{Name: "oidc_state", Value: cookieVal})
	rr := httptest.NewRecorder()
	handler.Callback(rr, req)

	if rr.Code != http.StatusFound {
		t.Errorf("Callback() success status = %d, want 302. Body: %s", rr.Code, rr.Body.String())
		return
	}

	// Check redirect includes ?oidc=success
	location := rr.Header().Get("Location")
	parsed, _ := url.Parse(location)
	if parsed.Query().Get("oidc") != "success" {
		t.Errorf("Callback() redirect %q missing ?oidc=success", location)
	}

	// Check JWT cookie set
	cookies := rr.Result().Cookies()
	var jwtCookie, stateCookie *http.Cookie
	for _, c := range cookies {
		switch c.Name {
		case "jwt":
			jwtCookie = c
		case "oidc_state":
			stateCookie = c
		}
	}
	if jwtCookie == nil {
		t.Error("Callback() success: expected 'jwt' cookie")
	}
	if stateCookie == nil || stateCookie.MaxAge != -1 {
		t.Error("Callback() success: expected oidc_state cookie cleared (MaxAge=-1)")
	}
}

// TestOIDCHandler_Callback_EmailCollisionWithoutAutoLink_Returns409 verifies
// F-5 default behaviour: when UpsertByOIDC reports ErrEmailAlreadyLinked and
// OIDC_AUTO_LINK_BY_EMAIL is unset, the handler responds 409 and emits
// auth.login.failure with metadata.reason="oidc_email_collision".
func TestOIDCHandler_Callback_EmailCollisionWithoutAutoLink_Returns409(t *testing.T) {
	cfg := testOIDCConfig()
	cfg.OIDC.AutoLinkByEmail = false
	mocks := testutil.New()
	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())

	userStore := &testutil.MockUserStore{
		UpsertByOIDCFn: func(_ context.Context, _, _, _, _, _ string) (*store.User, error) {
			return nil, store.ErrEmailAlreadyLinked
		},
	}

	exchanger := &mockOIDCExchanger{
		userInfo: &security.OIDCUserInfo{
			Subject:       "kc|attacker",
			Email:         "alice@example.com",
			EmailVerified: true,
			Name:          "Alice",
			Groups:        []string{},
		},
	}

	handler := NewOIDCHandler(cfg, exchanger, jwtManager, userStore, nil, zap.NewNop()).
		WithAuditLogger(mocks.Audit)

	cookieVal, _ := security.EncodeStateCookie([]byte(cfg.OIDC.StateCookieSecret), "mystate", "nonce", "verifier")
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state=mystate&code=authcode", nil)
	req.AddCookie(&http.Cookie{Name: "oidc_state", Value: cookieVal})
	rr := httptest.NewRecorder()
	handler.Callback(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("Callback() collision (auto-link off) status = %d, want 409. Body: %s", rr.Code, rr.Body.String())
	}

	failures := mocks.Audit.EventsByAction(store.AuditActionLoginFailure)
	if len(failures) != 1 {
		t.Fatalf("expected 1 auth.login.failure event, got %d", len(failures))
	}
	body := string(failures[0].Metadata)
	if !contains(body, `"reason":"oidc_email_collision"`) {
		t.Errorf("audit metadata = %s, want reason=oidc_email_collision", body)
	}
}

// TestOIDCHandler_Callback_EmailCollisionWithAutoLinkAndVerified_Relinks
// verifies F-5 auto-link path: when AutoLinkByEmail is true AND the IdP
// reported email_verified=true, the handler calls RelinkOIDC and proceeds to
// mint cookies + emit auth.login.success with metadata.oidc_link="auto".
func TestOIDCHandler_Callback_EmailCollisionWithAutoLinkAndVerified_Relinks(t *testing.T) {
	cfg := testOIDCConfig()
	cfg.OIDC.AutoLinkByEmail = true
	mocks := testutil.New()
	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())

	relinkedID := int64(0)
	relinkedSub := ""
	existing := &store.User{
		ID:          42,
		Email:       "alice@example.com",
		Name:        "Alice",
		Provider:    "okta",
		ProviderSub: "okta|orig",
		Role:        "viewer",
		IsActive:    true,
	}
	userStore := &testutil.MockUserStore{
		UpsertByOIDCFn: func(_ context.Context, _, _, _, _, _ string) (*store.User, error) {
			return nil, store.ErrEmailAlreadyLinked
		},
		GetByEmailFn: func(_ context.Context, _ string) (*store.User, error) {
			cp := *existing
			return &cp, nil
		},
		RelinkOIDCFn: func(_ context.Context, id int64, provider, sub string) error {
			relinkedID = id
			relinkedSub = sub
			_ = provider
			return nil
		},
	}

	exchanger := &mockOIDCExchanger{
		userInfo: &security.OIDCUserInfo{
			Subject:       "kc|verified",
			Email:         "alice@example.com",
			EmailVerified: true,
			Name:          "Alice",
			Groups:        []string{},
		},
	}

	handler := NewOIDCHandler(cfg, exchanger, jwtManager, userStore, nil, zap.NewNop()).
		WithAuditLogger(mocks.Audit)

	cookieVal, _ := security.EncodeStateCookie([]byte(cfg.OIDC.StateCookieSecret), "mystate", "nonce", "verifier")
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state=mystate&code=authcode", nil)
	req.AddCookie(&http.Cookie{Name: "oidc_state", Value: cookieVal})
	rr := httptest.NewRecorder()
	handler.Callback(rr, req)

	if rr.Code != http.StatusFound {
		t.Fatalf("Callback() auto-link verified status = %d, want 302. Body: %s", rr.Code, rr.Body.String())
	}
	if relinkedID != existing.ID {
		t.Errorf("RelinkOIDC called with id=%d, want %d", relinkedID, existing.ID)
	}
	if relinkedSub != "kc|verified" {
		t.Errorf("RelinkOIDC called with sub=%q, want kc|verified", relinkedSub)
	}

	successes := mocks.Audit.EventsByAction(store.AuditActionLoginSuccess)
	if len(successes) != 1 {
		t.Fatalf("expected 1 auth.login.success event, got %d", len(successes))
	}
	body := string(successes[0].Metadata)
	if !contains(body, `"oidc_link":"auto"`) {
		t.Errorf("audit metadata = %s, want oidc_link=auto", body)
	}
	if !contains(body, `"verified":true`) {
		t.Errorf("audit metadata = %s, want verified=true", body)
	}
	if !contains(body, `"previous_provider":"okta"`) {
		t.Errorf("audit metadata = %s, want previous_provider=okta", body)
	}
}

// TestOIDCHandler_Callback_EmailCollisionWithAutoLinkButUnverified_Returns409
// verifies the safety check: even with AutoLinkByEmail=true, an unverified
// email must not be allowed to take over an existing account.
func TestOIDCHandler_Callback_EmailCollisionWithAutoLinkButUnverified_Returns409(t *testing.T) {
	cfg := testOIDCConfig()
	cfg.OIDC.AutoLinkByEmail = true
	mocks := testutil.New()
	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())

	relinkCalled := false
	userStore := &testutil.MockUserStore{
		UpsertByOIDCFn: func(_ context.Context, _, _, _, _, _ string) (*store.User, error) {
			return nil, store.ErrEmailAlreadyLinked
		},
		RelinkOIDCFn: func(_ context.Context, _ int64, _, _ string) error {
			relinkCalled = true
			return nil
		},
	}

	exchanger := &mockOIDCExchanger{
		userInfo: &security.OIDCUserInfo{
			Subject:       "kc|unverified",
			Email:         "alice@example.com",
			EmailVerified: false, // <-- the safety check
			Name:          "Alice",
			Groups:        []string{},
		},
	}

	handler := NewOIDCHandler(cfg, exchanger, jwtManager, userStore, nil, zap.NewNop()).
		WithAuditLogger(mocks.Audit)

	cookieVal, _ := security.EncodeStateCookie([]byte(cfg.OIDC.StateCookieSecret), "mystate", "nonce", "verifier")
	req := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state=mystate&code=authcode", nil)
	req.AddCookie(&http.Cookie{Name: "oidc_state", Value: cookieVal})
	rr := httptest.NewRecorder()
	handler.Callback(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("Callback() auto-link unverified status = %d, want 409. Body: %s", rr.Code, rr.Body.String())
	}
	if relinkCalled {
		t.Error("RelinkOIDC must not be called when email_verified=false")
	}

	failures := mocks.Audit.EventsByAction(store.AuditActionLoginFailure)
	if len(failures) != 1 {
		t.Fatalf("expected 1 auth.login.failure event, got %d", len(failures))
	}
	body := string(failures[0].Metadata)
	if !contains(body, `"reason":"oidc_email_collision_unverified"`) {
		t.Errorf("audit metadata = %s, want reason=oidc_email_collision_unverified", body)
	}
}

// contains is a small substring helper kept local to oidc_test.go to avoid
// pulling in strings just for the audit-metadata assertions.
func contains(haystack, needle string) bool {
	return len(needle) == 0 || (len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
