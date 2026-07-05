package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// fakeDefectTestResultsStore decorates MemDefectStore to return seeded test
// results from GetTestResults, since MemDefectStore.GetTestResults always
// returns an empty slice. This is needed to exercise the flaky/retries field
// mapping in defectTestResp.
type fakeDefectTestResultsStore struct {
	*testutil.MemDefectStore
	results []store.TestResult
}

var _ store.DefectStorer = (*fakeDefectTestResultsStore)(nil)

func (f *fakeDefectTestResultsStore) GetTestResults(_ context.Context, _ string, _ *int64, _, _ int) ([]store.TestResult, int, error) {
	return f.results, len(f.results), nil
}

func newTestDefectHandler(t *testing.T) *DefectHandler {
	t.Helper()
	mocks := testutil.New()
	return NewDefectHandler(mocks.Defects, mocks.Projects, zap.NewNop())
}

func TestListProjectDefects_Empty(t *testing.T) {
	h := newTestDefectHandler(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/1/defects", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.ListProjectDefects(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].([]any)
	if len(data) != 0 {
		t.Fatalf("expected empty data array, got %d items", len(data))
	}
	pg, _ := resp["pagination"].(map[string]any)
	if pg == nil {
		t.Fatal("expected pagination field in response")
	}
	if pg["total"] == nil {
		t.Fatal("expected total in pagination")
	}
}

func TestGetProjectDefectSummary(t *testing.T) {
	h := newTestDefectHandler(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/1/defects/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.GetProjectDefectSummary(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["data"] == nil {
		t.Fatal("expected data field in summary response")
	}
}

func TestGetBuildDefectSummary(t *testing.T) {
	h := newTestDefectHandler(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/1/builds/1/defects/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")
	req.SetPathValue("build_id", "1")

	rr := httptest.NewRecorder()
	h.GetBuildDefectSummary(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp["data"] == nil {
		t.Fatal("expected data field in build summary response")
	}
}

func TestGetDefect_NotFound(t *testing.T) {
	h := newTestDefectHandler(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/1/defects/nonexistent-id", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")
	req.SetPathValue("defect_id", "nonexistent-id")

	rr := httptest.NewRecorder()
	h.GetDefect(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateDefect_InvalidCategory(t *testing.T) {
	h := newTestDefectHandler(t)

	body, _ := json.Marshal(map[string]any{
		"category": "not_a_valid_category",
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPatch,
		"/api/v1/projects/1/defects/some-id", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "1")
	req.SetPathValue("defect_id", "some-id")

	rr := httptest.NewRecorder()
	h.UpdateDefect(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// TestGetDefectTests_IncludesFlakyAndRetries verifies that the GetDefectTests
// response DTO exposes flaky/retries so the UI can badge defect-linked tests.
func TestGetDefectTests_IncludesFlakyAndRetries(t *testing.T) {
	defectStore := &fakeDefectTestResultsStore{
		MemDefectStore: testutil.NewMemDefectStore(),
		results: []store.TestResult{
			{BuildID: 5, TestName: "t1", FullName: "suite.t1", Status: "failed", Flaky: true, Retries: 2},
		},
	}
	h := NewDefectHandler(defectStore, testutil.New().Projects, zap.NewNop())

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/1/defects/fp-1/tests", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")
	req.SetPathValue("defect_id", "fp-1")

	rr := httptest.NewRecorder()
	h.GetDefectTests(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data []struct {
			TestName string `json:"test_name"`
			Flaky    bool   `json:"flaky"`
			Retries  int    `json:"retries"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 test result, got %d", len(resp.Data))
	}
	if !resp.Data[0].Flaky {
		t.Error("expected flaky=true")
	}
	if resp.Data[0].Retries != 2 {
		t.Errorf("expected retries=2, got %d", resp.Data[0].Retries)
	}
}

func TestBulkUpdateDefects_EmptyIDs(t *testing.T) {
	h := newTestDefectHandler(t)

	body, _ := json.Marshal(map[string]any{
		"defect_ids": []string{},
		"resolution": "fixed",
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/projects/1/defects/bulk", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.BulkUpdateDefects(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
