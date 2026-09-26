package runner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestStoreReport_MissingOptionalDir verifies that StoreReport gracefully skips
// variable dirs that are absent from latest/ and copies those that do exist.
func TestStoreReport_MissingOptionalDir(t *testing.T) {
	dir := t.ArtifactDir()
	projectID := "myproject"
	buildNumber := 2

	// Only data/ present — no widgets/ or history/.
	latestDir := filepath.Join(dir, projectID, "reports", "latest")
	mustWriteFile(t, filepath.Join(latestDir, "data", "results.json"), `{}`)

	a := newTestAllure(t, dir)
	if err := a.StoreReport(context.Background(), projectID, buildNumber); err != nil {
		t.Fatalf("StoreReport with partial dirs: %v", err)
	}

	buildDir := filepath.Join(dir, projectID, "reports", "2")
	if _, err := os.Stat(filepath.Join(buildDir, "data")); os.IsNotExist(err) {
		t.Error("data/ should be copied into build dir")
	}
	for _, d := range []string{"widgets", "history"} {
		if _, err := os.Stat(filepath.Join(buildDir, d)); !os.IsNotExist(err) {
			t.Errorf("%s/ should not exist (was absent from latest/)", d)
		}
	}
}

// TestStoreAndPruneBuild_PublishReportErrorPropagates verifies that a critical
// error is returned from storeAndPruneBuild instead of being swallowed. The
// build row is now reserved up front by the entry point (ReserveBuild), so
// PublishReport is storeAndPruneBuild's leading hard-fail.
func TestStoreAndPruneBuild_PublishReportErrorPropagates(t *testing.T) {
	dir := t.ArtifactDir()
	projectID := int64(10)
	slug := "err-proj"

	cfg := &config.Config{ProjectsPath: dir}
	st := &testutil.MockStorage{
		PublishReportFn: func(_ context.Context, _ string, _ int, _ string, _ storage.ProgressFn) error {
			return errors.New("boom") // any non-nil error
		},
	}
	mocks := testutil.New()
	a := NewAllure(AllureDeps{
		Config:     cfg,
		Store:      st,
		BuildStore: mocks.Builds,
		Locker:     mocks.Locker,
		Logger:     zap.NewNop(),
	})

	err := a.storeAndPruneBuild(context.Background(), projectID, slug, slug, "", dir, 1, store.CIMetadata{}, nil)
	if err == nil {
		t.Fatal("expected error from storeAndPruneBuild when PublishReport fails, got nil")
	}
	if !strings.Contains(err.Error(), "publish report") {
		t.Errorf("expected error containing %q, got: %v", "publish report", err)
	}
}
