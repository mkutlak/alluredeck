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

	handler := NewSystemHandler(cfg)

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

	if resp.Data.DevMode != 1 {
		t.Errorf("handler returned unexpected DevMode: got %v want 1", resp.Data.DevMode)
	}
	if resp.Data.SecurityEnabled != 0 {
		t.Errorf("handler returned unexpected SecurityEnabled: got %v want 0", resp.Data.SecurityEnabled)
	}
	if resp.Data.CheckResultsEverySeconds != "5" {
		t.Errorf("handler returned unexpected CheckResultsEverySeconds: got %v want 5", resp.Data.CheckResultsEverySeconds)
	}
}
