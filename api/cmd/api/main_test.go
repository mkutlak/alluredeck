package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/handlers"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

func TestRegisterRoutes(t *testing.T) {
	cfg := &config.Config{SecurityEnabled: true, JWTSecret: "test-secret"}

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	blacklistStore := store.NewBlacklistStore(db)
	buildStore := store.NewBuildStore(db)
	projectStore := store.NewProjectStore(db)
	lockManager := store.NewLockManager()

	jwtManager := security.NewJWTManager(cfg, blacklistStore)
	systemHandler := handlers.NewSystemHandler(cfg)
	authHandler := handlers.NewAuthHandler(cfg, jwtManager)
	localStore := storage.NewLocalStore(cfg)
	allureCore := runner.NewAllure(cfg, localStore, buildStore, lockManager)
	allureHandler := handlers.NewAllureHandler(cfg, allureCore, projectStore, buildStore, localStore)

	mux := http.NewServeMux()
	registerRoutes(mux, "", cfg, jwtManager, systemHandler, authHandler, allureHandler)
	registerRoutes(mux, "/api/v1", cfg, jwtManager, systemHandler, authHandler, allureHandler)

	tests := []struct {
		method string
		path   string
	}{
		{"GET", "/version"},
		{"GET", "/config"},
		{"POST", "/login"},
		{"GET", "/api/v1/version"},
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
