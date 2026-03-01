package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func testAuthConfig() *config.Config {
	return &config.Config{
		SecurityEnabled:    true,
		SecurityUser:       "admin",
		SecurityPass:       "password",
		JWTSecret:          "test-secret",
		AccessTokenExpiry:  15 * time.Minute,
		RefreshTokenExpiry: 30 * 24 * time.Hour,
	}
}

func TestAuthHandler_Login(t *testing.T) {
	cfg := testAuthConfig()
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist())
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
	if data["access_token"] == "" {
		t.Errorf("Expected an access token, got empty string")
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
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist())
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
