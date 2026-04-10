package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListKnownIssues_Empty(t *testing.T) {
	projectsDir := t.TempDir()
	h, _ := newTestKnownIssueHandler(t, projectsDir)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/1/known-issues", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", "1")

	rr := httptest.NewRecorder()
	h.ListKnownIssues(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].([]any)
	if len(data) != 0 {
		t.Fatalf("expected empty array, got %d items", len(data))
	}
}

func TestListKnownIssues_WithEntries(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestKnownIssueHandler(t, projectsDir)

	// Pre-create project and known issue via store
	ctx := context.Background()
	proj, err := mocks.Projects.CreateProject(ctx, "testproj")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.knownIssueStore.Create(ctx, proj.ID, "My flaky test", "", "http://ticket/1", "desc"); err != nil {
		t.Fatal(err)
	}
	projectID := fmt.Sprintf("%d", proj.ID)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectID+"/known-issues", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)

	rr := httptest.NewRecorder()
	h.ListKnownIssues(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("expected 1 item, got %d", len(data))
	}
}

func TestCreateKnownIssue_Success(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestKnownIssueHandler(t, projectsDir)

	ctx := context.Background()
	proj, err := mocks.Projects.CreateProject(ctx, "cproj")
	if err != nil {
		t.Fatal(err)
	}
	projectID := fmt.Sprintf("%d", proj.ID)

	body := map[string]any{
		"test_name":   "Slow checkout test",
		"pattern":     "",
		"ticket_url":  "http://jira/PROJ-1",
		"description": "Known performance issue",
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/projects/"+projectID+"/known-issues",
		bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", projectID)

	rr := httptest.NewRecorder()
	h.CreateKnownIssue(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("want 201, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	data, _ := resp["data"].(map[string]any)
	if data == nil {
		t.Fatal("expected data object")
	}
	if data["test_name"] != "Slow checkout test" {
		t.Errorf("test_name = %v, want %q", data["test_name"], "Slow checkout test")
	}
}

func TestCreateKnownIssue_Duplicate(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestKnownIssueHandler(t, projectsDir)

	ctx := context.Background()
	proj, err := mocks.Projects.CreateProject(ctx, "dproj")
	if err != nil {
		t.Fatal(err)
	}
	projectID := fmt.Sprintf("%d", proj.ID)
	if _, err := h.knownIssueStore.Create(ctx, proj.ID, "Dup test", "", "", ""); err != nil {
		t.Fatal(err)
	}

	body := map[string]any{"test_name": "Dup test"}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/projects/"+projectID+"/known-issues",
		bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", projectID)

	rr := httptest.NewRecorder()
	h.CreateKnownIssue(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("want 409 Conflict, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestCreateKnownIssue_InvalidProject(t *testing.T) {
	projectsDir := t.TempDir()
	h, _ := newTestKnownIssueHandler(t, projectsDir)

	body := map[string]any{"test_name": "Any test"}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/projects/../evil/known-issues",
		bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "../evil")

	rr := httptest.NewRecorder()
	h.CreateKnownIssue(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d", rr.Code)
	}
}

func TestUpdateKnownIssue_ToggleActive(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestKnownIssueHandler(t, projectsDir)

	ctx := context.Background()
	proj, err := mocks.Projects.CreateProject(ctx, "uproj")
	if err != nil {
		t.Fatal(err)
	}
	projectID := fmt.Sprintf("%d", proj.ID)
	issue, err := h.knownIssueStore.Create(ctx, proj.ID, "Toggle me", "", "", "")
	if err != nil {
		t.Fatal(err)
	}

	body := map[string]any{
		"ticket_url":  "http://new-ticket",
		"description": "updated",
		"is_active":   false,
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPut,
		"/api/v1/projects/"+projectID+"/known-issues/1",
		bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("issue_id", fmt.Sprintf("%d", issue.ID))

	rr := httptest.NewRecorder()
	h.UpdateKnownIssue(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteKnownIssue_Success(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestKnownIssueHandler(t, projectsDir)

	ctx := context.Background()
	proj, err := mocks.Projects.CreateProject(ctx, "delproj")
	if err != nil {
		t.Fatal(err)
	}
	projectID := fmt.Sprintf("%d", proj.ID)
	issue, err := h.knownIssueStore.Create(ctx, proj.ID, "Delete me", "", "", "")
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		"/api/v1/projects/"+projectID+"/known-issues/1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("issue_id", fmt.Sprintf("%d", issue.ID))

	rr := httptest.NewRecorder()
	h.DeleteKnownIssue(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateKnownIssue_CrossProjectRejected(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestKnownIssueHandler(t, projectsDir)

	ctx := context.Background()
	projA, err := mocks.Projects.CreateProject(ctx, "projA")
	if err != nil {
		t.Fatal(err)
	}
	projB, err := mocks.Projects.CreateProject(ctx, "projB")
	if err != nil {
		t.Fatal(err)
	}
	projBID := fmt.Sprintf("%d", projB.ID)
	issue, err := h.knownIssueStore.Create(ctx, projA.ID, "Test in projA", "", "http://ticket/1", "desc")
	if err != nil {
		t.Fatal(err)
	}

	body := map[string]any{
		"ticket_url":  "http://evil",
		"description": "hacked",
		"is_active":   true,
	}
	bodyBytes, _ := json.Marshal(body)

	// Try updating projA's issue via projB's URL.
	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		"/api/v1/projects/"+projBID+"/known-issues/1",
		bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", projBID)
	req.SetPathValue("issue_id", fmt.Sprintf("%d", issue.ID))

	rr := httptest.NewRecorder()
	h.UpdateKnownIssue(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteKnownIssue_CrossProjectRejected(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestKnownIssueHandler(t, projectsDir)

	ctx := context.Background()
	projA, err := mocks.Projects.CreateProject(ctx, "projA")
	if err != nil {
		t.Fatal(err)
	}
	projB, err := mocks.Projects.CreateProject(ctx, "projB")
	if err != nil {
		t.Fatal(err)
	}
	projBID := fmt.Sprintf("%d", projB.ID)
	issue, err := h.knownIssueStore.Create(ctx, projA.ID, "Test in projA", "", "", "")
	if err != nil {
		t.Fatal(err)
	}

	// Try deleting projA's issue via projB's URL.
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		"/api/v1/projects/"+projBID+"/known-issues/1", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projBID)
	req.SetPathValue("issue_id", fmt.Sprintf("%d", issue.ID))

	rr := httptest.NewRecorder()
	h.DeleteKnownIssue(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}

	// Verify issue still exists in projA.
	if _, err := h.knownIssueStore.Get(ctx, issue.ID); err != nil {
		t.Fatalf("issue should still exist: %v", err)
	}
}

func TestCreateKnownIssue_RejectsJavascriptURL(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestKnownIssueHandler(t, projectsDir)

	ctx := context.Background()
	proj, err := mocks.Projects.CreateProject(ctx, "xss")
	if err != nil {
		t.Fatal(err)
	}
	projectID := fmt.Sprintf("%d", proj.ID)

	body := map[string]any{
		"test_name":  "XSS test",
		"ticket_url": "javascript:alert(1)",
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		"/api/v1/projects/"+projectID+"/known-issues",
		bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", projectID)

	rr := httptest.NewRecorder()
	h.CreateKnownIssue(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestUpdateKnownIssue_RejectsJavascriptURL(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestKnownIssueHandler(t, projectsDir)

	ctx := context.Background()
	proj, err := mocks.Projects.CreateProject(ctx, "xss2")
	if err != nil {
		t.Fatal(err)
	}
	projectID := fmt.Sprintf("%d", proj.ID)
	issue, err := h.knownIssueStore.Create(ctx, proj.ID, "Some test", "", "http://valid", "desc")
	if err != nil {
		t.Fatal(err)
	}

	body := map[string]any{
		"ticket_url":  "javascript:alert(document.cookie)",
		"description": "hacked",
		"is_active":   true,
	}
	bodyBytes, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut,
		"/api/v1/projects/"+projectID+"/known-issues/1",
		bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("issue_id", fmt.Sprintf("%d", issue.ID))

	rr := httptest.NewRecorder()
	h.UpdateKnownIssue(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetReportKnownFailures_NoKnown(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestKnownIssueHandler(t, projectsDir)

	ctx := context.Background()
	proj, err := mocks.Projects.CreateProject(ctx, "kfproj")
	if err != nil {
		t.Fatal(err)
	}
	projectID := fmt.Sprintf("%d", proj.ID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"/api/v1/projects/"+projectID+"/reports/latest/known-failures", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("report_id", "latest")

	rr := httptest.NewRecorder()
	h.GetReportKnownFailures(rr, req)

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
}
