package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractProjectID(t *testing.T) {
	t.Parallel()
	projectsDir := t.TempDir()

	tests := []struct {
		name       string
		pathValue  string
		wantOK     bool
		wantID     string
		wantStatus int
	}{
		{name: "empty defaults to default", pathValue: "", wantOK: true, wantID: "default"},
		{name: "normal id", pathValue: "my-project", wantOK: true, wantID: "my-project"},
		{name: "url encoded space", pathValue: "my%20project", wantOK: true, wantID: "my project"},
		{name: "path traversal rejected", pathValue: "../etc/passwd", wantOK: false, wantStatus: http.StatusBadRequest},
		{name: "double dot rejected", pathValue: "foo..bar", wantOK: false, wantStatus: http.StatusBadRequest},
		{name: "slash rejected", pathValue: "foo/bar", wantOK: false, wantStatus: http.StatusBadRequest},
		{name: "backslash rejected", pathValue: "foo\\bar", wantOK: false, wantStatus: http.StatusBadRequest},
		{name: "reserved name swagger", pathValue: "swagger", wantOK: false, wantStatus: http.StatusBadRequest},
		{name: "reserved name version", pathValue: "version", wantOK: false, wantStatus: http.StatusBadRequest},
		{name: "overlong id rejected", pathValue: strings.Repeat("a", 101), wantOK: false, wantStatus: http.StatusBadRequest},
		{name: "max length accepted", pathValue: strings.Repeat("a", 100), wantOK: true, wantID: strings.Repeat("a", 100)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			req.SetPathValue("project_id", tt.pathValue)
			rr := httptest.NewRecorder()

			gotID, gotOK := extractProjectID(rr, req, projectsDir)

			if gotOK != tt.wantOK {
				t.Fatalf("extractProjectID() ok = %v, want %v; body: %s", gotOK, tt.wantOK, rr.Body.String())
			}
			if tt.wantOK {
				if gotID != tt.wantID {
					t.Errorf("extractProjectID() id = %q, want %q", gotID, tt.wantID)
				}
			} else if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", rr.Code, tt.wantStatus, rr.Body.String())
			}
		})
	}
}
