package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// mockAnalyticsStore implements store.AnalyticsStorer for tests.
type mockAnalyticsStore struct {
	topErrors      []store.ErrorCluster
	suitePassRates []store.SuitePassRate
	labelCounts    []store.LabelCount
	errToReturn    error
}

func (m *mockAnalyticsStore) ListTopErrors(_ context.Context, _ string, _, _ int) ([]store.ErrorCluster, error) {
	return m.topErrors, m.errToReturn
}

func (m *mockAnalyticsStore) ListSuitePassRates(_ context.Context, _ string, _ int) ([]store.SuitePassRate, error) {
	return m.suitePassRates, m.errToReturn
}

func (m *mockAnalyticsStore) ListLabelBreakdown(_ context.Context, _, _ string, _ int) ([]store.LabelCount, error) {
	return m.labelCounts, m.errToReturn
}

func TestAnalyticsHandler_NilStore_GetTopErrors(t *testing.T) {
	h := NewAnalyticsHandler(nil, zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/myproj/analytics/errors?builds=5&limit=3", nil)
	req.SetPathValue("project_id", "myproj")
	rr := httptest.NewRecorder()
	h.GetTopErrors(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected data to be array, got %T", resp["data"])
	}
	if len(data) != 0 {
		t.Errorf("expected empty data for nil store, got %d entries", len(data))
	}
}

func TestAnalyticsHandler_NilStore_GetSuitePassRates(t *testing.T) {
	h := NewAnalyticsHandler(nil, zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/myproj/analytics/suites?builds=5", nil)
	req.SetPathValue("project_id", "myproj")
	rr := httptest.NewRecorder()
	h.GetSuitePassRates(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected data to be array, got %T", resp["data"])
	}
	if len(data) != 0 {
		t.Errorf("expected empty data for nil store, got %d entries", len(data))
	}
}

func TestAnalyticsHandler_NilStore_GetLabelBreakdown(t *testing.T) {
	h := NewAnalyticsHandler(nil, zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/myproj/analytics/labels?name=severity&builds=5", nil)
	req.SetPathValue("project_id", "myproj")
	rr := httptest.NewRecorder()
	h.GetLabelBreakdown(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected data to be array, got %T", resp["data"])
	}
	if len(data) != 0 {
		t.Errorf("expected empty data for nil store, got %d entries", len(data))
	}
}

func TestAnalyticsHandler_GetTopErrors_WithData(t *testing.T) {
	mock := &mockAnalyticsStore{
		topErrors: []store.ErrorCluster{
			{Message: "NPE in Foo", Count: 5},
			{Message: "Timeout in Bar", Count: 3},
		},
	}
	h := NewAnalyticsHandler(mock, zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/analytics/errors", nil)
	req.SetPathValue("project_id", "proj1")
	rr := httptest.NewRecorder()
	h.GetTopErrors(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	if resp["project_id"] != "proj1" {
		t.Errorf("project_id = %v, want proj1", resp["project_id"])
	}
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected data array, got %T", resp["data"])
	}
	if len(data) != 2 {
		t.Errorf("expected 2 entries, got %d", len(data))
	}
}

func TestAnalyticsHandler_GetSuitePassRates_WithData(t *testing.T) {
	mock := &mockAnalyticsStore{
		suitePassRates: []store.SuitePassRate{
			{Suite: "SmokeTests", Total: 10, Passed: 9, PassRate: 90.0},
		},
	}
	h := NewAnalyticsHandler(mock, zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj2/analytics/suites", nil)
	req.SetPathValue("project_id", "proj2")
	rr := httptest.NewRecorder()
	h.GetSuitePassRates(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected data array, got %T", resp["data"])
	}
	if len(data) != 1 {
		t.Errorf("expected 1 entry, got %d", len(data))
	}
}

func TestAnalyticsHandler_GetLabelBreakdown_WithData(t *testing.T) {
	mock := &mockAnalyticsStore{
		labelCounts: []store.LabelCount{
			{Value: "critical", Count: 12},
			{Value: "minor", Count: 3},
		},
	}
	h := NewAnalyticsHandler(mock, zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj3/analytics/labels?name=severity", nil)
	req.SetPathValue("project_id", "proj3")
	rr := httptest.NewRecorder()
	h.GetLabelBreakdown(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected data array, got %T", resp["data"])
	}
	if len(data) != 2 {
		t.Errorf("expected 2 entries, got %d", len(data))
	}
}

func TestAnalyticsHandler_QueryParamDefaults(t *testing.T) {
	called := false
	var gotBuilds, gotLimit int
	mock := &mockAnalyticsStore{}
	// We just verify the handler doesn't crash with no query params
	h := NewAnalyticsHandler(mock, zap.NewNop())
	_ = called
	_ = gotBuilds
	_ = gotLimit

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/p/analytics/errors", nil)
	req.SetPathValue("project_id", "p")
	rr := httptest.NewRecorder()
	h.GetTopErrors(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
}
