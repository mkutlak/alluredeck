package runner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

func TestFileSystem(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "allure-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	cfg := &config.Config{
		ProjectsPath: tmpDir,
		KeepHistory:  true,
	}
	fs := NewFileSystem(cfg)

	projectID := "test-project"
	projectDir := filepath.Join(tmpDir, projectID)
	resultsDir := filepath.Join(projectDir, "results")
	reportsDir := filepath.Join(projectDir, "reports")
	latestDir := filepath.Join(reportsDir, "latest")

	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(latestDir, 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("CleanResults", func(t *testing.T) {
		testFile := filepath.Join(resultsDir, "test-result.json")
		if err := os.WriteFile(testFile, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := fs.CleanResults(projectID); err != nil {
			t.Errorf("CleanResults failed: %v", err)
		}

		if _, err := os.Stat(testFile); !os.IsNotExist(err) {
			t.Errorf("Expected test file to be deleted")
		}
	})

	t.Run("KeepHistory", func(t *testing.T) {
		historyDir := filepath.Join(latestDir, "history")
		if err := os.MkdirAll(historyDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(historyDir, "history.json"), []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}

		if err := fs.KeepHistory(projectID); err != nil {
			t.Errorf("KeepHistory failed: %v", err)
		}

		resultsHistoryFile := filepath.Join(resultsDir, "history", "history.json")
		if _, err := os.Stat(resultsHistoryFile); err != nil {
			t.Errorf("Expected history file to be copied to results: %v", err)
		}
	})

	t.Run("CleanHistory", func(t *testing.T) {
		// Setup more complex structure
		oldReportDir := filepath.Join(reportsDir, "old-report")
		if err := os.MkdirAll(oldReportDir, 0o755); err != nil {
			t.Fatal(err)
		}

		historyDir := filepath.Join(resultsDir, "history")
		if err := os.MkdirAll(historyDir, 0o755); err != nil {
			t.Fatal(err)
		}

		if err := fs.CleanHistory(projectID); err != nil {
			t.Errorf("CleanHistory failed: %v", err)
		}

		if _, err := os.Stat(oldReportDir); !os.IsNotExist(err) {
			t.Errorf("Expected old report dir to be deleted")
		}
		if _, err := os.Stat(historyDir); err == nil {
			// historyDir itself might exist but should be empty
			entries, _ := os.ReadDir(historyDir)
			if len(entries) > 0 {
				t.Errorf("Expected history dir to be empty")
			}
		}
	})
}
