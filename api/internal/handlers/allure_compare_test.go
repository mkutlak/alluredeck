package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestCompareBuilds_Success(t *testing.T) {
	projectID := "1"

	mocks := testutil.New()
	mocks.TestResults.GetBuildIDFn = func(_ context.Context, pid int64, buildNumber int) (int64, error) {
		switch buildNumber {
		case 1:
			return 10, nil
		case 2:
			return 20, nil
		}
		return 0, store.ErrBuildNotFound
	}
	mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, pid int64, buildIDA, buildIDB int64) ([]store.DiffEntry, error) {
		return []store.DiffEntry{
			{TestName: "LoginTest", FullName: "pkg.LoginTest", HistoryID: "h1", StatusA: "passed", StatusB: "failed", DurationA: 1000, DurationB: 2000, Category: store.DiffRegressed},
			{TestName: "NewTest", FullName: "pkg.NewTest", HistoryID: "h3", StatusA: "", StatusB: "passed", DurationA: 0, DurationB: 300, Category: store.DiffAdded},
		}, nil
	}

	h := newTestCompareHandler(t, mocks)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%s/compare?a=1&b=2", projectID), nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)

	rr := httptest.NewRecorder()
	h.CompareBuilds(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data to be object, got %T", resp["data"])
	}

	// Verify summary fields exist
	summary, ok := data["summary"].(map[string]any)
	if !ok {
		t.Fatalf("expected summary to be object, got %T", data["summary"])
	}
	if total, _ := summary["total"].(float64); int(total) == 0 {
		t.Errorf("expected non-zero total in summary")
	}

	// Verify tests array
	tests, ok := data["tests"].([]any)
	if !ok {
		t.Fatalf("expected tests to be array, got %T", data["tests"])
	}
	if len(tests) == 0 {
		t.Error("expected non-empty tests array")
	}

	// Verify build_a and build_b are present
	if ba, _ := data["build_a"].(float64); int(ba) != 1 {
		t.Errorf("expected build_a=1, got %v", data["build_a"])
	}
	if bb, _ := data["build_b"].(float64); int(bb) != 2 {
		t.Errorf("expected build_b=2, got %v", data["build_b"])
	}

	// Verify test entry structure
	entry, ok := tests[0].(map[string]any)
	if !ok {
		t.Fatalf("expected test entry to be object, got %T", tests[0])
	}
	for _, field := range []string{"test_name", "full_name", "history_id", "status_a", "status_b", "duration_a", "duration_b", "duration_delta", "category"} {
		if _, exists := entry[field]; !exists {
			t.Errorf("missing field %q in test entry", field)
		}
	}

	// metadata
	meta, ok := resp["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata to be object, got %T", resp["metadata"])
	}
	if msg, _ := meta["message"].(string); msg == "" {
		t.Error("expected non-empty metadata.message")
	}
}

func TestCompareBuilds_MissingParams(t *testing.T) {
	h := newTestCompareHandler(t, testutil.New())

	cases := []struct {
		name  string
		query string
	}{
		{"missing both", ""},
		{"missing b", "?a=1"},
		{"missing a", "?b=2"},
		{"a not integer", "?a=foo&b=2"},
		{"b not integer", "?a=1&b=bar"},
		{"a zero", "?a=0&b=2"},
		{"b zero", "?a=1&b=0"},
		{"a negative", "?a=-1&b=2"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
				"/api/v1/projects/1/compare"+tc.query, nil)
			req.SetPathValue("project_id", "1")

			rr := httptest.NewRecorder()
			h.CompareBuilds(rr, req)

			if rr.Code != http.StatusBadRequest {
				t.Errorf("want 400, got %d: %s", rr.Code, rr.Body.String())
			}
		})
	}
}

func TestCompareBuilds_SameBuild(t *testing.T) {
	h := newTestCompareHandler(t, testutil.New())

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/1/compare?a=1&b=1", nil)
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.CompareBuilds(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	meta, _ := resp["metadata"].(map[string]any)
	msg, _ := meta["message"].(string)
	if msg != "build_a and build_b must be different" {
		t.Errorf("unexpected message: %q", msg)
	}
}

func TestCompareBuilds_BuildNotFound(t *testing.T) {
	mocks := testutil.New()
	mocks.TestResults.GetBuildIDFn = func(_ context.Context, pid int64, buildNumber int) (int64, error) {
		if buildNumber == 99 {
			return 0, store.ErrBuildNotFound
		}
		return 10, nil
	}

	h := newTestCompareHandler(t, mocks)

	// Build 99 does not exist
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/1/compare?a=1&b=99", nil)
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.CompareBuilds(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCompareBuilds_InvalidProjectID(t *testing.T) {
	h := newTestCompareHandler(t, testutil.New())

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/../evil/compare?a=1&b=2", nil)
	req.SetPathValue("project_id", "../evil")

	rr := httptest.NewRecorder()
	h.CompareBuilds(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCompareBuilds_NoStore(t *testing.T) {
	// When testResultStore is nil, handler returns empty data (same pattern as analytics)
	// newTestCompareHandler with nil testResultStore
	h := NewCompareHandler(nil, nil)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/1/compare?a=1&b=2", nil)
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.CompareBuilds(rr, req)

	// nil store returns empty response, not an error
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200 with nil store, got %d: %s", rr.Code, rr.Body.String())
	}
}
