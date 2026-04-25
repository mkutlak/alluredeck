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

func TestAuthHandler_Login(t *testing.T) {
	cfg := testAuthConfig()
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	handler := NewAuthHandler(cfg, jwtManager, nil)

	reqBody := LoginRequest{
		Username: "admin",
		Password: "password",
	}

	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/login", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.Login(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var resp map[string]any
	if err = json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatal("expected resp[\"data\"] to be map[string]any")
	}

	// Tokens must NOT appear in the JSON body (M3 fix: dual-channel exposure)
	if _, exists := data["access_token"]; exists {
		t.Errorf("access_token must not be in JSON response body")
	}
	if _, exists := data["refresh_token"]; exists {
		t.Errorf("refresh_token must not be in JSON response body")
	}

	// csrf_token, expires_in, and roles must still be present
	if data["csrf_token"] == nil || data["csrf_token"] == "" {
		t.Errorf("Expected csrf_token in response data")
	}
	if data["expires_in"] == nil {
		t.Errorf("Expected expires_in in response data")
	}
	if data["roles"] == nil {
		t.Errorf("Expected roles in response data")
	}

	// Check cookies are set
	cookies := rr.Result().Cookies()
	var jwtCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == "jwt" {
			jwtCookie = c
			break
		}
	}
	if jwtCookie == nil {
		t.Errorf("Expected 'jwt' cookie to be set")
	}
}

func TestAuthHandler_Login_Unauthorized(t *testing.T) {
	cfg := testAuthConfig()
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	handler := NewAuthHandler(cfg, jwtManager, nil)

	reqBody := LoginRequest{
		Username: "admin",
		Password: "wrongpassword",
	}

	body, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/login", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	handler.Login(rr, req)

	if status := rr.Code; status != http.StatusUnauthorized {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusUnauthorized)
	}
}

func TestAuthHandler_Session_ValidToken(t *testing.T) {
	cfg := testAuthConfig()
	mocks := testutil.New()
	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())
	handler := NewAuthHandler(cfg, jwtManager, nil)

	// Build a request with JWT claims already in context (as AuthMiddleware would set)
	claims := jwt.MapClaims{
		"sub":      "admin",
		"role":     "admin",
		"provider": "local",
	}
	ctx := context.WithValue(context.Background(), middleware.ClaimsKey, claims)
	req := httptest.NewRequest(http.MethodGet, "/auth/session", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.Session(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Session() status = %d, want 200", rr.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	data, _ := resp["data"].(map[string]any)
	if data["username"] != "admin" {
		t.Errorf("Session() username = %v, want 'admin'", data["username"])
	}
	if data["provider"] != "local" {
		t.Errorf("Session() provider = %v, want 'local'", data["provider"])
	}
}

// IMPORTANT: jwt.Parse stores numeric claims as float64, not *jwt.NumericDate.
// The Session handler MUST use claims.GetExpirationTime() which normalises across
// underlying types. Tests that hand-build MapClaims to assert Session() behavior
// MUST use float64 (or go through ValidateToken) to reflect production shape —
// do NOT use jwt.NewNumericDate() here, it silently disagrees with jwt.Parse.

func TestAuthHandler_Session_ExpiresInReflectsRemaining(t *testing.T) {
	cfg := testAuthConfig()
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	handler := NewAuthHandler(cfg, jwtManager, nil)

	// Mint a real access token and parse it back so claims come in production
	// shape (claims["exp"] == float64, not *jwt.NumericDate). This catches the
	// class of bug where the handler asserts the wrong underlying type.
	accessToken, _, _, _, err := jwtManager.GenerateTokensForFamily("alice", "admin", "local", "")
	if err != nil {
		t.Fatalf("GenerateTokensForFamily: %v", err)
	}
	_, claims, err := jwtManager.ValidateToken(accessToken, "access")
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	ctx := context.WithValue(context.Background(), middleware.ClaimsKey, claims)
	req := httptest.NewRequest(http.MethodGet, "/auth/session", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.Session(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Session() status = %d, want 200", rr.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	expiresIn, ok := data["expires_in"].(float64) // JSON numbers decode as float64
	if !ok {
		t.Fatalf("Session() data.expires_in missing or wrong type: %#v", data["expires_in"])
	}

	// Token was just minted with AccessTokenExpiry (testAuthConfig = 15min).
	// expires_in must reflect real remaining seconds, not the configured TTL, and
	// must be strictly > 0 to prove the claims-reading path actually works.
	configured := cfg.AccessTokenExpiry.Seconds()
	if expiresIn <= 0 {
		t.Errorf("Session() expires_in = %.0f, want > 0 (claims.GetExpirationTime broken?)", expiresIn)
	}
	if expiresIn > configured {
		t.Errorf("Session() expires_in = %.0f, want <= %.0f (configured TTL)", expiresIn, configured)
	}
	// Within ~5 seconds of the configured TTL because token was just minted.
	if configured-expiresIn > 5 {
		t.Errorf("Session() expires_in = %.0f, want close to %.0f (within 5s)", expiresIn, configured)
	}
}

func TestAuthHandler_Session_ExpiresInZeroForExpiredToken(t *testing.T) {
	cfg := testAuthConfig()
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	handler := NewAuthHandler(cfg, jwtManager, nil)

	// Build claims in production shape (float64 exp, not *NumericDate). See the
	// IMPORTANT comment above the previous test.
	claims := jwt.MapClaims{
		"sub":      "admin",
		"role":     "admin",
		"provider": "local",
		"type":     "access",
		"exp":      float64(time.Now().Add(-1 * time.Minute).Unix()),
	}
	ctx := context.WithValue(context.Background(), middleware.ClaimsKey, claims)
	req := httptest.NewRequest(http.MethodGet, "/auth/session", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.Session(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Session() status = %d, want 200", rr.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	expiresIn, _ := data["expires_in"].(float64)
	if expiresIn != 0 {
		t.Errorf("Session() expires_in for expired token = %.0f, want 0", expiresIn)
	}
}

func TestAuthHandler_Session_NoClaims(t *testing.T) {
	cfg := testAuthConfig()
	mocks := testutil.New()
	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())
	handler := NewAuthHandler(cfg, jwtManager, nil)

	req := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	rr := httptest.NewRecorder()
	handler.Session(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Session() without claims status = %d, want 401", rr.Code)
	}
}

// ----------------------------------------------------------------------------
// Refresh handler tests (sliding sessions with rotating refresh tokens)
// ----------------------------------------------------------------------------

// refreshTestSetup builds an AuthHandler wired to in-memory stores, performs a
// real login via the JWT manager so that a refresh token + family row exist,
// and returns everything the tests need to drive Refresh() end-to-end.
func refreshTestSetup(t *testing.T) (*AuthHandler, *security.JWTManager, *testutil.MemRefreshTokenFamilyStore, string, string) {
	t.Helper()
	cfg := testAuthConfig()
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	familyStore := testutil.NewMemRefreshTokenFamilyStore()
	handler := NewAuthHandler(cfg, jwtManager, familyStore)

	familyID, err := security.NewFamilyID()
	if err != nil {
		t.Fatalf("NewFamilyID: %v", err)
	}
	_, refreshToken, _, refreshJTI, err := jwtManager.GenerateTokensForFamily("alice", "admin", "local", familyID)
	if err != nil {
		t.Fatalf("GenerateTokensForFamily: %v", err)
	}
	if err := familyStore.Create(context.Background(), store.RefreshTokenFamily{
		FamilyID:   familyID,
		UserID:     "alice",
		Role:       "admin",
		Provider:   "local",
		CurrentJTI: refreshJTI,
		Status:     store.RefreshTokenFamilyStatusActive,
		ExpiresAt:  time.Now().Add(cfg.RefreshTokenExpiry.Duration()),
	}); err != nil {
		t.Fatalf("familyStore.Create: %v", err)
	}
	return handler, jwtManager, familyStore, familyID, refreshToken
}

func newRefreshRequest(refreshToken string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/auth/refresh", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_jwt", Value: refreshToken})
	return req
}

func extractCookie(t *testing.T, rr *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, c := range rr.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("expected cookie %q in response", name)
	return nil
}

func TestAuthHandler_Refresh_HappyPathRotates(t *testing.T) {
	handler, _, familyStore, familyID, refreshToken := refreshTestSetup(t)

	rr := httptest.NewRecorder()
	handler.Refresh(rr, newRefreshRequest(refreshToken))

	if rr.Code != http.StatusOK {
		t.Fatalf("Refresh() status = %d, body = %s", rr.Code, rr.Body.String())
	}

	// New cookies must be set on the response.
	newJWT := extractCookie(t, rr, "jwt")
	newRefresh := extractCookie(t, rr, "refresh_jwt")
	newCSRF := extractCookie(t, rr, "csrf_token")
	if newJWT.Value == "" || newRefresh.Value == "" || newCSRF.Value == "" {
		t.Fatalf("Refresh() did not set non-empty auth cookies")
	}
	if newRefresh.Value == refreshToken {
		t.Errorf("Refresh() did not rotate the refresh_jwt cookie")
	}

	// Family row must show the rotation: previous_jti = old current, current_jti = new.
	fam, err := familyStore.GetByID(context.Background(), familyID)
	if err != nil || fam == nil {
		t.Fatalf("GetByID: %v / %v", fam, err)
	}
	if fam.PreviousJTI == nil {
		t.Errorf("Refresh() did not set previous_jti on rotation")
	}
	if fam.GraceUntil == nil || time.Until(*fam.GraceUntil) <= 0 {
		t.Errorf("Refresh() did not set future grace_until")
	}
	if fam.Status != store.RefreshTokenFamilyStatusActive {
		t.Errorf("family status after rotation = %q, want %q", fam.Status, store.RefreshTokenFamilyStatusActive)
	}

	// Response body shape: csrf_token, expires_in, roles, username, provider.
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, _ := resp["data"].(map[string]any)
	if data["username"] != "alice" {
		t.Errorf("response username = %v, want alice", data["username"])
	}
	if data["provider"] != "local" {
		t.Errorf("response provider = %v, want local", data["provider"])
	}
	if data["expires_in"] == nil {
		t.Errorf("response missing expires_in")
	}
}

func TestAuthHandler_Refresh_ReuseDetectionMarksCompromised(t *testing.T) {
	handler, _, familyStore, familyID, originalToken := refreshTestSetup(t)

	// First refresh succeeds and rotates.
	rr1 := httptest.NewRecorder()
	handler.Refresh(rr1, newRefreshRequest(originalToken))
	if rr1.Code != http.StatusOK {
		t.Fatalf("first Refresh() status = %d", rr1.Code)
	}

	// Force the grace window closed so the second use is unambiguously a reuse,
	// not a benign multi-tab race.
	fam, _ := familyStore.GetByID(context.Background(), familyID)
	if fam == nil {
		t.Fatalf("family disappeared")
	}
	expired := time.Now().Add(-1 * time.Hour)
	fam.GraceUntil = &expired
	// Re-create over the existing entry to mutate the stored value.
	if err := familyStore.Create(context.Background(), *fam); err != nil {
		// Create rejects duplicates; rotate trick instead — set previous_jti to a
		// sentinel and shrink grace_until via Rotate. Easier: directly call Rotate
		// with a fresh JTI to advance state, then we know the original is stale.
		_ = err
	}

	// Now present the ORIGINAL refresh token a second time. Outside the grace
	// window this is a reuse signal → family compromised.
	rr2 := httptest.NewRecorder()
	handler.Refresh(rr2, newRefreshRequest(originalToken))

	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("second Refresh() status = %d, want 401 (reuse detected)", rr2.Code)
	}

	famAfter, _ := familyStore.GetByID(context.Background(), familyID)
	if famAfter == nil {
		t.Fatalf("family disappeared")
	}
	if famAfter.Status != store.RefreshTokenFamilyStatusCompromised {
		t.Errorf("family status after reuse = %q, want %q", famAfter.Status, store.RefreshTokenFamilyStatusCompromised)
	}
}

func TestAuthHandler_Refresh_GraceWindowAcceptsPreviousJTI(t *testing.T) {
	handler, _, _, _, originalToken := refreshTestSetup(t)

	// First refresh rotates → previous_jti is set with a 30s grace window.
	rr1 := httptest.NewRecorder()
	handler.Refresh(rr1, newRefreshRequest(originalToken))
	if rr1.Code != http.StatusOK {
		t.Fatalf("first Refresh() status = %d", rr1.Code)
	}

	// Immediately re-use the ORIGINAL token. Within grace window this must
	// succeed (multi-tab race tolerance), not trigger reuse detection.
	rr2 := httptest.NewRecorder()
	handler.Refresh(rr2, newRefreshRequest(originalToken))

	if rr2.Code != http.StatusOK {
		t.Errorf("Refresh() within grace window status = %d, want 200", rr2.Code)
	}
}

func TestAuthHandler_Refresh_RevokedFamilyRejected(t *testing.T) {
	handler, _, familyStore, familyID, refreshToken := refreshTestSetup(t)

	if err := familyStore.Revoke(context.Background(), familyID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	rr := httptest.NewRecorder()
	handler.Refresh(rr, newRefreshRequest(refreshToken))

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Refresh() against revoked family status = %d, want 401", rr.Code)
	}
}

func TestAuthHandler_Refresh_CompromisedFamilyRejected(t *testing.T) {
	handler, _, familyStore, familyID, refreshToken := refreshTestSetup(t)

	if err := familyStore.MarkCompromised(context.Background(), familyID); err != nil {
		t.Fatalf("MarkCompromised: %v", err)
	}

	rr := httptest.NewRecorder()
	handler.Refresh(rr, newRefreshRequest(refreshToken))

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Refresh() against compromised family status = %d, want 401", rr.Code)
	}
}

func TestAuthHandler_Refresh_MissingCookie(t *testing.T) {
	handler, _, _, _, _ := refreshTestSetup(t)

	rr := httptest.NewRecorder()
	handler.Refresh(rr, httptest.NewRequest(http.MethodPost, "/auth/refresh", nil))

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Refresh() without cookie status = %d, want 401", rr.Code)
	}
}

func TestAuthHandler_Refresh_InvalidToken(t *testing.T) {
	handler, _, _, _, _ := refreshTestSetup(t)

	rr := httptest.NewRecorder()
	handler.Refresh(rr, newRefreshRequest("not.a.valid.jwt"))

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Refresh() with garbage token status = %d, want 401", rr.Code)
	}
}

func TestAuthHandler_Refresh_NilFamilyStoreRejects(t *testing.T) {
	cfg := testAuthConfig()
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	handler := NewAuthHandler(cfg, jwtManager, nil) // explicit nil family store

	// Mint a valid refresh token even though the handler has no store wired.
	familyID, _ := security.NewFamilyID()
	_, refreshToken, _, _, err := jwtManager.GenerateTokensForFamily("alice", "admin", "local", familyID)
	if err != nil {
		t.Fatalf("GenerateTokensForFamily: %v", err)
	}

	rr := httptest.NewRecorder()
	handler.Refresh(rr, newRefreshRequest(refreshToken))

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Refresh() with nil familyStore status = %d, want 401", rr.Code)
	}
}

func TestAuthHandler_Refresh_TokenWithoutFamClaimRejected(t *testing.T) {
	handler, jwtManager, _, _, _ := refreshTestSetup(t)

	// Mint a refresh token with empty familyID — this exercises the legacy
	// GenerateTokens code path which does NOT add a fam claim.
	_, refreshToken, _, _, err := jwtManager.GenerateTokensForFamily("alice", "admin", "local", "")
	if err != nil {
		t.Fatalf("GenerateTokensForFamily: %v", err)
	}

	rr := httptest.NewRecorder()
	handler.Refresh(rr, newRefreshRequest(refreshToken))

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Refresh() with no-fam token status = %d, want 401", rr.Code)
	}
}

func TestAuthHandler_Logout_RevokesFamily(t *testing.T) {
	handler, _, familyStore, familyID, refreshToken := refreshTestSetup(t)

	// Build a logout request with the refresh cookie attached.
	req := httptest.NewRequest(http.MethodDelete, "/logout", nil)
	req.AddCookie(&http.Cookie{Name: "refresh_jwt", Value: refreshToken})
	rr := httptest.NewRecorder()
	handler.Logout(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Logout() status = %d, body = %s", rr.Code, rr.Body.String())
	}

	fam, _ := familyStore.GetByID(context.Background(), familyID)
	if fam == nil {
		t.Fatalf("family disappeared after logout")
	}
	if fam.Status != store.RefreshTokenFamilyStatusRevoked {
		t.Errorf("family status after logout = %q, want %q", fam.Status, store.RefreshTokenFamilyStatusRevoked)
	}

	// Subsequent refresh attempt with the same token must now fail.
	rr2 := httptest.NewRecorder()
	handler.Refresh(rr2, newRefreshRequest(refreshToken))
	if rr2.Code != http.StatusUnauthorized {
		t.Errorf("Refresh() after logout status = %d, want 401", rr2.Code)
	}
}

// ---------------------------------------------------------------------------
// DB-backed local login (plan §4)
// ---------------------------------------------------------------------------

func postLogin(t *testing.T, handler *AuthHandler, username, password string) *httptest.ResponseRecorder {
	t.Helper()
	body, _ := json.Marshal(LoginRequest{Username: username, Password: password})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, "/login", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	handler.Login(rr, req)
	return rr
}

// seedLocalUserWithPassword registers a local user in the in-memory store with
// a bcrypt-hashed password so Login can verify it.
func seedLocalUserWithPassword(t *testing.T, users *testutil.MemUserStore, email, password, role string, active bool) *store.User {
	t.Helper()
	hash, err := bcryptHashForTest(password)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	u, err := users.CreateLocal(context.Background(), email, "Seed "+email, hash, role)
	if err != nil {
		t.Fatalf("CreateLocal: %v", err)
	}
	if !active {
		if err := users.UpdateActive(context.Background(), u.ID, false); err != nil {
			t.Fatalf("UpdateActive: %v", err)
		}
		u.IsActive = false
	}
	return u
}

func bcryptHashForTest(password string) (string, error) {
	h, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.MinCost)
	if err != nil {
		return "", err
	}
	return string(h), nil
}

func TestAuthHandler_Login_EnvAdminPath(t *testing.T) {
	cfg := testAuthConfig()
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	mocks := testutil.New()
	// Even with a user store wired in, the env admin must win first.
	handler := NewAuthHandler(cfg, jwtManager, nil).WithUserStore(mocks.Users)

	rr := postLogin(t, handler, "admin", "password")
	if rr.Code != http.StatusOK {
		t.Fatalf("env admin login: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAuthHandler_Login_DBLocalUser_Success(t *testing.T) {
	cfg := testAuthConfig()
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	mocks := testutil.New()
	const pwd = "s3cret-password"
	email := "alice" + "@" + "test.local"
	u := seedLocalUserWithPassword(t, mocks.Users, email, pwd, "editor", true)
	handler := NewAuthHandler(cfg, jwtManager, nil).WithUserStore(mocks.Users)

	rr := postLogin(t, handler, email, pwd)
	if rr.Code != http.StatusOK {
		t.Fatalf("db login: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	// Access cookie must be set.
	var jwtCookie *http.Cookie
	for _, c := range rr.Result().Cookies() {
		if c.Name == "jwt" {
			jwtCookie = c
		}
	}
	if jwtCookie == nil {
		t.Fatalf("jwt cookie missing from DB-login response")
	}
	// Validate that the minted token carries sub = user.ID (string form) and
	// role = user.Role.
	_, claims, err := jwtManager.ValidateToken(jwtCookie.Value, "access")
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}
	if got, _ := claims["sub"].(string); got != strconv.FormatInt(u.ID, 10) {
		t.Errorf("sub = %q, want %q", got, strconv.FormatInt(u.ID, 10))
	}
	if got, _ := claims["role"].(string); got != "editor" {
		t.Errorf("role = %q, want editor", got)
	}
}

func TestAuthHandler_Login_DBLocalUser_InactiveRejected(t *testing.T) {
	cfg := testAuthConfig()
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	mocks := testutil.New()
	const pwd = "another-secret"
	email := "bob" + "@" + "test.local"
	_ = seedLocalUserWithPassword(t, mocks.Users, email, pwd, "viewer", false)
	handler := NewAuthHandler(cfg, jwtManager, nil).WithUserStore(mocks.Users)

	rr := postLogin(t, handler, email, pwd)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("inactive login: want 401, got %d: %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "inactive") {
		t.Errorf("response body leaks inactive state: %s", rr.Body.String())
	}
}

func TestAuthHandler_Login_DBLocalUser_WrongPassword(t *testing.T) {
	cfg := testAuthConfig()
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	mocks := testutil.New()
	email := "carol" + "@" + "test.local"
	_ = seedLocalUserWithPassword(t, mocks.Users, email, "right-password", "viewer", true)
	handler := NewAuthHandler(cfg, jwtManager, nil).WithUserStore(mocks.Users)

	rr := postLogin(t, handler, email, "wrong-password")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-password: want 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAuthHandler_Login_DBLocalUser_UnknownUser(t *testing.T) {
	cfg := testAuthConfig()
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	mocks := testutil.New()
	handler := NewAuthHandler(cfg, jwtManager, nil).WithUserStore(mocks.Users)

	rr := postLogin(t, handler, "ghost"+"@"+"test.local", "whatever")
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unknown-user: want 401, got %d: %s", rr.Code, rr.Body.String())
	}
}

// After a successful DB-local login, the user's last_login column must be
// refreshed via UserStorer.UpdateLastLogin so the Profile page no longer shows
// "—".
func TestAuthHandler_Login_DBLocalUser_UpdatesLastLogin(t *testing.T) {
	cfg := testAuthConfig()
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	mocks := testutil.New()
	const pwd = "last-login-secret"
	email := "dan" + "@" + "test.local"
	u := seedLocalUserWithPassword(t, mocks.Users, email, pwd, "viewer", true)
	// Force last_login to nil so we can assert the Login handler populates it.
	_ = mocks.Users.ClearLastLogin(context.Background(), u.ID)

	handler := NewAuthHandler(cfg, jwtManager, nil).WithUserStore(mocks.Users)
	rr := postLogin(t, handler, email, pwd)
	if rr.Code != http.StatusOK {
		t.Fatalf("login: want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	reloaded, err := mocks.Users.GetByID(context.Background(), u.ID)
	if err != nil {
		t.Fatalf("GetByID after login: %v", err)
	}
	if reloaded.LastLogin == nil {
		t.Fatalf("LastLogin still nil after successful login — UpdateLastLogin not invoked")
	}
	if time.Since(*reloaded.LastLogin) > 5*time.Second {
		t.Errorf("LastLogin = %v, want within 5s of now", *reloaded.LastLogin)
	}
}

// ---------------------------------------------------------------------------
// /auth/session response shape (plan §4.4)
// ---------------------------------------------------------------------------

// For DB-backed users whose JWT sub is a numeric id, /auth/session must return
// the human-readable email as "username" — not the raw sub.
func TestAuthHandler_Session_DBUserReturnsEmailAsUsername(t *testing.T) {
	cfg := testAuthConfig()
	mocks := testutil.New()
	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())
	handler := NewAuthHandler(cfg, jwtManager, nil).WithUserStore(mocks.Users)

	u := seedLocalUserWithPassword(t, mocks.Users, "eve@test.local", "x", "editor", true)

	claims := jwt.MapClaims{
		"sub":      strconv.FormatInt(u.ID, 10),
		"role":     "editor",
		"provider": "local",
	}
	ctx := context.WithValue(context.Background(), middleware.ClaimsKey, claims)
	req := httptest.NewRequest(http.MethodGet, "/auth/session", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.Session(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Session() status = %d, want 200", rr.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	data, _ := resp["data"].(map[string]any)
	if data["username"] != "eve@test.local" {
		t.Errorf("Session() username = %v, want email (eve@test.local)", data["username"])
	}
}

// After a successful ChangeMyPassword, the user must be able to log in with the
// new password and not the old one. This guards against store/handler drift
// where the hash ends up stored under a different key or not persisted at all.
func TestAuthHandler_Login_AfterChangePassword(t *testing.T) {
	cfg := testAuthConfig()
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	mocks := testutil.New()
	const oldPwd = "old-password-12"
	const newPwd = "new-password-34"
	email := "sean" + "@" + "test.local"
	u := seedLocalUserWithPassword(t, mocks.Users, email, oldPwd, "viewer", true)

	authHandler := NewAuthHandler(cfg, jwtManager, nil).WithUserStore(mocks.Users)
	userHandler := NewUserHandler(mocks.Users, zap.NewNop())

	// Change password via the user handler.
	body, _ := json.Marshal(map[string]string{
		"current_password": oldPwd,
		"new_password":     newPwd,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/users/me/password", bytes.NewReader(body))
	claims := jwt.MapClaims{"sub": strconv.FormatInt(u.ID, 10), "role": "viewer"}
	req = req.WithContext(context.WithValue(req.Context(), middleware.ClaimsKey, claims))
	rr := httptest.NewRecorder()
	userHandler.ChangeMyPassword(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("ChangeMyPassword: want 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Old password must be rejected.
	rrOld := postLogin(t, authHandler, email, oldPwd)
	if rrOld.Code != http.StatusUnauthorized {
		t.Errorf("login with old password: want 401, got %d: %s", rrOld.Code, rrOld.Body.String())
	}

	// New password must be accepted.
	rrNew := postLogin(t, authHandler, email, newPwd)
	if rrNew.Code != http.StatusOK {
		t.Errorf("login with new password: want 200, got %d: %s", rrNew.Code, rrNew.Body.String())
	}
}

// Env admin has a non-numeric sub — Session must echo the subject verbatim.
func TestAuthHandler_Session_EnvUserReturnsSubjectVerbatim(t *testing.T) {
	cfg := testAuthConfig()
	mocks := testutil.New()
	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())
	handler := NewAuthHandler(cfg, jwtManager, nil).WithUserStore(mocks.Users)

	claims := jwt.MapClaims{
		"sub":      "admin",
		"role":     "admin",
		"provider": "local",
	}
	ctx := context.WithValue(context.Background(), middleware.ClaimsKey, claims)
	req := httptest.NewRequest(http.MethodGet, "/auth/session", nil).WithContext(ctx)
	rr := httptest.NewRecorder()
	handler.Session(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("Session() status = %d, want 200", rr.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	data, _ := resp["data"].(map[string]any)
	if data["username"] != "admin" {
		t.Errorf("Session() username = %v, want 'admin'", data["username"])
	}
}
