package handlers

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
	"github.com/mkutlak/alluredeck/api/internal/version"
)

func TestSystemHandler_ConfigEndpoint(t *testing.T) {
	cfg := &config.Config{
		DevMode:                  true,
		CheckResultsEverySeconds: "5",
		LLM:                      config.LLMConfig{Enabled: true, Provider: "openai", Model: "llama3.1", APIKey: "sk-secret", BaseURL: "http://ollama:11434/v1"},
	}
	code, body := serveJSON(t, NewSystemHandler(cfg, nil, nil, nil, nil).ConfigEndpoint, http.MethodGet, "/config", "")
	if code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %v", code, body)
	}
	wantJSON(t, body, map[string]any{
		"data.dev_mode": true, "data.check_results_every_seconds": "5", "data.llm_enabled": true,
		"data.app_version": version.Version, "data.app_build_date": version.BuildDate, "data.app_build_ref": version.BuildRef,
	})
	// The LLM API key must never leak into the config response.
	if s := fmt.Sprint(body); strings.Contains(s, "api_key") || strings.Contains(s, "sk-secret") {
		t.Errorf("config response leaks the LLM API key: %s", s)
	}
}

// TestSystemHandler_Ready probes each wired dependency; an unwired (nil) one is
// skipped and never fails readiness.
func TestSystemHandler_Ready(t *testing.T) {
	// A closed pool fails Ping without dialing, so no database is needed.
	closedDB, err := sql.Open("pgx", "postgres://unused")
	if err != nil {
		t.Fatal(err)
	}
	_ = closedDB.Close()
	storageDown := &testutil.MockStorage{HealthCheckFn: func(context.Context) error { return errors.New("bucket unreachable") }}
	tests := []struct {
		name     string
		db       *sql.DB
		store    storage.Store
		queue    *stubJobQueue
		want     int
		wantJSON map[string]any
	}{
		{name: "healthy without a db", store: &testutil.MockStorage{}, queue: &stubJobQueue{}, want: http.StatusOK,
			wantJSON: map[string]any{"status": "ok", "db": "skipped", "storage": "ok", "queue": "ok"}},
		{name: "db down", db: closedDB, store: &testutil.MockStorage{}, queue: &stubJobQueue{}, want: http.StatusServiceUnavailable,
			wantJSON: map[string]any{"status": "unavailable", "db": "error"}},
		{name: "storage down", store: storageDown, queue: &stubJobQueue{}, want: http.StatusServiceUnavailable,
			wantJSON: map[string]any{"storage": "error", "queue": "ok"}},
		{name: "queue down", store: &testutil.MockStorage{}, queue: &stubJobQueue{healthErr: errors.New("queue not running")}, want: http.StatusServiceUnavailable,
			wantJSON: map[string]any{"queue": "error"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, body := serveJSON(t, NewSystemHandler(&config.Config{}, tc.db, tc.store, tc.queue, nil).Ready, http.MethodGet, "/ready", "")
			if code != tc.want {
				t.Fatalf("status = %d, want %d: %v", code, tc.want, body)
			}
			wantJSON(t, body, tc.wantJSON)
		})
	}
}

// TestSystemHandler_Ready_BootstrapGate keeps /ready at 503 until the async
// startup bootstrap completes, while /health answers 200 throughout, so a slow
// bootstrap never gets the pod killed by its liveness probe.
func TestSystemHandler_Ready_BootstrapGate(t *testing.T) {
	bootstrapReady := &atomic.Bool{}
	h := NewSystemHandler(&config.Config{}, nil, nil, nil, bootstrapReady)
	for _, step := range []struct {
		done      bool
		wantReady int
		wantJSON  map[string]any
	}{
		{done: false, wantReady: http.StatusServiceUnavailable, wantJSON: map[string]any{"bootstrap": "pending", "status": "unavailable"}},
		{done: true, wantReady: http.StatusOK, wantJSON: map[string]any{"bootstrap": "ok", "status": "ok"}},
	} {
		bootstrapReady.Store(step.done)
		code, body := serveJSON(t, h.Ready, http.MethodGet, "/ready", "")
		if code != step.wantReady {
			t.Errorf("bootstrap done=%v: /ready status = %d, want %d: %v", step.done, code, step.wantReady, body)
		}
		wantJSON(t, body, step.wantJSON)
		code, body = serveJSON(t, h.Health, http.MethodGet, "/health", "")
		if code != http.StatusOK {
			t.Errorf("bootstrap done=%v: /health status = %d, want 200", step.done, code)
		}
		wantJSON(t, body, map[string]any{"status": "ok"})
	}
}
