package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/handlers"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestRegisterRoutes(t *testing.T) {
	cfg := &config.Config{SecurityEnabled: true, JWTSecret: "test-secret"}

	mocks := testutil.New()

	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())
	systemHandler := handlers.NewSystemHandler(cfg, nil)
	authHandler := handlers.NewAuthHandler(cfg, jwtManager)
	localStore := storage.NewLocalStore(cfg)
	allureCore := runner.NewAllure(cfg, localStore, mocks.Builds, mocks.Locker, nil, nil, zap.NewNop())
	allureHandler := handlers.NewAllureHandler(cfg, allureCore, nil, mocks.Projects, mocks.Builds, mocks.KnownIssues, nil, nil, localStore, zap.NewNop())

	loginLimiter := middleware.NewIPRateLimiter(5, 10, 15*time.Minute, false)

	mux := http.NewServeMux()
	adminHandler := handlers.NewAdminHandler(nil, nil, zap.NewNop())
	registerRoutes(mux, "/api/v1", cfg, jwtManager, loginLimiter, systemHandler, authHandler, allureHandler, adminHandler, nil, nil, nil, nil, nil, nil, nil)

	tests := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/version"},
		{"GET", "/api/v1/config"},
		{"POST", "/api/v1/login"},
		{"DELETE", "/api/v1/projects/testproj/reports/history"},
		{"DELETE", "/api/v1/projects/testproj/results"},
		{"POST", "/api/v1/projects/testproj/reports"},
		{"POST", "/api/v1/projects/testproj/results"},
		{"GET", "/api/v1/projects/testproj/reports"},
		{"DELETE", "/api/v1/projects/testproj/reports/42"},
	}

	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			// Login with security enabled will return 400 (Bad Request) because of missing body,
			// which is fine as it confirms the route is registered and reached the handler.
			if rr.Code == http.StatusNotFound {
				t.Errorf("Path %s %s not registered. Response: %d", tc.method, tc.path, rr.Code)
			}
		})
	}
}

func TestBareRoutes_Return404(t *testing.T) {
	cfg := &config.Config{SecurityEnabled: false, JWTSecret: "test-secret"}

	mocks := testutil.New()

	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())
	systemHandler := handlers.NewSystemHandler(cfg, nil)
	authHandler := handlers.NewAuthHandler(cfg, jwtManager)
	localStore := storage.NewLocalStore(cfg)
	allureCore := runner.NewAllure(cfg, localStore, mocks.Builds, mocks.Locker, nil, nil, zap.NewNop())
	allureHandler := handlers.NewAllureHandler(cfg, allureCore, nil, mocks.Projects, mocks.Builds, mocks.KnownIssues, nil, nil, localStore, zap.NewNop())

	loginLimiter := middleware.NewIPRateLimiter(5, 10, 15*time.Minute, false)

	mux := http.NewServeMux()
	adminHandler := handlers.NewAdminHandler(nil, nil, zap.NewNop())
	registerRoutes(mux, "/api/v1", cfg, jwtManager, loginLimiter, systemHandler, authHandler, allureHandler, adminHandler, nil, nil, nil, nil, nil, nil, nil)

	// Bare routes (no /api/v1 prefix) should return 404.
	bareRoutes := []struct {
		method string
		path   string
	}{
		{"GET", "/version"},
		{"GET", "/config"},
		{"POST", "/login"},
	}

	for _, tc := range bareRoutes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusNotFound {
				t.Errorf("Bare route %s %s should return 404, got %d", tc.method, tc.path, rr.Code)
			}
		})
	}
}
