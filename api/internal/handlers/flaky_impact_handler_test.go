package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestGetFlakyImpact_HappyPath verifies the handler resolves the project,
// calls ListFlakyImpact, and returns the {data, metadata} envelope.
func TestGetFlakyImpact_HappyPath(t *testing.T) {
	mock := &testutil.MockAnalyticsStore{
		ListFlakyImpactFn: func(_ context.Context, projectID int64, branchID *int64, builds, limit int) ([]store.FlakyImpact, error) {
			if projectID != 1 {
				t.Errorf("expected projectID 1, got %d", projectID)
			}
			if branchID != nil {
				t.Errorf("expected nil branchID, got %v", *branchID)
			}
			if builds != analyticsDefaultBuilds {
				t.Errorf("expected default builds %d, got %d", analyticsDefaultBuilds, builds)
			}
			if limit != analyticsDefaultLimit {
				t.Errorf("expected default limit %d, got %d", analyticsDefaultLimit, limit)
			}
			return []store.FlakyImpact{
				{FullName: "suite.TestFoo", FlakyCount: 3, RetrySum: 5, WastedMs: 12000, FailureRate: 0.4, Runs: 10, BuildsAffected: 3},
			}, nil
		},
	}
	h := NewFlakyImpactHandler(mock, nil, nil, zap.NewNop())

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/1/analytics/flaky", nil)
	req.SetPathValue("project_id", "1")
	rr := httptest.NewRecorder()
	h.GetFlakyImpact(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			Tests  []store.FlakyImpact `json:"tests"`
			Builds int                 `json:"builds"`
			Total  int                 `json:"total"`
		} `json:"data"`
		Metadata map[string]string `json:"metadata"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data.Tests) != 1 {
		t.Fatalf("expected 1 test entry, got %d", len(resp.Data.Tests))
	}
	if resp.Data.Tests[0].FullName != "suite.TestFoo" {
		t.Errorf("unexpected full_name: %q", resp.Data.Tests[0].FullName)
	}
	if resp.Data.Total != 1 {
		t.Errorf("expected total 1, got %d", resp.Data.Total)
	}
}

// TestGetFlakyImpact_ClampsBuildsAndLimit verifies that out-of-range builds/limit
// query params are clamped to the analytics max constants.
func TestGetFlakyImpact_ClampsBuildsAndLimit(t *testing.T) {
	var gotBuilds, gotLimit int
	mock := &testutil.MockAnalyticsStore{
		ListFlakyImpactFn: func(_ context.Context, _ int64, _ *int64, builds, limit int) ([]store.FlakyImpact, error) {
			gotBuilds = builds
			gotLimit = limit
			return nil, nil
		},
	}
	h := NewFlakyImpactHandler(mock, nil, nil, zap.NewNop())

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/1/analytics/flaky?builds=9999&limit=9999", nil)
	req.SetPathValue("project_id", "1")
	rr := httptest.NewRecorder()
	h.GetFlakyImpact(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if gotBuilds != analyticsMaxBuilds {
		t.Errorf("expected builds clamped to %d, got %d", analyticsMaxBuilds, gotBuilds)
	}
	if gotLimit != analyticsMaxLimit {
		t.Errorf("expected limit clamped to %d, got %d", analyticsMaxLimit, gotLimit)
	}

	// Zero/negative values fall back to defaults.
	req2, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/1/analytics/flaky?builds=0&limit=-5", nil)
	req2.SetPathValue("project_id", "1")
	rr2 := httptest.NewRecorder()
	h.GetFlakyImpact(rr2, req2)
	if gotBuilds != analyticsDefaultBuilds {
		t.Errorf("expected builds default %d, got %d", analyticsDefaultBuilds, gotBuilds)
	}
	if gotLimit != analyticsDefaultLimit {
		t.Errorf("expected limit default %d, got %d", analyticsDefaultLimit, gotLimit)
	}
}

// TestGetFlakyImpact_ProjectNotFound verifies a 404 is returned when the
// project slug cannot be resolved.
func TestGetFlakyImpact_ProjectNotFound(t *testing.T) {
	mocks := testutil.New()
	h := NewFlakyImpactHandler(mocks.Analytics, mocks.Branches, mocks.Projects, zap.NewNop())

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/does-not-exist/analytics/flaky", nil)
	req.SetPathValue("project_id", "does-not-exist")
	rr := httptest.NewRecorder()
	h.GetFlakyImpact(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestGetFlakyImpact_NilAnalyticsStore verifies the handler degrades
// gracefully (empty result, 200 OK) when analytics is unavailable.
func TestGetFlakyImpact_NilAnalyticsStore(t *testing.T) {
	h := NewFlakyImpactHandler(nil, nil, nil, zap.NewNop())

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/1/analytics/flaky", nil)
	req.SetPathValue("project_id", "1")
	rr := httptest.NewRecorder()
	h.GetFlakyImpact(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data map, got %T", resp["data"])
	}
	tests, ok := data["tests"].([]any)
	if !ok || len(tests) != 0 {
		t.Errorf("expected empty tests array, got %v", data["tests"])
	}
}
