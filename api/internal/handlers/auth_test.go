package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/security"
)

// memBlacklist is an in-memory security.BlacklistStore for tests.
type memBlacklist struct {
	mu      sync.RWMutex
	entries map[string]time.Time
}

func newMemBlacklist() *memBlacklist {
	return &memBlacklist{entries: make(map[string]time.Time)}
}

func (m *memBlacklist) AddToBlacklist(_ context.Context, jti string, expiresAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries[jti] = expiresAt
	return nil
}

func (m *memBlacklist) IsBlacklisted(_ context.Context, jti string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	exp, ok := m.entries[jti]
	if !ok {
		return false, nil
	}
	return time.Now().Before(exp), nil
}

func (m *memBlacklist) PruneExpired(_ context.Context) (int64, error) { return 0, nil }

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
	jwtManager := security.NewJWTManager(cfg, newMemBlacklist())
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
	jwtManager := security.NewJWTManager(cfg, newMemBlacklist())
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
