package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/security"
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
	handler := NewAuthHandler(cfg, jwtManager)

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
	handler := NewAuthHandler(cfg, jwtManager)

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
	handler := NewAuthHandler(cfg, jwtManager)

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

func TestAuthHandler_Session_NoClaims(t *testing.T) {
	cfg := testAuthConfig()
	mocks := testutil.New()
	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())
	handler := NewAuthHandler(cfg, jwtManager)

	req := httptest.NewRequest(http.MethodGet, "/auth/session", nil)
	rr := httptest.NewRecorder()
	handler.Session(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Session() without claims status = %d, want 401", rr.Code)
	}
}
