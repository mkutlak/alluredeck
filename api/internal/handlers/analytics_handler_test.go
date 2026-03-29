package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// newBranchStoreForAnalytics returns a MockBranchStore that maps "main" -> ID 42.
func newBranchStoreForAnalytics() *testutil.MockBranchStore {
	return &testutil.MockBranchStore{
		GetByNameFn: func(_ context.Context, _, name string) (*store.Branch, error) {
			if name == "main" {
				id := int64(42)
				_ = id
				return &store.Branch{ID: 42, Name: "main"}, nil
			}
			return nil, store.ErrBranchNotFound
		},
	}
}

// TestAnalyticsHandler_GetTopErrors_NoBranch verifies the handler returns 200 without a branch param.
func TestAnalyticsHandler_GetTopErrors_NoBranch(t *testing.T) {
	var capturedBranchID *int64
	mock := &mockAnalyticsStore{
		topErrors: []store.ErrorCluster{{Message: "NPE", Count: 3}},
	}
	// Wrap to capture branchID.
	type capturingStore struct{ *mockAnalyticsStore }
	_ = capturingStore{}

	h := NewAnalyticsHandler(mock, nil, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/analytics/errors?builds=5&limit=3", nil)
	req.SetPathValue("project_id", "proj1")
	rr := httptest.NewRecorder()
	h.GetTopErrors(rr, req)

	_ = capturedBranchID
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

// TestAnalyticsHandler_GetTopErrors_WithBranch verifies that ?branch=main resolves the branch and passes branchID.
func TestAnalyticsHandler_GetTopErrors_WithBranch(t *testing.T) {
	var capturedBranchID *int64
	analyticsStore := &captureAnalyticsStore{
		onListTopErrors: func(branchID *int64) {
			capturedBranchID = branchID
		},
		topErrors: []store.ErrorCluster{{Message: "Timeout", Count: 7}},
	}
	branches := newBranchStoreForAnalytics()

	h := NewAnalyticsHandler(analyticsStore, branches, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/analytics/errors?branch=main", nil)
	req.SetPathValue("project_id", "proj1")
	rr := httptest.NewRecorder()
	h.GetTopErrors(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if capturedBranchID == nil {
		t.Fatal("expected branchID to be set, got nil")
	}
	if *capturedBranchID != 42 {
		t.Errorf("branchID = %d, want 42", *capturedBranchID)
	}
}

// TestAnalyticsHandler_GetSuitePassRates_NoBranch verifies 200 without branch param.
func TestAnalyticsHandler_GetSuitePassRates_NoBranch(t *testing.T) {
	analyticsStore := &captureAnalyticsStore{
		suitePassRates: []store.SuitePassRate{{Suite: "Smoke", Total: 10, Passed: 9, PassRate: 90}},
	}
	h := NewAnalyticsHandler(analyticsStore, nil, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj2/analytics/suites?builds=10", nil)
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

// TestAnalyticsHandler_GetSuitePassRates_WithBranch verifies branch resolution for suite pass rates.
func TestAnalyticsHandler_GetSuitePassRates_WithBranch(t *testing.T) {
	var capturedBranchID *int64
	analyticsStore := &captureAnalyticsStore{
		onListSuitePassRates: func(branchID *int64) {
			capturedBranchID = branchID
		},
		suitePassRates: []store.SuitePassRate{},
	}
	branches := newBranchStoreForAnalytics()

	h := NewAnalyticsHandler(analyticsStore, branches, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj2/analytics/suites?branch=main", nil)
	req.SetPathValue("project_id", "proj2")
	rr := httptest.NewRecorder()
	h.GetSuitePassRates(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if capturedBranchID == nil {
		t.Fatal("expected branchID to be set, got nil")
	}
	if *capturedBranchID != 42 {
		t.Errorf("branchID = %d, want 42", *capturedBranchID)
	}
}

// TestAnalyticsHandler_GetLabelBreakdown_NoBranch verifies 200 without branch param.
func TestAnalyticsHandler_GetLabelBreakdown_NoBranch(t *testing.T) {
	analyticsStore := &captureAnalyticsStore{
		labelCounts: []store.LabelCount{{Value: "critical", Count: 5}},
	}
	h := NewAnalyticsHandler(analyticsStore, nil, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj3/analytics/labels?name=severity&builds=5", nil)
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
	if len(data) != 1 {
		t.Errorf("expected 1 entry, got %d", len(data))
	}
}

// TestAnalyticsHandler_GetLabelBreakdown_WithBranch verifies branch resolution for label breakdown.
func TestAnalyticsHandler_GetLabelBreakdown_WithBranch(t *testing.T) {
	var capturedBranchID *int64
	analyticsStore := &captureAnalyticsStore{
		onListLabelBreakdown: func(branchID *int64) {
			capturedBranchID = branchID
		},
		labelCounts: []store.LabelCount{},
	}
	branches := newBranchStoreForAnalytics()

	h := NewAnalyticsHandler(analyticsStore, branches, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj3/analytics/labels?name=severity&branch=main", nil)
	req.SetPathValue("project_id", "proj3")
	rr := httptest.NewRecorder()
	h.GetLabelBreakdown(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if capturedBranchID == nil {
		t.Fatal("expected branchID to be set, got nil")
	}
	if *capturedBranchID != 42 {
		t.Errorf("branchID = %d, want 42", *capturedBranchID)
	}
}

// TestAnalyticsHandler_GetTopErrors_UnknownBranch verifies that an unknown branch name
// results in a nil branchID (no filtering) rather than an error response.
func TestAnalyticsHandler_GetTopErrors_UnknownBranch(t *testing.T) {
	var capturedBranchID *int64
	analyticsStore := &captureAnalyticsStore{
		onListTopErrors: func(branchID *int64) {
			capturedBranchID = branchID
		},
		topErrors: []store.ErrorCluster{},
	}
	branches := newBranchStoreForAnalytics() // only "main" resolves

	h := NewAnalyticsHandler(analyticsStore, branches, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/analytics/errors?branch=nonexistent", nil)
	req.SetPathValue("project_id", "proj1")
	rr := httptest.NewRecorder()
	h.GetTopErrors(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	// Unknown branch → branchID stays nil (no filtering applied).
	if capturedBranchID != nil {
		t.Errorf("expected nil branchID for unknown branch, got %d", *capturedBranchID)
	}
}

// TestAnalyticsHandler_GetTrends_NoBranch verifies the handler returns 200 without a branch param.
func TestAnalyticsHandler_GetTrends_NoBranch(t *testing.T) {
	mock := &mockAnalyticsStore{
		trendPoints: []store.TrendPoint{
			{BuildOrder: 1, Passed: 40, Failed: 5, Broken: 2, Skipped: 3, Total: 50, PassRate: 80.0, DurationMs: 60000},
		},
	}
	h := NewAnalyticsHandler(mock, nil, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj5/analytics/trends?builds=5", nil)
	req.SetPathValue("project_id", "proj5")
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
	if _, ok := data["status"].([]any); !ok {
		t.Fatalf("expected data.status array, got %T", data["status"])
	}
}

// TestAnalyticsHandler_GetTrends_WithBranch verifies that ?branch=main resolves the branch and passes branchID.
func TestAnalyticsHandler_GetTrends_WithBranch(t *testing.T) {
	var capturedBranchID *int64
	analyticsStore := &captureAnalyticsStore{
		onListTrendPoints: func(branchID *int64) {
			capturedBranchID = branchID
		},
		trendPoints: []store.TrendPoint{
			{BuildOrder: 1, Passed: 45, Failed: 3, Broken: 1, Skipped: 1, Total: 50, PassRate: 90.0, DurationMs: 55000},
		},
	}
	branches := newBranchStoreForAnalytics()

	h := NewAnalyticsHandler(analyticsStore, branches, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj5/analytics/trends?branch=main", nil)
	req.SetPathValue("project_id", "proj5")
	rr := httptest.NewRecorder()
	h.GetTrends(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if capturedBranchID == nil {
		t.Fatal("expected branchID to be set, got nil")
	}
	if *capturedBranchID != 42 {
		t.Errorf("branchID = %d, want 42", *capturedBranchID)
	}
}

// TestAnalyticsHandler_GetTrends_UnknownBranch verifies that an unknown branch name
// results in a nil branchID (no filtering) rather than an error response.
func TestAnalyticsHandler_GetTrends_UnknownBranch(t *testing.T) {
	var capturedBranchID *int64
	analyticsStore := &captureAnalyticsStore{
		onListTrendPoints: func(branchID *int64) {
			capturedBranchID = branchID
		},
		trendPoints: []store.TrendPoint{},
	}
	branches := newBranchStoreForAnalytics() // only "main" resolves

	h := NewAnalyticsHandler(analyticsStore, branches, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj5/analytics/trends?branch=nonexistent", nil)
	req.SetPathValue("project_id", "proj5")
	rr := httptest.NewRecorder()
	h.GetTrends(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	// Unknown branch → branchID stays nil (no filtering applied).
	if capturedBranchID != nil {
		t.Errorf("expected nil branchID for unknown branch, got %d", *capturedBranchID)
	}
}

// ---------------------------------------------------------------------------
// captureAnalyticsStore — captures branchID passed to each method for assertions.
// ---------------------------------------------------------------------------

type captureAnalyticsStore struct {
	topErrors            []store.ErrorCluster
	suitePassRates       []store.SuitePassRate
	labelCounts          []store.LabelCount
	trendPoints          []store.TrendPoint
	onListTopErrors      func(*int64)
	onListSuitePassRates func(*int64)
	onListLabelBreakdown func(*int64)
	onListTrendPoints    func(*int64)
}

func (c *captureAnalyticsStore) ListTopErrors(_ context.Context, _ string, _, _ int, branchID *int64) ([]store.ErrorCluster, error) {
	if c.onListTopErrors != nil {
		c.onListTopErrors(branchID)
	}
	return c.topErrors, nil
}

func (c *captureAnalyticsStore) ListSuitePassRates(_ context.Context, _ string, _ int, branchID *int64) ([]store.SuitePassRate, error) {
	if c.onListSuitePassRates != nil {
		c.onListSuitePassRates(branchID)
	}
	return c.suitePassRates, nil
}

func (c *captureAnalyticsStore) ListLabelBreakdown(_ context.Context, _, _ string, _ int, branchID *int64) ([]store.LabelCount, error) {
	if c.onListLabelBreakdown != nil {
		c.onListLabelBreakdown(branchID)
	}
	return c.labelCounts, nil
}

func (c *captureAnalyticsStore) ListTrendPoints(_ context.Context, _ string, _ int, branchID *int64) ([]store.TrendPoint, error) {
	if c.onListTrendPoints != nil {
		c.onListTrendPoints(branchID)
	}
	return c.trendPoints, nil
}

// ---------------------------------------------------------------------------
// Error leakage tests — store errors must return 500, not leak internal details.
// ---------------------------------------------------------------------------

func TestAnalyticsHandler_GetTopErrors_StoreError_Returns500(t *testing.T) {
	mock := &mockAnalyticsStore{errToReturn: fmt.Errorf("pq: connection refused")}
	h := NewAnalyticsHandler(mock, nil, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/analytics/errors", nil)
	req.SetPathValue("project_id", "proj1")
	rr := httptest.NewRecorder()
	h.GetTopErrors(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d: %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "connection refused") {
		t.Error("response body leaks internal error details")
	}
}

func TestAnalyticsHandler_GetSuitePassRates_StoreError_Returns500(t *testing.T) {
	mock := &mockAnalyticsStore{errToReturn: fmt.Errorf("pq: connection refused")}
	h := NewAnalyticsHandler(mock, nil, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/analytics/suites", nil)
	req.SetPathValue("project_id", "proj1")
	rr := httptest.NewRecorder()
	h.GetSuitePassRates(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d: %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "connection refused") {
		t.Error("response body leaks internal error details")
	}
}

func TestAnalyticsHandler_GetLabelBreakdown_StoreError_Returns500(t *testing.T) {
	mock := &mockAnalyticsStore{errToReturn: fmt.Errorf("pq: connection refused")}
	h := NewAnalyticsHandler(mock, nil, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/analytics/labels", nil)
	req.SetPathValue("project_id", "proj1")
	rr := httptest.NewRecorder()
	h.GetLabelBreakdown(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d: %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "connection refused") {
		t.Error("response body leaks internal error details")
	}
}

func TestAnalyticsHandler_GetTrends_StoreError_Returns500(t *testing.T) {
	mock := &mockAnalyticsStore{errToReturn: fmt.Errorf("pq: connection refused")}
	h := NewAnalyticsHandler(mock, nil, t.TempDir(), zap.NewNop())
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/analytics/trends", nil)
	req.SetPathValue("project_id", "proj1")
	rr := httptest.NewRecorder()
	h.GetTrends(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("want 500, got %d: %s", rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), "connection refused") {
		t.Error("response body leaks internal error details")
	}
}
