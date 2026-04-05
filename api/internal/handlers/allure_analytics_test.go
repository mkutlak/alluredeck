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
	trendPoints    []store.TrendPoint
	errToReturn    error
}

func (m *mockAnalyticsStore) ListTopErrors(_ context.Context, _ []string, _, _ int, _ *int64) ([]store.ErrorCluster, error) {
	return m.topErrors, m.errToReturn
}

func (m *mockAnalyticsStore) ListSuitePassRates(_ context.Context, _ []string, _ int, _ *int64) ([]store.SuitePassRate, error) {
	return m.suitePassRates, m.errToReturn
}

func (m *mockAnalyticsStore) ListLabelBreakdown(_ context.Context, _ []string, _ string, _ int, _ *int64) ([]store.LabelCount, error) {
	return m.labelCounts, m.errToReturn
}

func (m *mockAnalyticsStore) ListTrendPoints(_ context.Context, _ []string, _ int, _ *int64) ([]store.TrendPoint, error) {
	return m.trendPoints, m.errToReturn
}

func TestAnalyticsHandler_NilStore_GetTopErrors(t *testing.T) {
	h := NewAnalyticsHandler(nil, nil, nil, t.TempDir(), zap.NewNop())
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
	h := NewAnalyticsHandler(nil, nil, nil, t.TempDir(), zap.NewNop())
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
	h := NewAnalyticsHandler(nil, nil, nil, t.TempDir(), zap.NewNop())
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
	h := NewAnalyticsHandler(mock, nil, nil, t.TempDir(), zap.NewNop())
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

	meta, _ := resp["metadata"].(map[string]any)
	if meta == nil {
		t.Fatal("expected metadata in response")
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
	h := NewAnalyticsHandler(mock, nil, nil, t.TempDir(), zap.NewNop())
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
	h := NewAnalyticsHandler(mock, nil, nil, t.TempDir(), zap.NewNop())
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
	h := NewAnalyticsHandler(mock, nil, nil, t.TempDir(), zap.NewNop())
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

func TestAnalyticsHandler_NilStore_GetTrends(t *testing.T) {
	h := NewAnalyticsHandler(nil, nil, nil, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/myproj/analytics/trends?builds=5", nil)
	req.SetPathValue("project_id", "myproj")
	rr := httptest.NewRecorder()
	h.GetTrends(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data to be map, got %T", resp["data"])
	}
	status, ok := data["status"].([]any)
	if !ok {
		t.Fatalf("expected data.status to be array, got %T", data["status"])
	}
	if len(status) != 0 {
		t.Errorf("expected empty status for nil store, got %d entries", len(status))
	}
	passRate, ok := data["pass_rate"].([]any)
	if !ok {
		t.Fatalf("expected data.pass_rate to be array, got %T", data["pass_rate"])
	}
	if len(passRate) != 0 {
		t.Errorf("expected empty pass_rate for nil store, got %d entries", len(passRate))
	}
	duration, ok := data["duration"].([]any)
	if !ok {
		t.Fatalf("expected data.duration to be array, got %T", data["duration"])
	}
	if len(duration) != 0 {
		t.Errorf("expected empty duration for nil store, got %d entries", len(duration))
	}
	if data["kpi"] != nil {
		t.Errorf("expected null kpi for nil store, got %v", data["kpi"])
	}
}

func TestAnalyticsHandler_GetTrends_WithData(t *testing.T) {
	mock := &mockAnalyticsStore{
		trendPoints: []store.TrendPoint{
			{BuildNumber: 1, Passed: 40, Failed: 5, Broken: 2, Skipped: 3, Total: 50, PassRate: 80.0, DurationMs: 60000},
			{BuildNumber: 2, Passed: 45, Failed: 3, Broken: 1, Skipped: 1, Total: 50, PassRate: 90.0, DurationMs: 55000},
			{BuildNumber: 3, Passed: 48, Failed: 1, Broken: 0, Skipped: 1, Total: 50, PassRate: 96.0, DurationMs: 50000},
		},
	}
	h := NewAnalyticsHandler(mock, nil, nil, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/analytics/trends", nil)
	req.SetPathValue("project_id", "proj1")
	rr := httptest.NewRecorder()
	h.GetTrends(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	meta, _ := resp["metadata"].(map[string]any)
	if meta == nil {
		t.Fatal("expected metadata in response")
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data map, got %T", resp["data"])
	}
	status, ok := data["status"].([]any)
	if !ok {
		t.Fatalf("expected data.status array, got %T", data["status"])
	}
	if len(status) != 3 {
		t.Errorf("expected 3 status entries, got %d", len(status))
	}
	passRate, ok := data["pass_rate"].([]any)
	if !ok {
		t.Fatalf("expected data.pass_rate array, got %T", data["pass_rate"])
	}
	if len(passRate) != 3 {
		t.Errorf("expected 3 pass_rate entries, got %d", len(passRate))
	}
	duration, ok := data["duration"].([]any)
	if !ok {
		t.Fatalf("expected data.duration array, got %T", data["duration"])
	}
	if len(duration) != 3 {
		t.Errorf("expected 3 duration entries, got %d", len(duration))
	}
	if data["kpi"] == nil {
		t.Error("expected non-nil kpi for data with 3 trend points")
	}
}

func TestAnalyticsHandler_GetTrends_KpiComputation(t *testing.T) {
	points := make([]store.TrendPoint, 15)
	for i := range points {
		points[i] = store.TrendPoint{
			BuildNumber: i + 1,
			Passed:      40 + i,
			Failed:      5,
			Broken:      1,
			Skipped:     1,
			Total:       47 + i,
			PassRate:    float64(40+i) / float64(47+i) * 100,
			DurationMs:  int64(60000 - i*1000),
		}
	}
	mock := &mockAnalyticsStore{trendPoints: points}
	h := NewAnalyticsHandler(mock, nil, nil, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj4/analytics/trends?builds=15", nil)
	req.SetPathValue("project_id", "proj4")
	rr := httptest.NewRecorder()
	h.GetTrends(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rr.Body).Decode(&resp)

	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data map, got %T", resp["data"])
	}

	kpi, ok := data["kpi"].(map[string]any)
	if !ok {
		t.Fatalf("expected kpi map, got %T", data["kpi"])
	}

	// Sparkline arrays should contain the last 10 points.
	passRateTrend, ok := kpi["pass_rate_trend"].([]any)
	if !ok {
		t.Fatalf("expected kpi.pass_rate_trend array, got %T", kpi["pass_rate_trend"])
	}
	if len(passRateTrend) != 10 {
		t.Errorf("expected pass_rate_trend length 10, got %d", len(passRateTrend))
	}

	durationTrend, ok := kpi["duration_trend"].([]any)
	if !ok {
		t.Fatalf("expected kpi.duration_trend array, got %T", kpi["duration_trend"])
	}
	if len(durationTrend) != 10 {
		t.Errorf("expected duration_trend length 10, got %d", len(durationTrend))
	}

	// KPI latest values should match the last point (BuildNumber 15, index 14).
	last := points[14]
	latestPassRate, ok := kpi["pass_rate"].(float64)
	if !ok {
		t.Fatalf("expected kpi.pass_rate float64, got %T", kpi["pass_rate"])
	}
	if latestPassRate != last.PassRate {
		t.Errorf("kpi.pass_rate = %v, want %v", latestPassRate, last.PassRate)
	}

	latestTotal, ok := kpi["total_tests"].(float64)
	if !ok {
		t.Fatalf("expected kpi.total_tests float64, got %T", kpi["total_tests"])
	}
	if int(latestTotal) != last.Total {
		t.Errorf("kpi.total_tests = %v, want %d", latestTotal, last.Total)
	}
}
