package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func newTestDefectHandler(t *testing.T) *DefectHandler {
	t.Helper()
	mocks := testutil.New()
	return NewDefectHandler(mocks.Defects, zap.NewNop())
}

func TestListProjectDefects_Empty(t *testing.T) {
	h := newTestDefectHandler(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/default/defects", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "default")

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
	if resp["total"] == nil {
		t.Fatal("expected total field in response")
	}
}

func TestGetProjectDefectSummary(t *testing.T) {
	h := newTestDefectHandler(t)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/default/defects/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "default")

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
		"/api/v1/projects/default/builds/1/defects/summary", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "default")
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
		"/api/v1/projects/default/defects/nonexistent-id", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "default")
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
		"/api/v1/projects/default/defects/some-id", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "default")
	req.SetPathValue("defect_id", "some-id")

	rr := httptest.NewRecorder()
	h.UpdateDefect(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestBulkUpdateDefects_EmptyIDs(t *testing.T) {
	h := newTestDefectHandler(t)

	body, _ := json.Marshal(map[string]any{
		"defect_ids": []string{},
		"resolution": "fixed",
	})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/projects/default/defects/bulk", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "default")

	rr := httptest.NewRecorder()
	h.BulkUpdateDefects(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
