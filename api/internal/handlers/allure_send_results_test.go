package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
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
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
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
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
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
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
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
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
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
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
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
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	req := makeJSONSendResultsReq(t, projectID, []map[string]string{})

	h := newTestAllureHandler(t, projectsDir)
	_, _, err := h.sendJSONResults(req, projectID)
	if err == nil {
		t.Fatal("expected error for empty results array, got nil")
	}
}

// ---------------------------------------------------------------------------
// tar.gz upload test helpers
// ---------------------------------------------------------------------------

// tarEntry allows custom tar.Header fields for security tests (symlinks, etc.).
type tarEntry struct {
	Header  tar.Header
	Content []byte
}

// makeTarGz builds a tar.gz archive in memory from a map of filename → content.
func makeTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	entries := make([]tarEntry, 0, len(files))
	// Sort keys for deterministic output.
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		entries = append(entries, tarEntry{
			Header: tar.Header{
				Name:     name,
				Size:     int64(len(files[name])),
				Mode:     0o644,
				Typeflag: tar.TypeReg,
			},
			Content: files[name],
		})
	}
	return makeTarGzWithOpts(t, entries)
}

// makeTarGzWithOpts builds a tar.gz archive with custom tar.Header fields.
func makeTarGzWithOpts(t *testing.T, entries []tarEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	for i := range entries {
		if err := tw.WriteHeader(&entries[i].Header); err != nil {
			t.Fatalf("tar write header %q: %v", entries[i].Header.Name, err)
		}
		if len(entries[i].Content) > 0 {
			if _, err := tw.Write(entries[i].Content); err != nil {
				t.Fatalf("tar write content %q: %v", entries[i].Header.Name, err)
			}
		}
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

// makeTarGzRequest creates a POST request with Content-Type: application/gzip.
func makeTarGzRequest(t *testing.T, projectID string, body []byte) *http.Request {
	t.Helper()
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
	req.Header.Set("Content-Type", "application/gzip")
	return req
}

// setupTarGzTest creates a temporary project directory and returns handler + projectID.
func setupTarGzTest(t *testing.T) (*AllureHandler, string, string) {
	t.Helper()
	projectsDir := t.TempDir()
	projectID := "targz-proj"
	resultsDir := filepath.Join(projectsDir, projectID, "results")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return newTestAllureHandler(t, projectsDir), projectID, resultsDir
}

// ---------------------------------------------------------------------------
// tar.gz upload tests — happy path
// ---------------------------------------------------------------------------

// TestSendTarGzResults_SingleFile verifies a single file is extracted correctly.
func TestSendTarGzResults_SingleFile(t *testing.T) {
	h, projectID, resultsDir := setupTarGzTest(t)

	wantContent := []byte("<allure-result><status>passed</status></allure-result>")
	archive := makeTarGz(t, map[string][]byte{
		"test-result.xml": wantContent,
	})
	req := makeTarGzRequest(t, projectID, archive)

	processed, failed, err := h.sendTarGzResults(req, projectID)
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

// TestSendTarGzResults_MultipleFiles verifies all files are extracted correctly.
func TestSendTarGzResults_MultipleFiles(t *testing.T) {
	h, projectID, resultsDir := setupTarGzTest(t)

	files := map[string][]byte{
		"a.xml":          []byte("<result>pass</result>"),
		"b.json":         []byte(`{"status":"passed"}`),
		"attachment.txt": []byte("stack trace\n"),
	}
	archive := makeTarGz(t, files)
	req := makeTarGzRequest(t, projectID, archive)

	processed, failed, err := h.sendTarGzResults(req, projectID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(failed) != 0 {
		t.Fatalf("expected no failed files, got %v", failed)
	}
	if len(processed) != len(files) {
		t.Fatalf("expected %d processed files, got %d", len(files), len(processed))
	}

	for name, want := range files {
		got, err := os.ReadFile(filepath.Join(resultsDir, name))
		if err != nil {
			t.Fatalf("file %s not written: %v", name, err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("file %s content mismatch:\n got  %q\n want %q", name, got, want)
		}
	}
}

// TestSendTarGzResults_ContentTypeVariants verifies all accepted Content-Type values.
func TestSendTarGzResults_ContentTypeVariants(t *testing.T) {
	contentTypes := []string{
		"application/gzip",
		"application/x-gzip",
		"application/x-tar+gzip",
	}
	for _, ct := range contentTypes {
		t.Run(ct, func(t *testing.T) {
			h, projectID, _ := setupTarGzTest(t)
			archive := makeTarGz(t, map[string][]byte{
				"result.xml": []byte("<result/>"),
			})
			req := makeTarGzRequest(t, projectID, archive)
			req.Header.Set("Content-Type", ct)

			processed, _, err := h.sendTarGzResults(req, projectID)
			if err != nil {
				t.Fatalf("Content-Type %q rejected: %v", ct, err)
			}
			if len(processed) != 1 {
				t.Fatalf("expected 1 processed file, got %d", len(processed))
			}
		})
	}
}

// ---------------------------------------------------------------------------
// tar.gz upload tests — validation
// ---------------------------------------------------------------------------

// TestSendTarGzResults_EmptyArchive verifies that an archive with zero entries is rejected.
func TestSendTarGzResults_EmptyArchive(t *testing.T) {
	h, projectID, _ := setupTarGzTest(t)
	archive := makeTarGz(t, map[string][]byte{})
	req := makeTarGzRequest(t, projectID, archive)

	_, _, err := h.sendTarGzResults(req, projectID)
	if !errors.Is(err, ErrArchiveEmpty) {
		t.Fatalf("expected ErrArchiveEmpty, got %v", err)
	}
}

// TestSendTarGzResults_DuplicateFileNames verifies duplicate entries are rejected.
func TestSendTarGzResults_DuplicateFileNames(t *testing.T) {
	h, projectID, _ := setupTarGzTest(t)
	entries := []tarEntry{
		{Header: tar.Header{Name: "dup.xml", Size: 4, Mode: 0o644, Typeflag: tar.TypeReg}, Content: []byte("data")},
		{Header: tar.Header{Name: "dup.xml", Size: 4, Mode: 0o644, Typeflag: tar.TypeReg}, Content: []byte("data")},
	}
	archive := makeTarGzWithOpts(t, entries)
	req := makeTarGzRequest(t, projectID, archive)

	_, _, err := h.sendTarGzResults(req, projectID)
	if !errors.Is(err, ErrArchiveDuplicateFile) {
		t.Fatalf("expected ErrArchiveDuplicateFile, got %v", err)
	}
}

// TestSendTarGzResults_InvalidGzip verifies that non-gzip data is rejected.
func TestSendTarGzResults_InvalidGzip(t *testing.T) {
	h, projectID, _ := setupTarGzTest(t)
	req := makeTarGzRequest(t, projectID, []byte("this is not gzip"))

	_, _, err := h.sendTarGzResults(req, projectID)
	if err == nil {
		t.Fatal("expected error for invalid gzip, got nil")
	}
}

// ---------------------------------------------------------------------------
// tar.gz upload tests — security
// ---------------------------------------------------------------------------

// TestSendTarGzResults_NestedDirectory rejects entries with nested paths.
func TestSendTarGzResults_NestedDirectory(t *testing.T) {
	h, projectID, _ := setupTarGzTest(t)
	entries := []tarEntry{
		{Header: tar.Header{Name: "subdir/file.xml", Size: 4, Mode: 0o644, Typeflag: tar.TypeReg}, Content: []byte("data")},
	}
	archive := makeTarGzWithOpts(t, entries)
	req := makeTarGzRequest(t, projectID, archive)

	_, _, err := h.sendTarGzResults(req, projectID)
	if !errors.Is(err, ErrArchiveNestedPath) {
		t.Fatalf("expected ErrArchiveNestedPath, got %v", err)
	}
}

// TestSendTarGzResults_PathTraversal rejects path traversal attempts.
func TestSendTarGzResults_PathTraversal(t *testing.T) {
	h, projectID, _ := setupTarGzTest(t)
	entries := []tarEntry{
		{Header: tar.Header{Name: "../../etc/passwd", Size: 4, Mode: 0o644, Typeflag: tar.TypeReg}, Content: []byte("root")},
	}
	archive := makeTarGzWithOpts(t, entries)
	req := makeTarGzRequest(t, projectID, archive)

	_, _, err := h.sendTarGzResults(req, projectID)
	if !errors.Is(err, ErrArchiveNestedPath) {
		t.Fatalf("expected ErrArchiveNestedPath, got %v", err)
	}
}

// TestSendTarGzResults_Symlink skips symlink entries (archive with only symlinks is empty).
func TestSendTarGzResults_Symlink(t *testing.T) {
	h, projectID, _ := setupTarGzTest(t)
	entries := []tarEntry{
		{Header: tar.Header{Name: "evil-link", Typeflag: tar.TypeSymlink, Linkname: "/etc/passwd"}, Content: nil},
	}
	archive := makeTarGzWithOpts(t, entries)
	req := makeTarGzRequest(t, projectID, archive)

	_, _, err := h.sendTarGzResults(req, projectID)
	if !errors.Is(err, ErrArchiveEmpty) {
		t.Fatalf("expected ErrArchiveEmpty, got %v", err)
	}
}

// TestSendTarGzResults_HardLink skips hard link entries (archive with only links is empty).
func TestSendTarGzResults_HardLink(t *testing.T) {
	h, projectID, _ := setupTarGzTest(t)
	entries := []tarEntry{
		{Header: tar.Header{Name: "evil-link", Typeflag: tar.TypeLink, Linkname: "target.xml"}, Content: nil},
	}
	archive := makeTarGzWithOpts(t, entries)
	req := makeTarGzRequest(t, projectID, archive)

	_, _, err := h.sendTarGzResults(req, projectID)
	if !errors.Is(err, ErrArchiveEmpty) {
		t.Fatalf("expected ErrArchiveEmpty, got %v", err)
	}
}

// TestSendTarGzResults_Directory skips directory entries (archive with only dirs is empty).
func TestSendTarGzResults_Directory(t *testing.T) {
	h, projectID, _ := setupTarGzTest(t)
	entries := []tarEntry{
		{Header: tar.Header{Name: "subdir/", Typeflag: tar.TypeDir, Mode: 0o755}, Content: nil},
	}
	archive := makeTarGzWithOpts(t, entries)
	req := makeTarGzRequest(t, projectID, archive)

	_, _, err := h.sendTarGzResults(req, projectID)
	if !errors.Is(err, ErrArchiveEmpty) {
		t.Fatalf("expected ErrArchiveEmpty, got %v", err)
	}
}

// TestSendTarGzResults_DecompressionBomb verifies the decompressed size limit.
func TestSendTarGzResults_DecompressionBomb(t *testing.T) {
	// Temporarily lower the limit so the test is fast.
	orig := maxDecompressedBytes
	maxDecompressedBytes = 1024 // 1 KB
	t.Cleanup(func() { maxDecompressedBytes = orig })

	h, projectID, _ := setupTarGzTest(t)
	// Create a file larger than the limit.
	bigContent := make([]byte, 2048)
	for i := range bigContent {
		bigContent[i] = 'A'
	}
	archive := makeTarGz(t, map[string][]byte{
		"big.bin": bigContent,
	})
	req := makeTarGzRequest(t, projectID, archive)

	_, _, err := h.sendTarGzResults(req, projectID)
	if !errors.Is(err, ErrArchiveDecompBomb) {
		t.Fatalf("expected ErrArchiveDecompBomb, got %v", err)
	}
}

// TestSendTarGzResults_FileCountExceeded verifies the file count limit.
func TestSendTarGzResults_FileCountExceeded(t *testing.T) {
	// Temporarily lower the limit so the test is fast.
	orig := maxArchiveFileCount
	maxArchiveFileCount = 3
	t.Cleanup(func() { maxArchiveFileCount = orig })

	h, projectID, _ := setupTarGzTest(t)
	files := map[string][]byte{
		"a.xml": []byte("a"),
		"b.xml": []byte("b"),
		"c.xml": []byte("c"),
		"d.xml": []byte("d"),
	}
	archive := makeTarGz(t, files)
	req := makeTarGzRequest(t, projectID, archive)

	_, _, err := h.sendTarGzResults(req, projectID)
	if !errors.Is(err, ErrArchiveTooManyFiles) {
		t.Fatalf("expected ErrArchiveTooManyFiles, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// tar.gz upload tests — edge cases
// ---------------------------------------------------------------------------

// TestSendTarGzResults_SpecialCharsInFilename verifies filenames with spaces/parens are preserved.
func TestSendTarGzResults_SpecialCharsInFilename(t *testing.T) {
	h, projectID, resultsDir := setupTarGzTest(t)

	wantName := "my file (1).json"
	wantContent := []byte(`{"ok":true}`)
	archive := makeTarGz(t, map[string][]byte{
		wantName: wantContent,
	})
	req := makeTarGzRequest(t, projectID, archive)

	processed, _, err := h.sendTarGzResults(req, projectID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(processed) != 1 || processed[0] != wantName {
		t.Fatalf("expected processed=[%s], got %v", wantName, processed)
	}

	got, err := os.ReadFile(filepath.Join(resultsDir, wantName))
	if err != nil {
		t.Fatalf("file not written: %v", err)
	}
	if !bytes.Equal(got, wantContent) {
		t.Errorf("content mismatch:\n got  %q\n want %q", got, wantContent)
	}
}

// TestSendTarGzResults_ProcessedFilesAreSorted verifies deterministic output order.
func TestSendTarGzResults_ProcessedFilesAreSorted(t *testing.T) {
	h, projectID, _ := setupTarGzTest(t)

	files := map[string][]byte{
		"zebra.xml":  []byte("z"),
		"alpha.xml":  []byte("a"),
		"middle.xml": []byte("m"),
	}
	archive := makeTarGz(t, files)
	req := makeTarGzRequest(t, projectID, archive)

	processed, _, err := h.sendTarGzResults(req, projectID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !sort.StringsAreSorted(processed) {
		t.Fatalf("processed files not sorted: %v", processed)
	}

	expected := []string{"alpha.xml", "middle.xml", "zebra.xml"}
	if len(processed) != len(expected) {
		t.Fatalf("expected %d files, got %d", len(expected), len(processed))
	}
	for i, name := range expected {
		if processed[i] != name {
			t.Errorf("processed[%d] = %q, want %q", i, processed[i], name)
		}
	}
}

// TestSendResults_ForceProjectCreation_RegistersInDB verifies that when
// force_project_creation=true is used for a non-existent project, the project
// is registered in the projectStore (DB) in addition to being created on the
// filesystem. This guards against River job failures caused by FK violations.
func TestSendResults_ForceProjectCreation_RegistersInDB(t *testing.T) {
	projectsDir := t.TempDir()
	projectID := "force-create-proj"

	cfg := &config.Config{ProjectsPath: projectsDir, MaxUploadSizeMB: 10}
	st := storage.NewLocalStore(cfg)
	logger := zap.NewNop()
	mocks := testutil.New()
	r := runner.NewAllure(cfg, st, mocks.MemBuilds, mocks.Locker, nil, nil, logger)
	h := NewAllureHandler(cfg, r, nil,
		mocks.Projects, mocks.MemBuilds, mocks.KnownIssues, nil, mocks.Search, st)

	encoded := base64.StdEncoding.EncodeToString([]byte("<result/>"))
	req := makeJSONSendResultsReq(t, projectID, []map[string]string{
		{"file_name": "result.xml", "content_base64": encoded},
	})
	q := req.URL.Query()
	q.Set("force_project_creation", "true")
	req.URL.RawQuery = q.Encode()

	w := httptest.NewRecorder()
	h.SendResults(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	exists, err := mocks.Projects.ProjectExists(context.Background(), projectID)
	if err != nil {
		t.Fatalf("unexpected error checking project in DB: %v", err)
	}
	if !exists {
		t.Error("project was not registered in projectStore after force_project_creation=true")
	}
}

// TestParseResultsBody_TarGzRouting verifies parseResultsBody routes tar.gz content types correctly.
func TestParseResultsBody_TarGzRouting(t *testing.T) {
	h, projectID, _ := setupTarGzTest(t)

	archive := makeTarGz(t, map[string][]byte{
		"routed.xml": []byte("<ok/>"),
	})

	contentTypes := []string{
		"application/gzip",
		"application/x-gzip",
		"application/x-tar+gzip",
	}
	for _, ct := range contentTypes {
		t.Run(ct, func(t *testing.T) {
			req := makeTarGzRequest(t, projectID, archive)
			req.Header.Set("Content-Type", ct)

			processed, _, err := h.parseResultsBody(req, projectID)
			if err != nil {
				t.Fatalf("parseResultsBody rejected Content-Type %q: %v", ct, err)
			}
			if len(processed) != 1 || processed[0] != "routed.xml" {
				t.Fatalf("expected processed=[routed.xml], got %v", processed)
			}
		})
	}
}
