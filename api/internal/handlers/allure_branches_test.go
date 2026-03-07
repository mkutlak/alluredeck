package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// newTestBranchHandler creates a BranchHandler backed by a real SQLite store.
func newTestBranchHandler(t *testing.T, db *store.SQLiteStore) *BranchHandler {
	t.Helper()
	logger := zap.NewNop()
	bs := store.NewBranchStore(db)
	buildStore := store.NewBuildStore(db, logger)
	return NewBranchHandler(bs, buildStore)
}

// seedBranchProject creates a project and seeds branches for tests.
func seedBranchProject(t *testing.T, db *store.SQLiteStore, projectID string) (*store.Branch, *store.Branch) {
	t.Helper()
	ctx := context.Background()
	ps := store.NewProjectStore(db, zap.NewNop())
	if err := ps.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	bs := store.NewBranchStore(db)
	main, _, err := bs.GetOrCreate(ctx, projectID, "main")
	if err != nil {
		t.Fatalf("GetOrCreate main: %v", err)
	}
	dev, _, err := bs.GetOrCreate(ctx, projectID, "dev")
	if err != nil {
		t.Fatalf("GetOrCreate dev: %v", err)
	}
	return main, dev
}

// --- ListBranches tests ---

func TestListBranches_Empty(t *testing.T) {
	db := openTestStore(t)
	ctx := context.Background()
	ps := store.NewProjectStore(db, zap.NewNop())
	projectID := "branch-empty"
	if err := ps.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	h := newTestBranchHandler(t, db)

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
	db := openTestStore(t)
	projectID := "branch-list"
	seedBranchProject(t, db, projectID)

	h := newTestBranchHandler(t, db)

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
	db := openTestStore(t)
	projectID := "branch-setdefault"
	main, dev := seedBranchProject(t, db, projectID)

	// main is default; set dev as default
	_ = main
	h := newTestBranchHandler(t, db)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut,
		fmt.Sprintf("/api/v1/projects/%s/branches/%d/default", projectID, dev.ID), nil)
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("branch_id", fmt.Sprintf("%d", dev.ID))

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
	db := openTestStore(t)
	projectID := "branch-setdefault-404"
	ctx := context.Background()
	ps := store.NewProjectStore(db, zap.NewNop())
	if err := ps.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	h := newTestBranchHandler(t, db)

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
	db := openTestStore(t)
	projectID := "branch-setdefault-wrongproj"
	_, dev := seedBranchProject(t, db, projectID)

	ctx := context.Background()
	ps := store.NewProjectStore(db, zap.NewNop())
	otherProject := "other-project"
	if err := ps.CreateProject(ctx, otherProject); err != nil {
		t.Fatalf("CreateProject other: %v", err)
	}

	h := newTestBranchHandler(t, db)

	// dev belongs to projectID, but we request with otherProject
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPut,
		fmt.Sprintf("/api/v1/projects/%s/branches/%d/default", otherProject, dev.ID), nil)
	req.SetPathValue("project_id", otherProject)
	req.SetPathValue("branch_id", fmt.Sprintf("%d", dev.ID))

	rr := httptest.NewRecorder()
	h.SetDefaultBranch(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404 for wrong project, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSetDefaultBranch_InvalidBranchID(t *testing.T) {
	db := openTestStore(t)
	projectID := "branch-setdefault-badid"
	ctx := context.Background()
	ps := store.NewProjectStore(db, zap.NewNop())
	if err := ps.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	h := newTestBranchHandler(t, db)

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
	db := openTestStore(t)
	projectID := "branch-delete"
	_, dev := seedBranchProject(t, db, projectID)

	h := newTestBranchHandler(t, db)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		fmt.Sprintf("/api/v1/projects/%s/branches/%d", projectID, dev.ID), nil)
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("branch_id", fmt.Sprintf("%d", dev.ID))

	rr := httptest.NewRecorder()
	h.DeleteBranch(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("want 204, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteBranch_DefaultBranch_Conflict(t *testing.T) {
	db := openTestStore(t)
	projectID := "branch-delete-default"
	main, _ := seedBranchProject(t, db, projectID)

	h := newTestBranchHandler(t, db)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		fmt.Sprintf("/api/v1/projects/%s/branches/%d", projectID, main.ID), nil)
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("branch_id", fmt.Sprintf("%d", main.ID))

	rr := httptest.NewRecorder()
	h.DeleteBranch(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteBranch_NotFound(t *testing.T) {
	db := openTestStore(t)
	projectID := "branch-delete-404"
	ctx := context.Background()
	ps := store.NewProjectStore(db, zap.NewNop())
	if err := ps.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	h := newTestBranchHandler(t, db)

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
	db := openTestStore(t)
	projectID := "branch-delete-badid"
	ctx := context.Background()
	ps := store.NewProjectStore(db, zap.NewNop())
	if err := ps.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	h := newTestBranchHandler(t, db)

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
