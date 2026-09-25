package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"testing"
)

func TestValidateReportID(t *testing.T) {
	tests := []struct {
		input   string
		wantErr error
	}{
		{"latest", nil},
		{"1", nil},
		{"", ErrReportIDRequired},
		{"../evil", ErrReportIDInvalid},
		{"..", ErrReportIDInvalid},
		{"1/2", ErrReportIDInvalid},
		{"-1", ErrReportIDInvalid},
		{"1abc", ErrReportIDInvalid},
		{" ", ErrReportIDInvalid},
	}
	for _, tt := range tests {
		if err := validateReportID(tt.input); !errors.Is(err, tt.wantErr) {
			t.Errorf("validateReportID(%q) = %v, want %v", tt.input, err, tt.wantErr)
		}
	}
}

func TestValidateTicketURL(t *testing.T) {
	tests := []struct {
		input   string
		wantErr error
	}{
		{"", nil},
		{"http://jira.example.com/PROJ-1", nil},
		{"https://jira.example.com/PROJ-1", nil},
		{"javascript:alert(1)", ErrTicketURLInvalidScheme},
		{"data:text/html,<script>alert(1)</script>", ErrTicketURLInvalidScheme},
		{"vbscript:MsgBox(1)", ErrTicketURLInvalidScheme},
		{"jira.example.com/PROJ-1", ErrTicketURLInvalidScheme},
		{"ftp://files.example.com/report", ErrTicketURLInvalidScheme},
	}
	for _, tt := range tests {
		if err := validateTicketURL(tt.input); !errors.Is(err, tt.wantErr) {
			t.Errorf("validateTicketURL(%q) = %v, want %v", tt.input, err, tt.wantErr)
		}
	}
}

// TestReportEndpoints_TraversalReportID checks that every endpoint taking a
// report_id path value validates it before touching storage.
func TestReportEndpoints_TraversalReportID(t *testing.T) {
	tests := []struct {
		name  string
		serve func(*ReportHandler, http.ResponseWriter, *http.Request) // nil: the KnownIssueHandler endpoint
	}{
		{"environment", (*ReportHandler).GetReportEnvironment},
		{"timeline", (*ReportHandler).GetReportTimeline},
		{"stability", (*ReportHandler).GetReportStability},
		{"categories", (*ReportHandler).GetReportCategories},
		{"delete", (*ReportHandler).DeleteReport},
		{"known failures", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projectsDir := t.TempDir()
			rh, mocks := newTestReportHandler(t, projectsDir)
			fn := func(w http.ResponseWriter, r *http.Request) { tc.serve(rh, w, r) }
			if tc.serve == nil {
				kh, kmocks := newTestKnownIssueHandler(t, projectsDir)
				fn, mocks = kh.GetReportKnownFailures, kmocks
			}
			proj, err := mocks.Projects.CreateProject(context.Background(), "default")
			if err != nil {
				t.Fatal(err)
			}
			id := strconv.FormatInt(proj.ID, 10)
			code, body := serveJSON(t, fn, http.MethodGet, "/api/v1/projects/"+id+"/reports/x", "", "project_id", id, "report_id", "../evil")
			if code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400: %v", code, body)
			}
			wantJSON(t, body, map[string]any{"metadata.message": ErrReportIDInvalid.Error()})
		})
	}
}
