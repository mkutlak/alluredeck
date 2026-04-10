package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
)

func TestValidateReportID(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"latest is valid", "latest", nil},
		{"numeric 1 is valid", "1", nil},
		{"numeric 42 is valid", "42", nil},
		{"numeric 999 is valid", "999", nil},
		{"empty is required", "", ErrReportIDRequired},
		{"traversal ../ is invalid", "../evil", ErrReportIDInvalid},
		{"traversal ../../etc/passwd is invalid", "../../etc/passwd", ErrReportIDInvalid},
		{"alphabetic abc is invalid", "abc", ErrReportIDInvalid},
		{"slash 1/2 is invalid", "1/2", ErrReportIDInvalid},
		{"negative -1 is invalid", "-1", ErrReportIDInvalid},
		{"dot-dot .. is invalid", "..", ErrReportIDInvalid},
		{"mixed 1abc is invalid", "1abc", ErrReportIDInvalid},
		{"space is invalid", " ", ErrReportIDInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateReportID(tt.input)
			if err != tt.wantErr {
				t.Errorf("validateReportID(%q) = %v, want %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestValidateTicketURL(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"empty is allowed", "", nil},
		{"http is valid", "http://jira.example.com/PROJ-1", nil},
		{"https is valid", "https://jira.example.com/PROJ-1", nil},
		{"javascript scheme is rejected", "javascript:alert(1)", ErrTicketURLInvalidScheme},
		{"data scheme is rejected", "data:text/html,<script>alert(1)</script>", ErrTicketURLInvalidScheme},
		{"vbscript scheme is rejected", "vbscript:MsgBox(1)", ErrTicketURLInvalidScheme},
		{"no scheme is rejected", "jira.example.com/PROJ-1", ErrTicketURLInvalidScheme},
		{"ftp scheme is rejected", "ftp://files.example.com/report", ErrTicketURLInvalidScheme},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateTicketURL(tt.input)
			if err != tt.wantErr {
				t.Errorf("validateTicketURL(%q) = %v, want %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

func TestGetReportEnvironment_TraversalReportID(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestReportHandler(t, projectsDir)

	proj, err := mocks.Projects.CreateProject(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	projectIDStr := strconv.FormatInt(proj.ID, 10)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/../evil/environment", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
	req.SetPathValue("report_id", "../evil")

	rr := httptest.NewRecorder()
	h.GetReportEnvironment(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	meta, _ := resp["metadata"].(map[string]any)
	if msg, _ := meta["message"].(string); msg != ErrReportIDInvalid.Error() {
		t.Errorf("message = %q, want %q", msg, ErrReportIDInvalid.Error())
	}
}

func TestGetReportTimeline_TraversalReportID(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestReportHandler(t, projectsDir)

	proj, err := mocks.Projects.CreateProject(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	projectIDStr := strconv.FormatInt(proj.ID, 10)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/../evil/timeline", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
	req.SetPathValue("report_id", "../evil")

	rr := httptest.NewRecorder()
	h.GetReportTimeline(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetReportStability_TraversalReportID(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestReportHandler(t, projectsDir)

	proj, err := mocks.Projects.CreateProject(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	projectIDStr := strconv.FormatInt(proj.ID, 10)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/../evil/stability", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
	req.SetPathValue("report_id", "../evil")

	rr := httptest.NewRecorder()
	h.GetReportStability(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetReportCategories_TraversalReportID(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestReportHandler(t, projectsDir)

	proj, err := mocks.Projects.CreateProject(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	projectIDStr := strconv.FormatInt(proj.ID, 10)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/../evil/categories", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
	req.SetPathValue("report_id", "../evil")

	rr := httptest.NewRecorder()
	h.GetReportCategories(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestGetReportKnownFailures_TraversalReportID(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestKnownIssueHandler(t, projectsDir)

	proj, err := mocks.Projects.CreateProject(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	projectIDStr := strconv.FormatInt(proj.ID, 10)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/"+projectIDStr+"/reports/../evil/known-failures", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
	req.SetPathValue("report_id", "../evil")

	rr := httptest.NewRecorder()
	h.GetReportKnownFailures(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteReport_TraversalReportID(t *testing.T) {
	projectsDir := t.TempDir()
	h, mocks := newTestReportHandler(t, projectsDir)

	proj, err := mocks.Projects.CreateProject(context.Background(), "default")
	if err != nil {
		t.Fatal(err)
	}
	projectIDStr := strconv.FormatInt(proj.ID, 10)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		"/api/v1/projects/"+projectIDStr+"/reports/../evil", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectIDStr)
	req.SetPathValue("report_id", "../evil")

	rr := httptest.NewRecorder()
	h.DeleteReport(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}
