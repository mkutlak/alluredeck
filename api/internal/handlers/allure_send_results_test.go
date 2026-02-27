package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// makeJSONSendResultsReq builds a POST request with a JSON body containing the
// given results slice. It mirrors the wire format expected by sendJSONResults.
func makeJSONSendResultsReq(t *testing.T, projectID string, results []map[string]string) *http.Request {
	t.Helper()
	body, err := json.Marshal(map[string]any{"results": results})
	if err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/api/v1/projects/"+projectID+"/results",
		bytes.NewReader(body),
	)
	if err != nil {
		t.Fatal(err)
	}
	req.SetPathValue("project_id", projectID)
	req.Header.Set("Content-Type", "application/json")
	return req
}

// TestSendJSONResults_WritesFileCorrectly verifies that a single base64-encoded
// file is decoded and written to disk with the correct content.
// This is the primary regression test for the streaming-decode implementation.
func TestSendJSONResults_WritesFileCorrectly(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "proj1"
	resultsDir := filepath.Join(projectsDir, projectID, "results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil { //nolint:gosec // G301: test fixtures run in isolated t.TempDir(); relaxed permissions are acceptable
		t.Fatal(err)
	}

	wantContent := []byte("<allure-result><status>passed</status></allure-result>")
	encoded := base64.StdEncoding.EncodeToString(wantContent)

	req := makeJSONSendResultsReq(t, projectID, []map[string]string{
		{"file_name": "test-result.xml", "content_base64": encoded},
	})

	h := newTestAllureHandler(t, projectsDir)
	processed, failed, err := h.sendJSONResults(req, projectID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(failed) != 0 {
		t.Fatalf("expected no failed files, got %v", failed)
	}
	if len(processed) != 1 || processed[0] != "test-result.xml" {
		t.Fatalf("expected processed=[test-result.xml], got %v", processed)
	}

	got, err := os.ReadFile(filepath.Join(resultsDir, "test-result.xml"))
	if err != nil {
		t.Fatalf("result file not written to disk: %v", err)
	}
	if !bytes.Equal(got, wantContent) {
		t.Errorf("file content mismatch:\n got  %q\n want %q", got, wantContent)
	}
}

// TestSendJSONResults_MultipleFiles verifies all files in a batch are written
// correctly, each with the right decoded content.
func TestSendJSONResults_MultipleFiles(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "proj2"
	resultsDir := filepath.Join(projectsDir, projectID, "results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil { //nolint:gosec // G301: test fixtures run in isolated t.TempDir(); relaxed permissions are acceptable
		t.Fatal(err)
	}

	files := []struct {
		name    string
		content []byte
	}{
		{"a.xml", []byte("<result>pass</result>")},
		{"b.json", []byte(`{"status":"passed","name":"login test"}`)},
		{"attachment.txt", []byte("stack trace line 1\nstack trace line 2\n")},
	}

	results := make([]map[string]string, len(files))
	for i, f := range files {
		results[i] = map[string]string{
			"file_name":      f.name,
			"content_base64": base64.StdEncoding.EncodeToString(f.content),
		}
	}

	h := newTestAllureHandler(t, projectsDir)
	processed, failed, err := h.sendJSONResults(makeJSONSendResultsReq(t, projectID, results), projectID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(failed) != 0 {
		t.Fatalf("expected no failed files, got %v", failed)
	}
	if len(processed) != len(files) {
		t.Fatalf("expected %d processed files, got %d", len(files), len(processed))
	}

	for _, f := range files {
		got, err := os.ReadFile(filepath.Join(resultsDir, f.name))
		if err != nil {
			t.Fatalf("file %s not written: %v", f.name, err)
		}
		if !bytes.Equal(got, f.content) {
			t.Errorf("file %s content mismatch:\n got  %q\n want %q", f.name, got, f.content)
		}
	}
}

// TestSendJSONResults_InvalidBase64 verifies that a malformed base64 string
// causes sendJSONResults to return a descriptive error.
func TestSendJSONResults_InvalidBase64(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "proj3"
	resultsDir := filepath.Join(projectsDir, projectID, "results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil { //nolint:gosec // G301: test fixtures run in isolated t.TempDir(); relaxed permissions are acceptable
		t.Fatal(err)
	}

	req := makeJSONSendResultsReq(t, projectID, []map[string]string{
		{"file_name": "bad.xml", "content_base64": "not!valid!base64!!!"},
	})

	h := newTestAllureHandler(t, projectsDir)
	_, _, err := h.sendJSONResults(req, projectID)
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

// TestSendJSONResults_DuplicateFileNames verifies that duplicate file_name
// entries in the results array are rejected before any file is written.
func TestSendJSONResults_DuplicateFileNames(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "proj4"
	resultsDir := filepath.Join(projectsDir, projectID, "results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil { //nolint:gosec // G301: test fixtures run in isolated t.TempDir(); relaxed permissions are acceptable
		t.Fatal(err)
	}

	encoded := base64.StdEncoding.EncodeToString([]byte("data"))
	req := makeJSONSendResultsReq(t, projectID, []map[string]string{
		{"file_name": "dup.xml", "content_base64": encoded},
		{"file_name": "dup.xml", "content_base64": encoded},
	})

	h := newTestAllureHandler(t, projectsDir)
	_, _, err := h.sendJSONResults(req, projectID)
	if err == nil {
		t.Fatal("expected error for duplicate file names, got nil")
	}
}

// TestSendJSONResults_MissingContentBase64 verifies that a result entry
// without content_base64 is rejected with a descriptive error.
func TestSendJSONResults_MissingContentBase64(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "proj5"
	resultsDir := filepath.Join(projectsDir, projectID, "results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil { //nolint:gosec // G301: test fixtures run in isolated t.TempDir(); relaxed permissions are acceptable
		t.Fatal(err)
	}

	req := makeJSONSendResultsReq(t, projectID, []map[string]string{
		{"file_name": "missing-content.xml"},
	})

	h := newTestAllureHandler(t, projectsDir)
	_, _, err := h.sendJSONResults(req, projectID)
	if err == nil {
		t.Fatal("expected error for missing content_base64, got nil")
	}
}

// TestSendJSONResults_EmptyResults verifies that an empty results array is rejected.
func TestSendJSONResults_EmptyResults(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "proj6"
	resultsDir := filepath.Join(projectsDir, projectID, "results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil { //nolint:gosec // G301: test fixtures run in isolated t.TempDir(); relaxed permissions are acceptable
		t.Fatal(err)
	}

	req := makeJSONSendResultsReq(t, projectID, []map[string]string{})

	h := newTestAllureHandler(t, projectsDir)
	_, _, err := h.sendJSONResults(req, projectID)
	if err == nil {
		t.Fatal("expected error for empty results array, got nil")
	}
}
