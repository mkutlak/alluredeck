package runner

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// Sentinel errors for report path validation.
//
// Sentinel errors for report path validation.
var (
	ErrReportIDEmpty   = errors.New("report ID must not be empty")
	ErrReportIDInvalid = errors.New("invalid report ID: must be a positive integer")
	ErrInvalidPath     = errors.New("invalid report path")
	ErrReportNotFound  = errors.New("report not found")
	ErrSourceNotDir    = errors.New("source is not a directory")
)

// FileSystem provides native file system operations equivalent to the bash scripts
type FileSystem struct {
	cfg *config.Config
}

// NewFileSystem creates a new FileSystem utility
func NewFileSystem(cfg *config.Config) *FileSystem {
	return &FileSystem{cfg: cfg}
}

// CleanResults implements cleanAllureResults.sh
func (fs *FileSystem) CleanResults(projectID string) error {
	resultsDir := filepath.Join(fs.cfg.ProjectsDirectory, projectID, "results")
	return removeDirContents(resultsDir)
}

// CleanHistory implements cleanAllureHistory.sh
func (fs *FileSystem) CleanHistory(projectID string) error {
	reportsDir := filepath.Join(fs.cfg.ProjectsDirectory, projectID, "reports")
	latestDir := filepath.Join(reportsDir, "latest")
	historyDir := filepath.Join(fs.cfg.ProjectsDirectory, projectID, "results", "history")

	if err := removeDirContents(latestDir); err != nil {
		return err
	}

	entries, err := os.ReadDir(reportsDir)
	if err == nil {
		for _, e := range entries {
			if e.IsDir() && e.Name() != "latest" && e.Name() != "0" {
				if err := os.RemoveAll(filepath.Join(reportsDir, e.Name())); err != nil {
					return fmt.Errorf("removing report dir %s: %w", e.Name(), err)
				}
			}
		}
	}

	if err := removeDirContents(historyDir); err != nil {
		return err
	}

	executorPath := filepath.Join(fs.cfg.ProjectsDirectory, projectID, "results", "executor.json")
	if _, err := os.Stat(executorPath); err == nil {
		if err := os.WriteFile(executorPath, []byte(""), 0o644); err != nil { //nolint:gosec // G306: 0o644 required for allure CLI to read executor file
			return fmt.Errorf("clearing executor.json: %w", err)
		}
	}

	return nil
}

// KeepHistory implements keepAllureHistory.sh
func (fs *FileSystem) KeepHistory(projectID string) error {
	if !fs.cfg.KeepHistory {
		historyDir := filepath.Join(fs.cfg.ProjectsDirectory, projectID, "results", "history")
		if err := os.RemoveAll(historyDir); err != nil {
			return fmt.Errorf("remove history dir %q: %w", historyDir, err)
		}
		return nil
	}

	latestHistoryDir := filepath.Join(fs.cfg.ProjectsDirectory, projectID, "reports", "latest", "history")
	resultsHistoryDir := filepath.Join(fs.cfg.ProjectsDirectory, projectID, "results", "history")

	if err := os.MkdirAll(resultsHistoryDir, 0o755); err != nil { //nolint:gosec // G301: 0o755 required for allure web server to read history
		return fmt.Errorf("failed to create results history dir: %w", err)
	}

	if _, err := os.Stat(latestHistoryDir); err == nil {
		return copyDir(latestHistoryDir, resultsHistoryDir)
	}
	return nil
}

// DeleteReport removes a single numbered report directory for a project.
// reportID must be a numeric string (e.g. "3"); "latest" and non-numeric IDs are rejected.
func (fs *FileSystem) DeleteReport(projectID, reportID string) error {
	if reportID == "" {
		return ErrReportIDEmpty
	}
	// Only allow numeric report IDs — never "latest" or path traversal attempts.
	for _, ch := range reportID {
		if ch < '0' || ch > '9' {
			return fmt.Errorf("report ID %q: %w", reportID, ErrReportIDInvalid)
		}
	}

	reportsDir := filepath.Join(fs.cfg.ProjectsDirectory, projectID, "reports")
	reportDir := filepath.Join(reportsDir, reportID)

	// Guard against path traversal: resolved path must stay inside reportsDir.
	absReports, err := filepath.Abs(reportsDir)
	if err != nil {
		return fmt.Errorf("resolving reports dir: %w", err)
	}
	absReport, err := filepath.Abs(reportDir)
	if err != nil {
		return fmt.Errorf("resolving report dir: %w", err)
	}
	if absReport == absReports || !strings.HasPrefix(absReport, absReports+string(filepath.Separator)) {
		return ErrInvalidPath
	}

	if _, err := os.Stat(reportDir); os.IsNotExist(err) {
		return fmt.Errorf("report %q: %w", reportID, ErrReportNotFound)
	}
	if err := os.RemoveAll(reportDir); err != nil {
		return fmt.Errorf("remove report dir %q: %w", reportDir, err)
	}
	return nil
}

// removeDirContents removes all files and subdirectories inside the given directory
func removeDirContents(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open source dir %q: %w", dir, err)
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return fmt.Errorf("list entries in %q: %w", dir, err)
	}
	for _, name := range names {
		if err := os.RemoveAll(filepath.Join(dir, name)); err != nil { //nolint:gosec // G703: name from ReadDir listing, not user input
			return fmt.Errorf("remove entry %q: %w", filepath.Join(dir, name), err)
		}
	}
	return nil
}

// copyDir recursively copies a directory tree
func copyDir(src, dst string) error {
	src = filepath.Clean(src)
	dst = filepath.Clean(dst)

	si, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source %q: %w", src, err)
	}
	if !si.IsDir() {
		return ErrSourceNotDir
	}

	if err := os.MkdirAll(dst, si.Mode()); err != nil {
		return fmt.Errorf("create destination dir %q: %w", dst, err)
	}

	entries, err := os.ReadDir(src)
	if err != nil {
		return fmt.Errorf("read directory %q: %w", src, err)
	}

	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file from src to dst, preserving permissions and timestamps.
func copyFile(src, dst string) error {
	si, err := os.Stat(src)
	if err != nil {
		return fmt.Errorf("stat source file %q: %w", src, err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source file %q: %w", src, err)
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("create destination file %q: %w", dst, err)
	}
	defer out.Close()

	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %q to %q: %w", src, dst, err)
	}

	if err := os.Chmod(dst, si.Mode()); err != nil {
		return fmt.Errorf("set permissions on %q: %w", dst, err)
	}

	if err := os.Chtimes(dst, si.ModTime(), si.ModTime()); err != nil {
		return fmt.Errorf("set timestamps on %q: %w", dst, err)
	}
	return nil
}
