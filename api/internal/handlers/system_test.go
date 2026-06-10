package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
)

// openTestDB opens a *sql.DB for use in handler tests that require DB connectivity.
// Skips if TEST_DATABASE_URL environment variable is not set.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping postgres-dependent test")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open test DB: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestSystemHandler_ConfigEndpoint(t *testing.T) {
	cfg := &config.Config{
		Port:                     "5050",
		DevMode:                  true,
		SecurityEnabled:          false,
		CheckResultsEverySeconds: "5",
	}

	handler := NewSystemHandler(cfg, nil, nil, nil)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "/config", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler.ConfigEndpoint(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var resp ConfigResponse
	if err = json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if resp.Data.DevMode != true {
		t.Errorf("handler returned unexpected DevMode: got %v want true", resp.Data.DevMode)
	}
	if resp.Data.SecurityEnabled != false {
		t.Errorf("handler returned unexpected SecurityEnabled: got %v want false", resp.Data.SecurityEnabled)
	}
	if resp.Data.CheckResultsEverySeconds != "5" {
		t.Errorf("handler returned unexpected CheckResultsEverySeconds: got %v want 5", resp.Data.CheckResultsEverySeconds)
	}
	if resp.Data.AppVersion == "" {
		t.Error("handler returned empty AppVersion")
	}
	if resp.Data.AppBuildDate == "" {
		t.Error("handler returned empty AppBuildDate")
	}
	if resp.Data.AppBuildRef == "" {
		t.Error("handler returned empty AppBuildRef")
	}
}

func TestSystemHandler_Health(t *testing.T) {
	handler := NewSystemHandler(&config.Config{}, nil, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handler.Health(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", resp["status"])
	}
}

func TestSystemHandler_Ready_OK(t *testing.T) {
	db := openTestDB(t)
	handler := NewSystemHandler(&config.Config{}, db, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	handler.Ready(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", resp["status"])
	}
	if resp["db"] != "ok" {
		t.Errorf("expected db=ok, got %q", resp["db"])
	}
}

func TestSystemHandler_Ready_DBDown(t *testing.T) {
	db := openTestDB(t)
	// Close the DB to simulate failure
	_ = db.Close()

	handler := NewSystemHandler(&config.Config{}, db, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	handler.Ready(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}

// stubQueue is a minimal runner.JobQueuer for readiness tests; only Healthy is
// meaningful, the rest are no-op stubs.
type stubQueue struct{ healthErr error }

var _ runner.JobQueuer = (*stubQueue)(nil)

func (s *stubQueue) Submit(context.Context, int64, string, runner.JobParams) *runner.Job { return nil }
func (s *stubQueue) SubmitPlaywright(context.Context, int64, string, string, string, string, string, string, string, string) *runner.Job {
	return nil
}
func (s *stubQueue) SubmitStagedTarGz(context.Context, int64, string, runner.StagedTarGzParams) *runner.Job {
	return nil
}
func (s *stubQueue) ListJobs(context.Context) []*runner.Job  { return nil }
func (s *stubQueue) Cancel(context.Context, string) error    { return nil }
func (s *stubQueue) Delete(context.Context, string) error    { return nil }
func (s *stubQueue) Get(context.Context, string) *runner.Job { return nil }
func (s *stubQueue) Start(context.Context)                   {}
func (s *stubQueue) Shutdown()                               {}
func (s *stubQueue) Healthy(context.Context) error           { return s.healthErr }

// readyBody runs the readiness probe and returns the decoded status map.
func readyBody(t *testing.T, h *SystemHandler) (int, map[string]string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	h.Ready(rr, req)
	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	return rr.Code, resp
}

func TestSystemHandler_Ready_DependenciesHealthy_NoDB(t *testing.T) {
	// db nil → skipped; healthy storage + queue → 200.
	handler := NewSystemHandler(&config.Config{}, nil, &storage.MockStore{}, &stubQueue{})

	code, resp := readyBody(t, handler)
	if code != http.StatusOK {
		t.Errorf("expected 200, got %d: %v", code, resp)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", resp["status"])
	}
	if resp["db"] != "skipped" {
		t.Errorf("expected db=skipped, got %q", resp["db"])
	}
	if resp["storage"] != "ok" {
		t.Errorf("expected storage=ok, got %q", resp["storage"])
	}
	if resp["queue"] != "ok" {
		t.Errorf("expected queue=ok, got %q", resp["queue"])
	}
}

func TestSystemHandler_Ready_StorageDown(t *testing.T) {
	storageDown := &storage.MockStore{
		HealthCheckFn: func(context.Context) error { return errors.New("bucket unreachable") },
	}
	handler := NewSystemHandler(&config.Config{}, nil, storageDown, &stubQueue{})

	code, resp := readyBody(t, handler)
	if code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %v", code, resp)
	}
	if resp["storage"] != "error" {
		t.Errorf("expected storage=error, got %q", resp["storage"])
	}
	if resp["queue"] != "ok" {
		t.Errorf("expected queue=ok, got %q", resp["queue"])
	}
}

func TestSystemHandler_Ready_QueueDown(t *testing.T) {
	handler := NewSystemHandler(&config.Config{}, nil, &storage.MockStore{},
		&stubQueue{healthErr: errors.New("queue not running")})

	code, resp := readyBody(t, handler)
	if code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %v", code, resp)
	}
	if resp["queue"] != "error" {
		t.Errorf("expected queue=error, got %q", resp["queue"])
	}
}
