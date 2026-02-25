package storage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// Sentinel errors for LocalStore operations.
var (
	ErrReportIDEmpty   = errors.New("report ID must not be empty")
	ErrReportIDInvalid = errors.New("invalid report ID: must be a positive integer")
	ErrInvalidPath     = errors.New("invalid report path")
	ErrSourceNotDir    = errors.New("source is not a directory")
)

// variableDirs are the only subdirectories that differ between builds.
// Static assets (index.html, app.js, plugins/, etc.) are served from
// reports/latest/ via the overlay HTTP handler, so they are not duplicated
// into each numbered build directory (~90% disk reduction per build).
//
//nolint:gochecknoglobals // read-only slice constant, initialized once at package level
var variableDirs = []string{"data", "widgets", "history"}

// LocalStore implements Store for the local filesystem.
type LocalStore struct {
	cfg *config.Config
}

// NewLocalStore creates a LocalStore backed by cfg.
func NewLocalStore(cfg *config.Config) *LocalStore {
	return &LocalStore{cfg: cfg}
}

// CreateProject creates the project directory with results/ and reports/ subdirs.
// It is idempotent — calling on an existing project is not an error.
func (ls *LocalStore) CreateProject(_ context.Context, projectID string) error {
	base := filepath.Join(ls.cfg.ProjectsDirectory, projectID)
	//nolint:gosec // G301: 0o755 required for allure web server to read project dirs
	if err := os.MkdirAll(filepath.Join(base, "results"), 0o755); err != nil {
		return fmt.Errorf("create results dir: %w", err)
	}
	//nolint:gosec // G301: 0o755 required for allure web server to read project dirs
	if err := os.MkdirAll(filepath.Join(base, "reports"), 0o755); err != nil {
		return fmt.Errorf("create reports dir: %w", err)
	}
	return nil
}

// DeleteProject removes the entire project directory.
// Returns ErrProjectNotFound if the project does not exist.
func (ls *LocalStore) DeleteProject(_ context.Context, projectID string) error {
	base := filepath.Join(ls.cfg.ProjectsDirectory, projectID)
	if _, err := os.Stat(base); os.IsNotExist(err) {
		return fmt.Errorf("project %q: %w", projectID, ErrProjectNotFound)
	}
	if err := os.RemoveAll(base); err != nil {
		return fmt.Errorf("remove project dir %q: %w", base, err)
	}
	return nil
}

// ProjectExists returns true if the project directory exists.
func (ls *LocalStore) ProjectExists(_ context.Context, projectID string) (bool, error) {
	_, err := os.Stat(filepath.Join(ls.cfg.ProjectsDirectory, projectID))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat project dir: %w", err)
}

// ListProjects returns all project directory names sorted alphabetically.
func (ls *LocalStore) ListProjects(_ context.Context) ([]string, error) {
	entries, err := os.ReadDir(ls.cfg.ProjectsDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read projects dir: %w", err)
	}
	var projects []string
	for _, e := range entries {
		if e.IsDir() {
			projects = append(projects, e.Name())
		}
	}
	sort.Strings(projects)
	return projects, nil
}

// WriteResultFile writes r to projectID/results/filename.
func (ls *LocalStore) WriteResultFile(_ context.Context, projectID, filename string, r io.Reader) error {
	resultsDir := filepath.Join(ls.cfg.ProjectsDirectory, projectID, "results")
	//nolint:gosec // G301: 0o755 required for allure web server to read results
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		return fmt.Errorf("create results dir: %w", err)
	}
	dest := filepath.Join(resultsDir, filename)
	f, err := os.Create(dest)
	if err != nil {
		return fmt.Errorf("create result file %q: %w", dest, err)
	}
	defer f.Close()
	if _, err := io.Copy(f, r); err != nil {
		return fmt.Errorf("write result file %q: %w", dest, err)
	}
	return nil
}

// ListResultFiles returns the names of files (not dirs) in projectID/results/.
func (ls *LocalStore) ListResultFiles(_ context.Context, projectID string) ([]string, error) {
	resultsDir := filepath.Join(ls.cfg.ProjectsDirectory, projectID, "results")
	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read results dir: %w", err)
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() {
			files = append(files, e.Name())
		}
	}
	return files, nil
}

// CleanResults removes all contents of projectID/results/.
func (ls *LocalStore) CleanResults(_ context.Context, projectID string) error {
	return removeDirContents(filepath.Join(ls.cfg.ProjectsDirectory, projectID, "results"))
}

// PrepareLocal ensures results/ and reports/ dirs exist and returns the project directory.
func (ls *LocalStore) PrepareLocal(_ context.Context, projectID string) (string, error) {
	base := filepath.Join(ls.cfg.ProjectsDirectory, projectID)
	//nolint:gosec // G301: 0o755 required for allure web server
	if err := os.MkdirAll(filepath.Join(base, "results"), 0o755); err != nil {
		return "", fmt.Errorf("create results dir: %w", err)
	}
	//nolint:gosec // G301: 0o755 required for allure web server
	if err := os.MkdirAll(filepath.Join(base, "reports"), 0o755); err != nil {
		return "", fmt.Errorf("create reports dir: %w", err)
	}
	return base, nil
}

// CleanupLocal is a no-op for LocalStore.
func (ls *LocalStore) CleanupLocal(_ string) error {
	return nil
}

// PublishReport copies only the variable-content subdirs (data, widgets, history)
// from localProjectDir/reports/latest/ to localProjectDir/reports/<buildOrder>/.
// If latest does not exist or is empty, it returns nil without error.
func (ls *LocalStore) PublishReport(_ context.Context, _ string, buildOrder int, localProjectDir string) error {
	latestDir := filepath.Join(localProjectDir, "reports", "latest")

	empty, err := isDirEmpty(latestDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("checking latest report dir: %w", err)
	}
	if empty {
		return nil
	}

	newReportDir := filepath.Join(localProjectDir, "reports", strconv.Itoa(buildOrder))
	//nolint:gosec // G301: 0o755 required for allure web server to serve snapshot reports
	if err := os.MkdirAll(newReportDir, 0o755); err != nil {
		return fmt.Errorf("creating build dir: %w", err)
	}

	for _, dir := range variableDirs {
		src := filepath.Join(latestDir, dir)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			continue
		}
		if err := copyDir(src, filepath.Join(newReportDir, dir)); err != nil {
			return fmt.Errorf("copying %s: %w", dir, err)
		}
	}
	return nil
}

// DeleteReport removes a single numbered report directory for a project.
// reportID must be a numeric string; "latest" and non-numeric IDs are rejected.
func (ls *LocalStore) DeleteReport(_ context.Context, projectID, reportID string) error {
	if reportID == "" {
		return ErrReportIDEmpty
	}
	for _, ch := range reportID {
		if ch < '0' || ch > '9' {
			return fmt.Errorf("report ID %q: %w", reportID, ErrReportIDInvalid)
		}
	}

	reportsDir := filepath.Join(ls.cfg.ProjectsDirectory, projectID, "reports")
	reportDir := filepath.Join(reportsDir, reportID)

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

// PruneReportDirs removes the directories for the given build orders.
func (ls *LocalStore) PruneReportDirs(_ context.Context, projectID string, buildOrders []int) error {
	reportsDir := filepath.Join(ls.cfg.ProjectsDirectory, projectID, "reports")
	for _, bo := range buildOrders {
		dirPath := filepath.Join(reportsDir, strconv.Itoa(bo))
		if err := os.RemoveAll(dirPath); err != nil {
			return fmt.Errorf("removing old report %d: %w", bo, err)
		}
	}
	return nil
}

// KeepHistory copies the latest report's history into results/history (when cfg.KeepHistory=true),
// or removes results/history entirely (when cfg.KeepHistory=false).
func (ls *LocalStore) KeepHistory(_ context.Context, projectID string) error {
	if !ls.cfg.KeepHistory {
		historyDir := filepath.Join(ls.cfg.ProjectsDirectory, projectID, "results", "history")
		if err := os.RemoveAll(historyDir); err != nil {
			return fmt.Errorf("remove history dir %q: %w", historyDir, err)
		}
		return nil
	}

	latestHistoryDir := filepath.Join(ls.cfg.ProjectsDirectory, projectID, "reports", "latest", "history")
	resultsHistoryDir := filepath.Join(ls.cfg.ProjectsDirectory, projectID, "results", "history")

	//nolint:gosec // G301: 0o755 required for allure web server to read history
	if err := os.MkdirAll(resultsHistoryDir, 0o755); err != nil {
		return fmt.Errorf("failed to create results history dir: %w", err)
	}

	if _, err := os.Stat(latestHistoryDir); err == nil {
		return copyDir(latestHistoryDir, resultsHistoryDir)
	}
	return nil
}

// CleanHistory removes all numbered report dirs, clears latest contents,
// clears results/history, and empties executor.json.
func (ls *LocalStore) CleanHistory(_ context.Context, projectID string) error {
	reportsDir := filepath.Join(ls.cfg.ProjectsDirectory, projectID, "reports")
	latestDir := filepath.Join(reportsDir, "latest")
	historyDir := filepath.Join(ls.cfg.ProjectsDirectory, projectID, "results", "history")

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

	executorPath := filepath.Join(ls.cfg.ProjectsDirectory, projectID, "results", "executor.json")
	if _, err := os.Stat(executorPath); err == nil {
		//nolint:gosec // G306: 0o644 required for allure CLI to read executor file
		if err := os.WriteFile(executorPath, []byte(""), 0o644); err != nil {
			return fmt.Errorf("clearing executor.json: %w", err)
		}
	}

	return nil
}

// ReadBuildStats reads cached statistics from a numbered report's widget files.
// Tries widgets/summary.json (Allure 2) first, then widgets/statistic.json (Allure 3).
func (ls *LocalStore) ReadBuildStats(_ context.Context, projectID string, buildOrder int) (BuildStats, error) {
	widgetsDir := filepath.Join(ls.cfg.ProjectsDirectory, projectID, "reports", strconv.Itoa(buildOrder), "widgets")

	// Allure 2: widgets/summary.json
	if data, err := os.ReadFile(filepath.Join(widgetsDir, "summary.json")); err == nil {
		var s struct {
			Statistic struct {
				Passed  int `json:"passed"`
				Failed  int `json:"failed"`
				Broken  int `json:"broken"`
				Skipped int `json:"skipped"`
				Unknown int `json:"unknown"`
				Total   int `json:"total"`
			} `json:"statistic"`
			Time *struct {
				Duration int64 `json:"duration"`
			} `json:"time"`
		}
		if json.Unmarshal(data, &s) == nil {
			stats := BuildStats{
				Passed:  s.Statistic.Passed,
				Failed:  s.Statistic.Failed,
				Broken:  s.Statistic.Broken,
				Skipped: s.Statistic.Skipped,
				Unknown: s.Statistic.Unknown,
				Total:   s.Statistic.Total,
			}
			if s.Time != nil {
				stats.DurationMs = s.Time.Duration
			}
			return stats, nil
		}
	}

	// Allure 3: widgets/statistic.json
	if data, err := os.ReadFile(filepath.Join(widgetsDir, "statistic.json")); err == nil {
		var stat struct {
			Passed  int `json:"passed"`
			Failed  int `json:"failed"`
			Broken  int `json:"broken"`
			Skipped int `json:"skipped"`
			Unknown int `json:"unknown"`
			Total   int `json:"total"`
		}
		if json.Unmarshal(data, &stat) == nil && stat.Total > 0 {
			return BuildStats{
				Passed:  stat.Passed,
				Failed:  stat.Failed,
				Broken:  stat.Broken,
				Skipped: stat.Skipped,
				Unknown: stat.Unknown,
				Total:   stat.Total,
			}, nil
		}
	}

	return BuildStats{}, fmt.Errorf("build %d: %w", buildOrder, ErrStatsNotFound)
}

// ReadFile reads the file at projectID/relPath and returns its contents.
func (ls *LocalStore) ReadFile(_ context.Context, projectID, relPath string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(ls.cfg.ProjectsDirectory, projectID, filepath.FromSlash(relPath)))
	if err != nil {
		return nil, fmt.Errorf("read file %q: %w", relPath, err)
	}
	return data, nil
}

// ReadDir returns the entries of the directory at projectID/relPath.
func (ls *LocalStore) ReadDir(_ context.Context, projectID, relPath string) ([]DirEntry, error) {
	dir := filepath.Join(ls.cfg.ProjectsDirectory, projectID, filepath.FromSlash(relPath))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read dir %q: %w", dir, err)
	}
	result := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		result = append(result, DirEntry{
			Name:    e.Name(),
			Size:    info.Size(),
			ModTime: info.ModTime().UnixNano(),
			IsDir:   e.IsDir(),
		})
	}
	return result, nil
}

// OpenReportFile opens projectID/reports/reportID/filePath and returns a ReadCloser
// and the detected MIME content type.
func (ls *LocalStore) OpenReportFile(_ context.Context, projectID, reportID, filePath string) (io.ReadCloser, string, error) {
	path := filepath.Join(ls.cfg.ProjectsDirectory, projectID, "reports", reportID, filePath)
	f, err := os.Open(path)
	if err != nil {
		return nil, "", fmt.Errorf("open report file %q: %w", path, err)
	}
	ct := mime.TypeByExtension(filepath.Ext(filePath))
	if ct == "" {
		ct = "application/octet-stream"
	}
	return f, ct, nil
}

// ListReportBuilds returns the numeric build order directories sorted ascending.
func (ls *LocalStore) ListReportBuilds(_ context.Context, projectID string) ([]int, error) {
	reportsDir := filepath.Join(ls.cfg.ProjectsDirectory, projectID, "reports")
	entries, err := os.ReadDir(reportsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read reports dir: %w", err)
	}
	var builds []int
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		n, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		builds = append(builds, n)
	}
	sort.Ints(builds)
	return builds, nil
}

// LatestReportExists returns true if the reports/latest directory exists.
func (ls *LocalStore) LatestReportExists(_ context.Context, projectID string) (bool, error) {
	_, err := os.Stat(filepath.Join(ls.cfg.ProjectsDirectory, projectID, "reports", "latest"))
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, fmt.Errorf("stat latest dir: %w", err)
}

// ResultsDirHash returns a hash of the results directory contents for change detection.
// Files named executor.json and allurereport.config.json are excluded to avoid
// spurious retriggers from files written by GenerateReport.
func (ls *LocalStore) ResultsDirHash(_ context.Context, projectID string) (string, error) {
	resultsDir := filepath.Join(ls.cfg.ProjectsDirectory, projectID, "results")
	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		return "", fmt.Errorf("read project results dir %q: %w", resultsDir, err)
	}

	type fileEntry struct {
		name    string
		size    int64
		modTime int64
	}
	var files []fileEntry
	for _, e := range entries {
		if e.IsDir() || e.Name() == "executor.json" || e.Name() == "allurereport.config.json" {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileEntry{
			name:    e.Name(),
			size:    info.Size(),
			modTime: info.ModTime().UnixNano(),
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].name < files[j].name
	})

	h := sha256.New()
	for _, f := range files {
		fmt.Fprintf(h, "%s:%d:%d\n", f.name, f.size, f.modTime)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// --- Helper functions ---

// removeDirContents removes all files and subdirectories inside the given directory.
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

// copyDir recursively copies a directory tree from src to dst.
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

// isDirEmpty returns (true, nil) when the directory exists but is empty.
// Returns (false, os.ErrNotExist-wrapped) when the directory does not exist.
func isDirEmpty(name string) (bool, error) {
	f, err := os.Open(name)
	if err != nil {
		return false, fmt.Errorf("open dir %q: %w", name, err)
	}
	defer f.Close()

	_, err = f.Readdirnames(1)
	if errors.Is(err, io.EOF) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("list dir %q: %w", name, err)
	}
	return false, nil
}
