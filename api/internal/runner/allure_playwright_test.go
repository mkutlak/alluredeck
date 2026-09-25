package runner

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestCopyPlaywrightReport covers the hybrid pipeline's Playwright step: an
// uploaded playwright-reports/latest/ is copied to the numbered build dir,
// flagged on the build, its data/ files are registered as attachments (skipping
// .dat step metadata and unknown extensions, matching extensions
// case-insensitively), and latest/ is cleaned. Without a report it is a no-op.
func TestCopyPlaywrightReport(t *testing.T) {
	const slug, buildNumber = "pw", 3
	tests := []struct {
		name     string
		files    map[string]string // under latest/; nil = no latest/ dir at all
		wantCopy bool
		wantAtts []store.TestAttachment
	}{
		{name: "no latest dir is a no-op"},
		{name: "empty latest dir is a no-op", files: map[string]string{}},
		{name: "report without data/ has no attachments", files: map[string]string{"index.html": "<html></html>"}, wantCopy: true},
		{
			name: "data files become attachments",
			files: map[string]string{
				"index.html": "<html>pw report</html>", "data/abc123.png": "\x89PNG", "data/shot.PNG": "png",
				"data/trace.zip": "zip", "data/abc.dat": "step metadata", "data/blob.bin": "unknown",
			},
			wantCopy: true,
			wantAtts: []store.TestAttachment{
				{Name: "abc123.png", Source: "data/abc123.png", MimeType: "image/png"},
				{Name: "shot.PNG", Source: "data/shot.PNG", MimeType: "image/png"},
				{Name: "trace.zip", Source: "data/trace.zip", MimeType: "application/zip"},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			latestDir := filepath.Join(dir, slug, "playwright-reports", "latest")
			if tc.files != nil {
				if err := os.MkdirAll(latestDir, 0o755); err != nil {
					t.Fatal(err)
				}
			}
			for name, body := range tc.files {
				mustWriteFile(t, filepath.Join(latestDir, name), body)
			}

			mocks := testutil.New()
			var flagged []int
			mocks.Builds.SetHasPlaywrightReportFn = func(_ context.Context, _ int64, bo int, value bool) error {
				if value {
					flagged = append(flagged, bo)
				}
				return nil
			}
			mocks.TestResults.GetBuildIDFn = func(context.Context, int64, int) (int64, error) { return 99, nil }
			var atts []store.TestAttachment
			mocks.Attachments.InsertBuildAttachmentsFn = func(_ context.Context, _, _ int64, a []store.TestAttachment) error {
				atts = a
				return nil
			}
			cfg := &config.Config{ProjectsPath: dir}
			a := NewAllure(AllureDeps{
				Config: cfg, Store: storage.NewLocalStore(cfg), BuildStore: mocks.Builds, Locker: mocks.Locker,
				TestResultStore: mocks.TestResults, AttachmentStore: mocks.Attachments, Logger: zap.NewNop(),
			})

			a.copyPlaywrightReport(context.Background(), 1, slug, slug, buildNumber)

			if got := slices.Equal(flagged, []int{buildNumber}); got != tc.wantCopy {
				t.Errorf("SetHasPlaywrightReport(true) calls = %v, want copied=%v", flagged, tc.wantCopy)
			}
			buildDir := filepath.Join(dir, slug, "playwright-reports", "3")
			if !tc.wantCopy {
				if _, err := os.Stat(buildDir); !os.IsNotExist(err) {
					t.Errorf("playwright-reports/3/ must not exist without a report, stat err = %v", err)
				}
			}
			for name := range tc.files {
				if _, err := os.Stat(filepath.Join(buildDir, name)); err != nil {
					t.Errorf("%s not copied to build dir: %v", name, err)
				}
			}
			if entries, _ := os.ReadDir(latestDir); tc.wantCopy && len(entries) != 0 {
				t.Errorf("latest/ should be empty after copy, got %d entries", len(entries))
			}
			slices.SortFunc(atts, func(x, y store.TestAttachment) int { return strings.Compare(x.Source, y.Source) })
			if !reflect.DeepEqual(atts, tc.wantAtts) {
				t.Errorf("InsertBuildAttachments = %+v, want %+v", atts, tc.wantAtts)
			}
		})
	}
}
