package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

func TestSystemHandler_ConfigEndpoint(t *testing.T) {
	cfg := &config.Config{
		Port:             "5050",
		DevMode:          true,
		SecurityEnabled:  false,
		CheckResultsSecs: "5",
	}

	handler := NewSystemHandler(cfg, nil)

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
	handler := NewSystemHandler(&config.Config{}, nil)

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
	handler := NewSystemHandler(&config.Config{}, db)

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

	handler := NewSystemHandler(&config.Config{}, db)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	handler.Ready(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", rr.Code, rr.Body.String())
	}
}
