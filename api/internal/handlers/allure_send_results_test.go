package handlers

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// resultsRequest builds a POST /projects/{projectID}/results request.
func resultsRequest(projectID, contentType string, body []byte) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/"+projectID+"/results", bytes.NewReader(body))
	req.SetPathValue("project_id", projectID)
	req.Header.Set("Content-Type", contentType)
	return req
}

// jsonResults encodes {"results":[...]} the way CI clients send it.
func jsonResults(t *testing.T, results ...map[string]string) []byte {
	t.Helper()
	body, err := json.Marshal(map[string]any{"results": results})
	if err != nil {
		t.Fatal(err)
	}
	return body
}

// wantResultFiles asserts processed lists exactly the files in order and that
// each was written byte-for-byte under the batch directory.
func wantResultFiles(t *testing.T, batchDir string, processed []string, files [][2]string) {
	t.Helper()
	names := make([]string, len(files))
	for i, f := range files {
		names[i] = f[0]
		if got, err := os.ReadFile(filepath.Join(batchDir, f[0])); err != nil || string(got) != f[1] {
			t.Errorf("%s = %q (err %v), want %q", f[0], got, err, f[1])
		}
	}
	if !slices.Equal(processed, names) {
		t.Errorf("processed = %v, want %v", processed, names)
	}
}

// TestSendJSONResults covers the base64 JSON body. The success row is the
// regression guard for the streaming decode: every file lands on disk
// byte-for-byte.
func TestSendJSONResults(t *testing.T) {
	b64 := func(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
	files := [][2]string{
		{"a.xml", "<result>pass</result>"},
		{"b.json", `{"status":"passed","name":"login test"}`},
		{"attachment.txt", "stack trace line 1\nstack trace line 2\n"},
	}
	tests := []struct {
		name    string
		results []map[string]string
		wantErr bool
	}{
		{name: "decodes every file", results: []map[string]string{
			{"file_name": files[0][0], "content_base64": b64(files[0][1])},
			{"file_name": files[1][0], "content_base64": b64(files[1][1])},
			{"file_name": files[2][0], "content_base64": b64(files[2][1])},
		}},
		{name: "invalid base64", results: []map[string]string{{"file_name": "bad.xml", "content_base64": "not!valid!base64!!!"}}, wantErr: true},
		{name: "duplicate file names", results: []map[string]string{
			{"file_name": "dup.xml", "content_base64": b64("data")},
			{"file_name": "dup.xml", "content_base64": b64("data")},
		}, wantErr: true},
		{name: "missing content_base64", results: []map[string]string{{"file_name": "missing-content.xml"}}, wantErr: true},
		{name: "empty results", results: []map[string]string{}, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			projectsDir := t.TempDir()
			h, _ := newTestResultUploadHandler(t, projectsDir)
			processed, failed, err := h.sendJSONResults(resultsRequest("proj", "application/json", jsonResults(t, tc.results...)), "proj", "testbatch")
			if tc.wantErr {
				if err == nil {
					t.Fatal("err = nil, want an error")
				}
				return
			}
			if err != nil || len(failed) != 0 {
				t.Fatalf("err = %v, failed = %v", err, failed)
			}
			wantResultFiles(t, filepath.Join(projectsDir, "proj", "results", "testbatch"), processed, files)
		})
	}
}

// tarEntry allows custom tar.Header fields for security tests (symlinks, etc.).
type tarEntry struct {
	Header  tar.Header
	Content []byte
}

// makeTarGz builds a tar.gz archive in memory from a map of filename → content.
func makeTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic archive order
	entries := make([]tarEntry, 0, len(files))
	for _, name := range names {
		entries = append(entries, tarEntry{
			Header:  tar.Header{Name: name, Size: int64(len(files[name])), Mode: 0o644, Typeflag: tar.TypeReg},
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

// TestSendTarGzResults covers archive extraction through the handler, including
// the handler's own decompression and file-count limits.
func TestSendTarGzResults(t *testing.T) {
	reg := func(name string) tarEntry {
		return tarEntry{Header: tar.Header{Name: name, Typeflag: tar.TypeReg, Mode: 0o644, Size: 4}, Content: []byte("data")}
	}
	link := func(name string, typ byte, target string) tarEntry {
		return tarEntry{Header: tar.Header{Name: name, Typeflag: typ, Linkname: target}}
	}
	many := make([][2]string, 200)
	for i := range many {
		many[i] = [2]string{fmt.Sprintf("result-%03d.json", i), fmt.Sprintf(`{"i":%d}`, i)}
	}
	tests := []struct {
		name     string
		files    [][2]string // regular entries, in their sorted (processed) order
		entries  []tarEntry  // hand-built entries; used when files is nil
		raw      string      // a raw, non-gzip body
		maxBytes int64
		maxFiles int
		wantErr  error
	}{
		// Names with spaces and parens survive, and processed is sorted.
		{name: "writes entries sorted", files: [][2]string{{"alpha.xml", "a"}, {"my file (1).json", `{"ok":true}`}, {"zebra.xml", "z"}}},
		// Enough files for the parallel writes to race under -race; processed
		// must still come back sorted.
		{name: "many files in parallel", files: many},
		{name: "empty archive", entries: []tarEntry{}, wantErr: ErrArchiveEmpty},
		{name: "duplicate names", entries: []tarEntry{reg("dup.xml"), reg("dup.xml")}, wantErr: ErrArchiveDuplicateFile},
		{name: "invalid gzip", raw: "this is not gzip", wantErr: gzip.ErrHeader},
		{name: "nested path", entries: []tarEntry{reg("subdir/file.xml")}, wantErr: ErrArchiveNestedPath},
		{name: "path traversal", entries: []tarEntry{reg("../../etc/passwd")}, wantErr: ErrArchiveNestedPath},
		// Links and directories are skipped, so an archive of only those is empty.
		{name: "symlink only", entries: []tarEntry{link("evil-link", tar.TypeSymlink, "/etc/passwd")}, wantErr: ErrArchiveEmpty},
		{name: "hard link only", entries: []tarEntry{link("evil-link", tar.TypeLink, "target.xml")}, wantErr: ErrArchiveEmpty},
		{name: "directory only", entries: []tarEntry{{Header: tar.Header{Name: "subdir/", Typeflag: tar.TypeDir, Mode: 0o755}}}, wantErr: ErrArchiveEmpty},
		{name: "decompression bomb", files: [][2]string{{"big.bin", strings.Repeat("A", 2048)}}, maxBytes: 1024, wantErr: ErrArchiveDecompBomb},
		{name: "too many files", files: [][2]string{{"a.xml", "a"}, {"b.xml", "b"}, {"c.xml", "c"}, {"d.xml", "d"}}, maxFiles: 3, wantErr: ErrArchiveTooManyFiles},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if tc.maxBytes > 0 {
				orig := maxDecompressedBytes
				maxDecompressedBytes = tc.maxBytes
				t.Cleanup(func() { maxDecompressedBytes = orig })
			}
			if tc.maxFiles > 0 {
				orig := maxArchiveFileCount
				maxArchiveFileCount = tc.maxFiles
				t.Cleanup(func() { maxArchiveFileCount = orig })
			}
			body := []byte(tc.raw)
			switch {
			case tc.files != nil:
				m := make(map[string][]byte, len(tc.files))
				for _, f := range tc.files {
					m[f[0]] = []byte(f[1])
				}
				body = makeTarGz(t, m)
			case tc.entries != nil:
				body = makeTarGzWithOpts(t, tc.entries)
			}
			projectsDir := t.TempDir()
			h, _ := newTestResultUploadHandler(t, projectsDir)
			processed, failed, err := h.sendTarGzResults(resultsRequest("proj", "application/gzip", body), "proj", "testbatch")
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err = %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil || len(failed) != 0 {
				t.Fatalf("err = %v, failed = %v", err, failed)
			}
			wantResultFiles(t, filepath.Join(projectsDir, "proj", "results", "testbatch"), processed, tc.files)
		})
	}
}

// TestSendTarGzResults_StorageWriteFailure pins the atomicity of the parallel
// commit phase: one failed write fails the batch with processed and failed
// both nil, so the caller never schedules a parsing job for a partial batch.
func TestSendTarGzResults_StorageWriteFailure(t *testing.T) {
	cfg := &config.Config{ProjectsPath: t.TempDir(), MaxUploadSizeMB: 100, UploadWriteConcurrency: 8}
	mockStore := &storage.MockStore{
		WriteResultFileFn: func(_ context.Context, _, _, filename string, r io.Reader) error {
			_, _ = io.Copy(io.Discard, r)
			if filename == "boom.json" {
				return errors.New("simulated MinIO PUT failure")
			}
			return nil
		},
	}
	mocks := testutil.New()
	r := runner.NewAllure(runner.AllureDeps{Config: cfg, Store: mockStore, BuildStore: mocks.MemBuilds, Locker: mocks.Locker, Logger: zap.NewNop()})
	h := NewResultUploadHandler(mockStore, mocks.Projects, runner.NewMemJobManager(nil, 0, zap.NewNop()), r, cfg, zap.NewNop())

	archive := makeTarGz(t, map[string][]byte{"a.json": []byte("ok"), "boom.json": []byte("nope"), "c.json": []byte("ok"), "d.json": []byte("ok")})
	processed, failed, err := h.sendTarGzResults(resultsRequest("proj", "application/gzip", archive), "proj", "batch-fail")
	if err == nil || processed != nil || failed != nil {
		t.Fatalf("err, processed, failed = %v, %v, %v; want an error and both lists nil", err, processed, failed)
	}
}

// TestParseResultsBody_TarGzRouting pins the Content-Type values routed to the
// tar.gz parser.
func TestParseResultsBody_TarGzRouting(t *testing.T) {
	projectsDir := t.TempDir()
	h, _ := newTestResultUploadHandler(t, projectsDir)
	archive := makeTarGz(t, map[string][]byte{"routed.xml": []byte("<ok/>")})
	for _, ct := range []string{"application/gzip", "application/x-gzip", "application/x-tar+gzip"} {
		t.Run(ct, func(t *testing.T) {
			processed, _, err := h.parseResultsBody(resultsRequest("proj", ct, archive), "proj", "testbatch")
			if err != nil || !slices.Equal(processed, []string{"routed.xml"}) {
				t.Fatalf("processed, err = %v, %v; want [routed.xml], nil", processed, err)
			}
		})
	}
}

// TestSendResults_ForceProjectCreation covers uploads to a project that does
// not exist yet: it is registered in the DB (so downstream jobs do not fail on
// the FK), and a missing parent slug is created as a top-level project instead
// of failing with "parent_id not found" — the failure mode after a full reset.
func TestSendResults_ForceProjectCreation(t *testing.T) {
	tests := []struct {
		name   string
		parent string
	}{
		{name: "registers project in db"},
		{name: "auto-creates missing parent slug", parent: "acme-api-tests-api"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			h, mocks := newTestResultUploadHandler(t, t.TempDir())
			body := jsonResults(t, map[string]string{"file_name": "result.xml", "content_base64": base64.StdEncoding.EncodeToString([]byte("<result/>"))})
			req := resultsRequest("api-users", "application/json", body)
			req.URL.RawQuery = "force_project_creation=true&parent_id=" + tc.parent
			rr := httptest.NewRecorder()
			h.SendResults(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
			}
			child, err := mocks.Projects.GetProjectBySlugAny(ctx, "api-users")
			if err != nil {
				t.Fatalf("project not registered: %v", err)
			}
			if tc.parent == "" {
				return
			}
			parent, err := mocks.Projects.GetProjectBySlugAny(ctx, tc.parent)
			if err != nil {
				t.Fatalf("parent not auto-created: %v", err)
			}
			if parent.ParentID != nil {
				t.Errorf("parent ParentID = %v, want top-level", *parent.ParentID)
			}
			if child.ParentID == nil || *child.ParentID != parent.ID {
				t.Errorf("child ParentID = %v, want %d", child.ParentID, parent.ID)
			}
		})
	}
}
