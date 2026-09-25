package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSendResults_Async covers POST ?async=true: a gzip body is streamed to a
// staging blob and answered 202 {job_id, batch_id} before any extraction.
func TestSendResults_Async(t *testing.T) {
	t.Parallel()
	rows := []struct {
		name        string
		contentType string
		body        string
		want        int
	}{
		// Only the two magic bytes are sniffed; the worker is not exercised.
		{"gzip is staged", "application/gzip", "\x1f\x8b" + strings.Repeat("\x00", 32), http.StatusAccepted},
		{"non-gzip body rejected before any write", "application/gzip", "PK\x03\x04 zip header not gzip", http.StatusBadRequest},
		// Regression guard for the back-compat decision: JSON uploads ignore
		// the async flag and take the sync path.
		{"JSON ignores async", "application/json", `{"results":[{"file_name":"r.json","content_base64":"YQ=="}]}`, http.StatusOK},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			h, mocks := newTestResultUploadHandler(t, dir)
			if _, err := mocks.Projects.CreateProject(context.Background(), "asyncproj"); err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/asyncproj/results?async=true", strings.NewReader(tc.body))
			req.SetPathValue("project_id", "asyncproj")
			req.Header.Set("Content-Type", tc.contentType)
			rr := httptest.NewRecorder()
			h.SendResults(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			staged, _ := filepath.Glob(filepath.Join(dir, "staging", "*"))
			if tc.want != http.StatusAccepted {
				if len(staged) != 0 {
					t.Errorf("staged blobs = %v, want none", staged)
				}
				return
			}
			var resp struct {
				Data map[string]any `json:"data"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			batchID, _ := resp.Data["batch_id"].(string)
			if _, ok := resp.Data["job_id"]; !ok || batchID == "" {
				t.Fatalf("data = %v, want job_id and batch_id", resp.Data)
			}
			if _, err := os.Stat(filepath.Join(dir, "staging", batchID+".tar.gz")); err != nil {
				t.Errorf("staging blob not written: %v", err)
			}
		})
	}
}
