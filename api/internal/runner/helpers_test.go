package runner

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// newTestAllure constructs an Allure instance pointed at projectsDir using mock stores.
// Pass t.TempDir() when no specific project directory is needed.
func newTestAllure(t *testing.T, projectsDir string) *Allure {
	t.Helper()
	cfg := &config.Config{ProjectsPath: projectsDir}
	st := storage.NewLocalStore(cfg)
	mocks := testutil.New()
	return NewAllure(AllureDeps{
		Config:     cfg,
		Store:      st,
		BuildStore: mocks.Builds,
		Locker:     mocks.Locker,
		Logger:     zap.NewNop(),
	})
}

// mustWriteFile creates parent dirs and writes content to path.
func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// summaryJSON returns a minimal widgets/summary.json payload.
func summaryJSON(total, passed, failed, broken, skipped, unknown int) string {
	type statistic struct {
		Total   int `json:"total"`
		Passed  int `json:"passed"`
		Failed  int `json:"failed"`
		Broken  int `json:"broken"`
		Skipped int `json:"skipped"`
		Unknown int `json:"unknown"`
	}
	data, _ := json.Marshal(map[string]any{
		"statistic": statistic{
			Total: total, Passed: passed, Failed: failed,
			Broken: broken, Skipped: skipped, Unknown: unknown,
		},
	})
	return string(data)
}
