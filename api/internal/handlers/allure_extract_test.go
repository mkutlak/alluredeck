package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Valid, empty, and traversal project IDs are rows of TestExtractProjectID;
// report IDs are covered by TestValidateReportID and the *_TraversalReportID
// handler tests.
func TestExtractProjectID_InvalidEncoding(t *testing.T) {
	t.Parallel()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.SetPathValue("project_id", "%zz")
	w := httptest.NewRecorder()

	if _, ok := extractProjectID(w, req, t.TempDir()); ok || w.Code != http.StatusBadRequest {
		t.Fatalf("extractProjectID(%%zz) ok = %v, status = %d, want false and 400", ok, w.Code)
	}
	var resp struct {
		Metadata ResponseMeta `json:"metadata"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatal(err)
	}
	if resp.Metadata.Message != "invalid project_id encoding" {
		t.Errorf("message = %q, want %q", resp.Metadata.Message, "invalid project_id encoding")
	}
}
