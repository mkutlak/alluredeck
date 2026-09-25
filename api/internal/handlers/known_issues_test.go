package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestKnownIssueHandler serves every known-issue endpoint against projects 1
// and 2, where project 1 owns issue 1, "Existing test".
func TestKnownIssueHandler(t *testing.T) {
	t.Parallel()
	list, create := (*KnownIssueHandler).ListKnownIssues, (*KnownIssueHandler).CreateKnownIssue
	update, del := (*KnownIssueHandler).UpdateKnownIssue, (*KnownIssueHandler).DeleteKnownIssue
	rows := []struct {
		name     string
		endpoint func(*KnownIssueHandler, http.ResponseWriter, *http.Request)
		project  string
		body     string
		want     int
		check    func(t *testing.T, h *KnownIssueHandler, data any)
	}{
		{"list", list, "1", "", http.StatusOK, wantKnownIssues(1)},
		{"list empty", list, "2", "", http.StatusOK, wantKnownIssues(0)},
		{"create", create, "1", `{"test_name":"Slow checkout test","ticket_url":"http://jira/PROJ-1","description":"Known performance issue"}`, http.StatusCreated,
			func(t *testing.T, _ *KnownIssueHandler, data any) {
				if d, _ := data.(map[string]any); d["test_name"] != "Slow checkout test" {
					t.Errorf("data = %v, want test_name Slow checkout test", data)
				}
			}},
		{"create duplicate", create, "1", `{"test_name":"Existing test"}`, http.StatusConflict, nil},
		{"create invalid project id", create, "../evil", `{"test_name":"Any test"}`, http.StatusBadRequest, nil},
		// XSS: ticket URLs must be http(s).
		{"create javascript URL", create, "1", `{"test_name":"XSS test","ticket_url":"javascript:alert(1)"}`, http.StatusBadRequest, nil},
		{"update", update, "1", `{"ticket_url":"http://new-ticket","description":"updated","is_active":false}`, http.StatusOK,
			func(t *testing.T, _ *KnownIssueHandler, data any) {
				if d, _ := data.(map[string]any); d["is_active"] != false || d["ticket_url"] != "http://new-ticket" {
					t.Errorf("data = %v, want is_active false and the new ticket_url", data)
				}
			}},
		{"update javascript URL", update, "1", `{"ticket_url":"javascript:alert(document.cookie)","description":"hacked","is_active":true}`, http.StatusBadRequest, nil},
		// Project isolation: another project's URL cannot reach the issue.
		{"update from another project", update, "2", `{"ticket_url":"http://evil","description":"hacked","is_active":true}`, http.StatusNotFound, nil},
		{"delete", del, "1", "", http.StatusOK, nil},
		{"delete from another project", del, "2", "", http.StatusNotFound,
			func(t *testing.T, h *KnownIssueHandler, _ any) {
				if _, err := h.knownIssueStore.Get(context.Background(), 1); err != nil {
					t.Errorf("issue 1 was deleted: %v", err)
				}
			}},
		{"known failures of a report", (*KnownIssueHandler).GetReportKnownFailures, "1", "", http.StatusOK, nil},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h, mocks := newTestKnownIssueHandler(t, t.TempDir())
			ctx := context.Background()
			p1, _ := mocks.Projects.CreateProject(ctx, "proj-1")
			_, _ = mocks.Projects.CreateProject(ctx, "proj-2")
			if _, err := h.knownIssueStore.Create(ctx, p1.ID, "Existing test", "", "http://ticket/1", "desc"); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+tc.project+"/known-issues", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", "application/json")
			req.SetPathValue("project_id", tc.project)
			req.SetPathValue("issue_id", "1")
			req.SetPathValue("report_id", "latest")
			rr := httptest.NewRecorder()
			tc.endpoint(h, rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			var resp struct {
				Data any `json:"data"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if tc.want < 300 && resp.Data == nil {
				t.Fatal("response has no data")
			}
			if tc.check != nil {
				tc.check(t, h, resp.Data)
			}
		})
	}
}

func wantKnownIssues(n int) func(*testing.T, *KnownIssueHandler, any) {
	return func(t *testing.T, _ *KnownIssueHandler, data any) {
		t.Helper()
		if issues, ok := data.([]any); !ok || len(issues) != n {
			t.Errorf("data = %v, want %d known issues", data, n)
		}
	}
}
