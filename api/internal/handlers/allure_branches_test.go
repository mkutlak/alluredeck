package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func newTestBranchHandler(t *testing.T, mocks *testutil.MockStores) *BranchHandler {
	t.Helper()
	return NewBranchHandler(mocks.Branches, mocks.Builds, mocks.Projects)
}

// --- ListBranches tests ---

func TestListBranches_Empty(t *testing.T) {
	mocks := testutil.New()
	projectID := "1"
	mocks.Branches.ListFn = func(_ context.Context, _ int64) ([]store.Branch, error) {
		return []store.Branch{}, nil
	}

	h := newTestBranchHandler(t, mocks)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%s/branches", projectID), nil)
	req.SetPathValue("project_id", projectID)

	rr := httptest.NewRecorder()
	h.ListBranches(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", resp["data"])
	}
	branches, ok := data["branches"].([]any)
	if !ok {
		t.Fatalf("expected branches array, got %T", data["branches"])
	}
	if len(branches) != 0 {
		t.Errorf("expected empty branches, got %d", len(branches))
	}
}

func TestListBranches_WithData(t *testing.T) {
	mocks := testutil.New()
	projectID := "1"
	now := time.Now().UTC().Truncate(time.Second)
	mocks.Branches.ListFn = func(_ context.Context, _ int64) ([]store.Branch, error) {
		return []store.Branch{
			{ID: 1, ProjectID: 1, Name: "main", IsDefault: true, CreatedAt: now},
			{ID: 2, ProjectID: 1, Name: "dev", IsDefault: false, CreatedAt: now},
		}, nil
	}

	h := newTestBranchHandler(t, mocks)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		fmt.Sprintf("/api/v1/projects/%s/branches", projectID), nil)
	req.SetPathValue("project_id", projectID)

	rr := httptest.NewRecorder()
	h.ListBranches(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", resp["data"])
	}
	branches, ok := data["branches"].([]any)
	if !ok {
		t.Fatalf("expected branches array, got %T", data["branches"])
	}
	if len(branches) != 2 {
		t.Errorf("expected 2 branches, got %d", len(branches))
	}

	// Verify branch JSON shape
	entry, ok := branches[0].(map[string]any)
	if !ok {
		t.Fatalf("expected branch entry to be object, got %T", branches[0])
	}
	for _, field := range []string{"id", "project_id", "name", "is_default", "created_at"} {
		if _, exists := entry[field]; !exists {
			t.Errorf("missing field %q in branch entry", field)
		}
	}

	// metadata
	meta, ok := resp["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata object, got %T", resp["metadata"])
	}
	if msg, _ := meta["message"].(string); msg == "" {
		t.Error("expected non-empty metadata.message")
	}
}

// --- SetDefaultBranch tests ---

func TestSetDefaultBranch_Success(t *testing.T) {
	mocks := testutil.New()
	projectID := "1"
	mocks.Branches.SetDefaultFn = func(_ context.Context, _ int64, _ int64) error {
		return nil
	}

	h := newTestBranchHandler(t, mocks)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut,
		fmt.Sprintf("/api/v1/projects/%s/branches/2/default", projectID), nil)
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("branch_id", "2")

	rr := httptest.NewRecorder()
	h.SetDefaultBranch(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	meta, ok := resp["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata object, got %T", resp["metadata"])
	}
	if msg, _ := meta["message"].(string); msg == "" {
		t.Error("expected non-empty metadata.message")
	}
}

func TestSetDefaultBranch_NotFound(t *testing.T) {
	mocks := testutil.New()
	projectID := "1"
	mocks.Branches.SetDefaultFn = func(_ context.Context, _ int64, _ int64) error {
		return fmt.Errorf("%w: branch=9999 project=%s", store.ErrBranchNotFound, projectID)
	}

	h := newTestBranchHandler(t, mocks)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut,
		fmt.Sprintf("/api/v1/projects/%s/branches/9999/default", projectID), nil)
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("branch_id", "9999")

	rr := httptest.NewRecorder()
	h.SetDefaultBranch(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSetDefaultBranch_WrongProject(t *testing.T) {
	mocks := testutil.New()
	otherProject := "2"
	mocks.Branches.SetDefaultFn = func(_ context.Context, _ int64, _ int64) error {
		return fmt.Errorf("%w: branch does not belong to project", store.ErrBranchNotFound)
	}

	h := newTestBranchHandler(t, mocks)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut,
		fmt.Sprintf("/api/v1/projects/%s/branches/2/default", otherProject), nil)
	req.SetPathValue("project_id", otherProject)
	req.SetPathValue("branch_id", "2")

	rr := httptest.NewRecorder()
	h.SetDefaultBranch(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 for wrong project, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSetDefaultBranch_InvalidBranchID(t *testing.T) {
	mocks := testutil.New()
	projectID := "1"

	h := newTestBranchHandler(t, mocks)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut,
		fmt.Sprintf("/api/v1/projects/%s/branches/notanumber/default", projectID), nil)
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("branch_id", "notanumber")

	rr := httptest.NewRecorder()
	h.SetDefaultBranch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

// --- DeleteBranch tests ---

func TestDeleteBranch_Success(t *testing.T) {
	mocks := testutil.New()
	projectID := "1"
	mocks.Branches.DeleteFn = func(_ context.Context, _ int64, _ int64) error {
		return nil
	}

	h := newTestBranchHandler(t, mocks)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		fmt.Sprintf("/api/v1/projects/%s/branches/2", projectID), nil)
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("branch_id", "2")

	rr := httptest.NewRecorder()
	h.DeleteBranch(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteBranch_DefaultBranch_Conflict(t *testing.T) {
	mocks := testutil.New()
	projectID := "1"
	mocks.Branches.DeleteFn = func(_ context.Context, _ int64, _ int64) error {
		return store.ErrCannotDeleteDefaultBranch
	}

	h := newTestBranchHandler(t, mocks)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		fmt.Sprintf("/api/v1/projects/%s/branches/1", projectID), nil)
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("branch_id", "1")

	rr := httptest.NewRecorder()
	h.DeleteBranch(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteBranch_NotFound(t *testing.T) {
	mocks := testutil.New()
	projectID := "1"
	mocks.Branches.DeleteFn = func(_ context.Context, _ int64, _ int64) error {
		return fmt.Errorf("%w: branch=9999 project=%s", store.ErrBranchNotFound, projectID)
	}

	h := newTestBranchHandler(t, mocks)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		fmt.Sprintf("/api/v1/projects/%s/branches/9999", projectID), nil)
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("branch_id", "9999")

	rr := httptest.NewRecorder()
	h.DeleteBranch(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteBranch_InvalidBranchID(t *testing.T) {
	mocks := testutil.New()
	projectID := "1"

	h := newTestBranchHandler(t, mocks)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		fmt.Sprintf("/api/v1/projects/%s/branches/notanumber", projectID), nil)
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("branch_id", "notanumber")

	rr := httptest.NewRecorder()
	h.DeleteBranch(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
