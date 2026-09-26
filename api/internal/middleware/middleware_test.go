package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func testAuthConfig() *config.Config {
	return &config.Config{
		SecurityEnabled:    true,
		JWTSecret:          "test-secret",
		AccessTokenExpiry:  config.DurationSeconds(15 * time.Minute),
		RefreshTokenExpiry: config.DurationSeconds(30 * 24 * time.Hour),
	}
}

// okHandler answers 200 and is the protected endpoint in these tests.
func okHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

// TestAuthMiddleware sends one request per row through AuthMiddleware: JWTs
// from the Authorization header or the jwt cookie, ald_ API keys looked up by
// their hash, and the F-3 is_active recheck, which dispatches on the API key's
// username shape (env literal, numeric user ID, email), rejects a deactivated
// user's JWT or API key with 401 "Account inactive" (fix 9ccee8e relies on it),
// and fails open when the user row is missing (fix 927ef9e). Rejections never
// leak token details (REVIEW #7).
func TestAuthMiddleware(t *testing.T) {
	cfg := testAuthConfig()
	jwtMgr := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	valid, _, err := jwtMgr.GenerateTokens("testuser", "admin")
	if err != nil {
		t.Fatal(err)
	}
	expiredCfg := *cfg
	expiredCfg.AccessTokenExpiry = config.DurationSeconds(-time.Minute)
	expired, _, err := security.NewJWTManager(&expiredCfg, testutil.NewMemBlacklist(), zap.NewNop()).GenerateTokens("testuser", "admin")
	if err != nil {
		t.Fatal(err)
	}

	// Users 1 (admin@example.com) and 2 (x@y.z) are active; user 3
	// (gone@example.com) is deactivated.
	users := testutil.NewMemUserStore()
	for _, email := range []string{"admin@example.com", "x@y.z", "gone@example.com"} {
		if _, err := users.UpsertByOIDC(context.Background(), "local", "sub-"+email, email, email, "viewer"); err != nil {
			t.Fatal(err)
		}
	}
	if err := users.Deactivate(context.Background(), 3); err != nil {
		t.Fatal(err)
	}
	cache := NewUserActiveCache(users, time.Second, 10)
	inactive, _, err := jwtMgr.GenerateTokens("3", "viewer")
	if err != nil {
		t.Fatal(err)
	}

	const apiKey = "ald_a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4e5f6a1b2"
	future, past := time.Now().Add(time.Hour), time.Now().Add(-time.Hour)
	keyOf := func(username string, expires *time.Time) *store.APIKey {
		return &store.APIKey{ID: 1, Username: username, Role: "viewer", ExpiresAt: expires}
	}

	tests := []struct {
		name     string
		cfg      *config.Config // nil = security enabled
		header   string         // Authorization header
		cookie   string         // jwt cookie
		key      *store.APIKey  // stored under apiKey's hash; nil = none
		cache    *UserActiveCache
		wantCode int
		wantMsg  string // metadata.message of a rejection
		wantSub  string // sub claim reaching the handler
	}{
		{name: "security disabled", cfg: &config.Config{}, wantCode: http.StatusOK},
		{name: "missing token", wantCode: http.StatusUnauthorized, wantMsg: "Missing authorization token"},
		{name: "JWT in the header", header: "Bearer " + valid, wantCode: http.StatusOK, wantSub: "testuser"},
		{name: "JWT in the jwt cookie", cookie: valid, wantCode: http.StatusOK, wantSub: "testuser"},
		{name: "invalid JWT", header: "Bearer invalid-token", wantCode: http.StatusUnauthorized, wantMsg: "Invalid token"},
		{name: "expired JWT leaks no expiry details", header: "Bearer " + expired, wantCode: http.StatusUnauthorized, wantMsg: "Invalid token"},
		{name: "API key", header: "Bearer " + apiKey, key: keyOf("apiuser", &future), wantCode: http.StatusOK, wantSub: "apiuser"},
		{name: "expired API key", header: "Bearer " + apiKey, key: keyOf("apiuser", &past), wantCode: http.StatusUnauthorized, wantMsg: "API key has expired"},
		{name: "unknown API key", header: "Bearer " + apiKey, wantCode: http.StatusUnauthorized, wantMsg: "Invalid API key"},
		{name: "API key of a legacy env username", header: "Bearer " + apiKey, key: keyOf("admin", &future), cache: cache, wantCode: http.StatusOK, wantSub: "admin"},
		{name: "API key of an active numeric username", header: "Bearer " + apiKey, key: keyOf("1", &future), cache: cache, wantCode: http.StatusOK, wantSub: "1"},
		{name: "API key of an active email username", header: "Bearer " + apiKey, key: keyOf("x@y.z", &future), cache: cache, wantCode: http.StatusOK, wantSub: "x@y.z"},
		{name: "API key of an unknown email fails open", header: "Bearer " + apiKey, key: keyOf("nope@nowhere", &future), cache: cache, wantCode: http.StatusOK, wantSub: "nope@nowhere"},
		{name: "JWT of an inactive user", header: "Bearer " + inactive, cache: cache, wantCode: http.StatusUnauthorized, wantMsg: "Account inactive"},
		{name: "API key of an inactive user", header: "Bearer " + apiKey, key: keyOf("3", &future), cache: cache, wantCode: http.StatusUnauthorized, wantMsg: "Account inactive"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			keys := &testutil.MockAPIKeyStore{
				GetByHashFn: func(_ context.Context, hash string) (*store.APIKey, error) {
					if tc.key == nil || hash != security.HashAPIKey(apiKey) {
						return nil, store.ErrAPIKeyNotFound
					}
					return tc.key, nil
				},
			}
			c := cfg
			if tc.cfg != nil {
				c = tc.cfg
			}
			var gotSub string
			h := AuthMiddleware(c, jwtMgr, false, keys, tc.cache)(func(w http.ResponseWriter, r *http.Request) {
				claims, _ := ClaimsFromContext(r.Context())
				gotSub, _ = claims["sub"].(string)
				w.WriteHeader(http.StatusOK)
			})

			req := httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: "jwt", Value: tc.cookie})
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)

			if rr.Code != tc.wantCode || gotSub != tc.wantSub {
				t.Fatalf("got %d sub %q, want %d sub %q", rr.Code, gotSub, tc.wantCode, tc.wantSub)
			}
			if tc.wantMsg != "" {
				var resp struct {
					Metadata struct{ Message string } `json:"metadata"`
				}
				if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil || resp.Metadata.Message != tc.wantMsg {
					t.Errorf("body %s: want exact message %q", rr.Body, tc.wantMsg)
				}
			}
		})
	}
}

// TestRequireRole checks each role against each required level through the
// real auth chain; admin > editor > viewer.
func TestRequireRole(t *testing.T) {
	t.Parallel()
	cfg := testAuthConfig()
	jwtMgr := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	for _, tc := range []struct {
		role, required string
		want           int
	}{
		{"viewer", "viewer", http.StatusOK}, {"viewer", "editor", http.StatusForbidden}, {"viewer", "admin", http.StatusForbidden},
		{"editor", "viewer", http.StatusOK}, {"editor", "editor", http.StatusOK}, {"editor", "admin", http.StatusForbidden},
		{"admin", "viewer", http.StatusOK}, {"admin", "editor", http.StatusOK}, {"admin", "admin", http.StatusOK},
	} {
		token, _, err := jwtMgr.GenerateTokens(tc.role+"-user", tc.role)
		if err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		AuthMiddleware(cfg, jwtMgr, false, nil, nil)(RequireRole(tc.required)(okHandler)).ServeHTTP(rr, req)
		if rr.Code != tc.want {
			t.Errorf("%s on a %s endpoint: got %d, want %d", tc.role, tc.required, rr.Code, tc.want)
		}
	}

	// Without AuthMiddleware in front there are no claims to check.
	rr := httptest.NewRecorder()
	RequireRole("admin")(okHandler).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/", nil))
	if rr.Code != http.StatusForbidden {
		t.Errorf("missing claims: got %d, want 403", rr.Code)
	}
}

// TestCORSMiddleware: a wildcard allows any origin without credentials, an
// allowlist echoes only listed origins with credentials, preflights get the
// allowed methods, and Vary: Origin is always set so caches key by Origin.
func TestCORSMiddleware(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		origins     []string
		method      string
		origin      string
		wantOrigin  string
		wantCreds   string
		wantMethods bool
	}{
		{"wildcard", []string{"*"}, http.MethodGet, "http://example.com", "*", "", false},
		{"allowlisted origin", []string{"http://example.com"}, http.MethodGet, "http://example.com", "http://example.com", "true", false},
		{"origin not in the allowlist", []string{"http://example.com"}, http.MethodGet, "http://malicious.com", "", "", false},
		{"preflight", []string{"*"}, http.MethodOptions, "http://example.com", "*", "", true},
	}
	for _, tc := range tests {
		h := CORSMiddleware(&config.Config{CORSAllowedOrigins: tc.origins}, http.HandlerFunc(okHandler))
		req := httptest.NewRequest(tc.method, "/", nil)
		req.Header.Set("Origin", tc.origin)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		hdr := rr.Header()
		if rr.Code != http.StatusOK || hdr.Get("Access-Control-Allow-Origin") != tc.wantOrigin ||
			hdr.Get("Access-Control-Allow-Credentials") != tc.wantCreds ||
			(hdr.Get("Access-Control-Allow-Methods") != "") != tc.wantMethods || hdr.Get("Vary") != "Origin" {
			t.Errorf("%s: got %d %v", tc.name, rr.Code, hdr)
		}
	}
}
