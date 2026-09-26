package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestPlaywrightHandler_UploadReport(t *testing.T) {
	t.Parallel()
	const html = "<html><body>Playwright Report</body></html>"
	png := []byte("\x89PNG\r\n\x1a\n")
	index := makeTarGz(t, map[string][]byte{"index.html": []byte(html)})
	rows := []struct {
		name      string
		seed      bool   // register project "pw" first
		query     string // "{parent}" is replaced by a seeded parent project's ID
		archive   []byte
		want      int
		wantFiles map[string][]byte // under the project's playwright-reports dir
		check     func(t *testing.T, mocks *testutil.MockStores, parentID int64, flagged []int)
	}{
		// Files are staged in latest/ with their subdirectories preserved.
		{name: "upload", seed: true, archive: makeTarGz(t, map[string][]byte{"index.html": []byte(html), "data/screen.png": png}), want: http.StatusOK,
			wantFiles: map[string][]byte{"latest/index.html": []byte(html), "latest/data/screen.png": png}},
		{name: "archive without index.html", seed: true, archive: makeTarGz(t, map[string][]byte{"data/test.png": png}), want: http.StatusBadRequest},
		{name: "unknown project", archive: index, want: http.StatusNotFound},
		{name: "not gzip", seed: true, archive: []byte("this is not gzip data"), want: http.StatusBadRequest},
		{name: "path traversal entry", seed: true, archive: makeTarGz(t, map[string][]byte{"../../etc/passwd": []byte("root"), "index.html": []byte(html)}),
			want: http.StatusBadRequest},
		{name: "auto-create child project", query: "force_project_creation=true&parent_id={parent}", archive: index, want: http.StatusOK,
			check: func(t *testing.T, mocks *testutil.MockStores, parentID int64, _ []int) {
				if p, err := mocks.Projects.GetProjectBySlugAny(context.Background(), "pw"); err != nil || p.ParentID == nil || *p.ParentID != parentID {
					t.Errorf("project pw = %+v (err %v), want registered under parent %d", p, err, parentID)
				}
			}},
		// With build_number the report goes straight to that build's dir,
		// which is flagged has_playwright_report; latest/ is never created.
		{name: "into build 5", seed: true, query: "build_number=5", archive: index, want: http.StatusOK,
			wantFiles: map[string][]byte{"5/index.html": []byte(html)},
			check: func(t *testing.T, _ *testutil.MockStores, _ int64, flagged []int) {
				if len(flagged) != 1 || flagged[0] != 5 {
					t.Errorf("builds flagged with a Playwright report = %v, want [5]", flagged)
				}
			}},
		{name: "unknown build", seed: true, query: "build_number=99", archive: index, want: http.StatusNotFound},
		{name: "invalid build number", seed: true, query: "build_number=abc", archive: index, want: http.StatusBadRequest},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			cfg := &config.Config{ProjectsPath: dir, MaxUploadSizeMB: 100}
			mocks := testutil.New()
			var flagged []int
			mocks.Builds.GetBuildByNumberFn = func(_ context.Context, _ int64, n int) (store.Build, error) {
				if n != 5 {
					return store.Build{}, store.ErrBuildNotFound
				}
				return store.Build{BuildNumber: n}, nil
			}
			mocks.Builds.SetHasPlaywrightReportFn = func(_ context.Context, _ int64, n int, v bool) error {
				if v {
					flagged = append(flagged, n)
				}
				return nil
			}
			parent, _ := mocks.Projects.CreateProject(context.Background(), "pw-parent")
			if tc.seed {
				_, _ = mocks.Projects.CreateProject(context.Background(), "pw")
			}
			query := strings.ReplaceAll(tc.query, "{parent}", strconv.FormatInt(parent.ID, 10))
			req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/pw/playwright?"+query, bytes.NewReader(tc.archive))
			req.SetPathValue("project_id", "pw")
			req.Header.Set("Content-Type", "application/gzip")
			rr := httptest.NewRecorder()
			NewPlaywrightHandler(storage.NewLocalStore(cfg), mocks.Projects, mocks.Builds, nil, cfg, zap.NewNop()).UploadReport(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.want == http.StatusOK {
				var resp struct {
					Data struct {
						Status string `json:"status"`
					} `json:"data"`
				}
				if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil || resp.Data.Status != "uploaded" {
					t.Errorf("body = %s, want status uploaded", rr.Body.String())
				}
			}
			reports := filepath.Join(dir, "pw", "playwright-reports")
			for rel, want := range tc.wantFiles {
				if got, err := os.ReadFile(filepath.Join(reports, rel)); err != nil || !bytes.Equal(got, want) {
					t.Errorf("%s = %q (err %v), want %q", rel, got, err, want)
				}
			}
			if tc.wantFiles != nil && tc.wantFiles["latest/index.html"] == nil {
				if _, err := os.Stat(filepath.Join(reports, "latest")); !os.IsNotExist(err) {
					t.Errorf("latest/ exists (err %v), want it untouched", err)
				}
			}
			if tc.check != nil {
				tc.check(t, mocks, parent.ID, flagged)
			}
		})
	}
}

// extractPlaywrightArchive spools each entry to disk and streams it to
// storage, so a large entry is never buffered whole; the first failed write
// is returned and cancels the rest.
func TestPlaywrightHandler_ExtractArchive(t *testing.T) {
	t.Parallel()
	large := make([]byte, 10<<20)
	for i := range large {
		large[i] = byte(i % 251)
	}
	injected := errors.New("injected store error")
	rows := []struct {
		name   string
		files  map[string][]byte
		failOn string // entry whose write fails
	}{
		{"every entry reaches storage", map[string][]byte{
			"index.html": []byte("<html/>"), "data/trace.zip": large, "data/screenshot.png": []byte("\x89PNG\r\n"),
			"data/video.webm": []byte("WEBM"), "assets/app.js": []byte("console.log('hi')"),
		}, ""},
		{"failed write", map[string][]byte{
			"index.html": []byte("<html/>"), "data/fail-me.bin": []byte("bad"), "data/ok1.bin": []byte("ok1"), "data/ok2.bin": []byte("ok2"),
		}, "data/fail-me.bin"},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var mu sync.Mutex
			got := map[string][]byte{}
			mock := &testutil.MockStorage{WritePlaywrightFileFn: func(ctx context.Context, _, subPath string, r io.Reader) error {
				if tc.failOn != "" && strings.HasSuffix(subPath, tc.failOn) {
					return injected
				}
				data, err := io.ReadAll(r)
				mu.Lock()
				got[subPath] = data
				mu.Unlock()
				if err != nil {
					return err
				}
				return ctx.Err()
			}}
			h := &PlaywrightHandler{store: mock, cfg: &config.Config{MaxUploadSizeMB: 200, UploadWriteConcurrency: 4}, logger: zap.NewNop()}
			req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(makeTarGz(t, tc.files)))
			req.Header.Set("Content-Type", "application/gzip")
			err := h.extractPlaywrightArchive(req, "proj-key", "latest")
			if tc.failOn != "" {
				if !errors.Is(err, injected) {
					t.Errorf("err = %v, want the injected store error", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("extractPlaywrightArchive: %v", err)
			}
			if len(got) != len(tc.files) {
				t.Fatalf("uploads = %d, want %d", len(got), len(tc.files))
			}
			for name, want := range tc.files {
				if !bytes.Equal(got["latest/"+name], want) {
					t.Errorf("latest/%s: got %d bytes, want %d", name, len(got["latest/"+name]), len(want))
				}
			}
		})
	}
}
