package main

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// writeTestFile creates parent dirs and writes content to the given path.
func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// childProjectStore holds one child project, "api-happy", whose numeric
// StorageKey differs from its slug, so a handler that addresses storage by the
// URL slug misses its files (fixes 404f3ee, cf6e4f3).
func childProjectStore(t *testing.T) (store.ProjectStorer, string) {
	t.Helper()
	ctx := context.Background()
	ps := testutil.NewMemProjectStore()
	parent, err := ps.CreateProject(ctx, "parent")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	child, err := ps.CreateProjectWithParent(ctx, "api-happy", parent.ID)
	if err != nil {
		t.Fatalf("CreateProjectWithParent: %v", err)
	}
	return ps, child.StorageKey
}

// TestOverlayHandler serves Allure report files from disk. Numbered build dirs
// hold only variable content, so a file missing there falls back to
// reports/latest/; the project segment is resolved to its StorageKey first.
// Each file's content is its own path, so the body names the file served.
func TestOverlayHandler(t *testing.T) {
	ps, key := childProjectStore(t)
	tests := []struct {
		name     string
		file     string // written under the projects dir; {key} is the child's StorageKey
		url      string
		wantCode int
	}{
		{"build dir file served directly", "myproject/reports/3/data/test.json", "/myproject/reports/3/data/test.json", http.StatusOK},
		{"missing build file falls back to latest", "myproject/reports/latest/index.html", "/myproject/reports/3/index.html", http.StatusOK},
		{"non-report path never falls back", "myproject/reports/latest/index.html", "/myproject/emailable-report-render/index.html", http.StatusNotFound},
		{"absent from build dir and latest", "", "/myproject/reports/5/index.html", http.StatusNotFound},
		{"slug resolves to storage key", "{key}/reports/1/data/test.json", "/api-happy/reports/1/data/test.json", http.StatusOK},
		{"slug with latest fallback", "{key}/reports/latest/index.html", "/api-happy/reports/2/index.html", http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if tc.file != "" {
				path := strings.ReplaceAll(tc.file, "{key}", key)
				writeTestFile(t, filepath.Join(dir, filepath.FromSlash(path)), tc.file)
			}
			rr := httptest.NewRecorder()
			newOverlayHandler(dir, ps).ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.url, nil))
			if rr.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d; body: %s", rr.Code, tc.wantCode, rr.Body)
			}
			if tc.wantCode == http.StatusOK && rr.Body.String() != tc.file {
				t.Errorf("served %q, want %q", rr.Body.String(), tc.file)
			}
		})
	}
}

// TestStorageReportHandlers pins that the Playwright and S3 report handlers
// read under the project's StorageKey rather than the URL slug (fixes cf6e4f3,
// 404f3ee), and that an unknown project never reaches storage.
func TestStorageReportHandlers(t *testing.T) {
	ps, key := childProjectStore(t)
	var reads []string
	st := &storage.MockStore{
		ReadPlaywrightFileFn: func(_ context.Context, projectID, subPath string) (io.ReadCloser, string, error) {
			reads = append(reads, projectID+":"+subPath)
			return io.NopCloser(strings.NewReader("playwright")), "text/html", nil
		},
		OpenReportFileFn: func(_ context.Context, projectID, reportID, filePath string) (io.ReadCloser, string, error) {
			reads = append(reads, projectID+":"+reportID+"/"+filePath)
			return io.NopCloser(strings.NewReader("allure")), "text/html", nil
		},
	}
	playwrightReq := func(slug string) *http.Request {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+slug+"/playwright-reports/1/index.html", nil)
		req.SetPathValue("projectID", slug)
		req.SetPathValue("reportID", "1")
		req.SetPathValue("rest", "index.html")
		return req
	}

	tests := []struct {
		name      string
		handler   http.Handler
		req       *http.Request
		wantCode  int
		wantBody  string
		wantReads []string
	}{
		{"playwright slug reads by storage key", newPlaywrightReportHandler(st, ps), playwrightReq("api-happy"),
			http.StatusOK, "playwright", []string{key + ":1/index.html"}},
		{"playwright unknown slug is 404 without a read", newPlaywrightReportHandler(st, ps), playwrightReq("no-such-slug"),
			http.StatusNotFound, "", nil},
		{"s3 slug reads by storage key", newS3ReportHandler(st, ps), httptest.NewRequest(http.MethodGet, "/api-happy/reports/1/index.html", nil),
			http.StatusOK, "allure", []string{key + ":1/index.html"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reads = nil
			rr := httptest.NewRecorder()
			tc.handler.ServeHTTP(rr, tc.req)
			if rr.Code != tc.wantCode {
				t.Fatalf("status = %d, want %d; body: %s", rr.Code, tc.wantCode, rr.Body)
			}
			if tc.wantBody != "" && rr.Body.String() != tc.wantBody {
				t.Errorf("body = %q, want %q", rr.Body.String(), tc.wantBody)
			}
			if !slices.Equal(reads, tc.wantReads) {
				t.Errorf("storage reads = %v, want %v", reads, tc.wantReads)
			}
		})
	}
}
