package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestUpdateProjectTags_Success(t *testing.T) {
	h := newTestAllureHandler(t, t.TempDir())

	ctx := context.Background()
	if err := h.projectStore.CreateProject(ctx, "tagproj"); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]any{"tags": []string{"backend", "nightly"}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		"/api/v1/projects/tagproj/tags", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "tagproj")

	rr := httptest.NewRecorder()
	h.UpdateProjectTags(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data object")
	}
	tags, _ := data["tags"].([]any)
	if len(tags) != 2 {
		t.Errorf("expected 2 tags, got %d: %v", len(tags), tags)
	}
}

func TestUpdateProjectTags_EmptyTags(t *testing.T) {
	h := newTestAllureHandler(t, t.TempDir())

	ctx := context.Background()
	if err := h.projectStore.CreateProject(ctx, "clearproj"); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]any{"tags": []string{}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		"/api/v1/projects/clearproj/tags", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "clearproj")

	rr := httptest.NewRecorder()
	h.UpdateProjectTags(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateProjectTags_InvalidChars(t *testing.T) {
	h := newTestAllureHandler(t, t.TempDir())

	ctx := context.Background()
	if err := h.projectStore.CreateProject(ctx, "validproj"); err != nil {
		t.Fatal(err)
	}

	body, _ := json.Marshal(map[string]any{"tags": []string{"invalid tag!"}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		"/api/v1/projects/validproj/tags", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "validproj")

	rr := httptest.NewRecorder()
	h.UpdateProjectTags(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for invalid tag chars, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateProjectTags_TooManyTags(t *testing.T) {
	h := newTestAllureHandler(t, t.TempDir())

	ctx := context.Background()
	if err := h.projectStore.CreateProject(ctx, "manytagsproj"); err != nil {
		t.Fatal(err)
	}

	tags := make([]string, 21)
	for i := range tags {
		tags[i] = "tag"
	}
	body, _ := json.Marshal(map[string]any{"tags": tags})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		"/api/v1/projects/manytagsproj/tags", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "manytagsproj")

	rr := httptest.NewRecorder()
	h.UpdateProjectTags(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for too many tags, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateProjectTags_TagTooLong(t *testing.T) {
	h := newTestAllureHandler(t, t.TempDir())

	ctx := context.Background()
	if err := h.projectStore.CreateProject(ctx, "longtagproj"); err != nil {
		t.Fatal(err)
	}

	longTag := ""
	for i := 0; i < 51; i++ {
		longTag += "a"
	}
	body, _ := json.Marshal(map[string]any{"tags": []string{longTag}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		"/api/v1/projects/longtagproj/tags", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "longtagproj")

	rr := httptest.NewRecorder()
	h.UpdateProjectTags(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400 for tag too long, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateProjectTags_ProjectNotFound(t *testing.T) {
	h := newTestAllureHandler(t, t.TempDir())

	body, _ := json.Marshal(map[string]any{"tags": []string{"backend"}})
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut,
		"/api/v1/projects/ghost/tags", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "ghost")

	rr := httptest.NewRecorder()
	h.UpdateProjectTags(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestListTags_Empty(t *testing.T) {
	h := newTestAllureHandler(t, t.TempDir())

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/tags", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	h.ListTags(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].([]any)
	if len(data) != 0 {
		t.Errorf("expected empty tags, got %v", data)
	}
}

func TestListTags_ReturnsDistinctTags(t *testing.T) {
	h := newTestAllureHandler(t, t.TempDir())

	ctx := context.Background()
	for _, id := range []string{"proj-a", "proj-b"} {
		if err := h.projectStore.CreateProject(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := h.projectStore.SetTags(ctx, "proj-a", []string{"backend", "nightly"}); err != nil {
		t.Fatal(err)
	}
	if err := h.projectStore.SetTags(ctx, "proj-b", []string{"backend", "frontend"}); err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/tags", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	h.ListTags(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].([]any)
	// backend, frontend, nightly — 3 distinct tags
	if len(data) != 3 {
		t.Errorf("expected 3 distinct tags, got %d: %v", len(data), data)
	}
}
