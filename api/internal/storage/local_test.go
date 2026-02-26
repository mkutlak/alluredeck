package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// newTestConfig returns a Config pointing at a temp directory.
func newTestConfig(t *testing.T) (*config.Config, string) {
	t.Helper()
	dir := t.TempDir()
	return &config.Config{
		ProjectsDirectory: dir,
		KeepHistory:       false,
	}, dir
}

// makeLocalStore returns a LocalStore backed by a temp directory.
func makeLocalStore(t *testing.T) (*LocalStore, string) {
	t.Helper()
	cfg, dir := newTestConfig(t)
	return NewLocalStore(cfg), dir
}

// mkdirAll creates dirs inside the temp root, ignoring errors in tests.
func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil { //nolint:gosec // G301: test helper
		t.Fatalf("mkdirAll %q: %v", path, err)
	}
}

// writeFile writes content to path, creating parent dirs as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil { //nolint:gosec // G306: test helper
		t.Fatalf("writeFile %q: %v", path, err)
	}
}

// --- CreateProject ---

func TestLocalStore_CreateProject(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj1"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	// results and reports dirs must exist
	for _, sub := range []string{"results", "reports"} {
		p := filepath.Join(root, "proj1", sub)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected dir %q to exist: %v", p, err)
		}
	}
	// idempotent — calling again must not error
	if err := ls.CreateProject(ctx, "proj1"); err != nil {
		t.Fatalf("CreateProject idempotent: %v", err)
	}
}

// --- ProjectExists ---

func TestLocalStore_ProjectExists(t *testing.T) {
	ls, _ := makeLocalStore(t)
	ctx := context.Background()

	exists, err := ls.ProjectExists(ctx, "ghost")
	if err != nil {
		t.Fatalf("ProjectExists: %v", err)
	}
	if exists {
		t.Fatal("expected false for non-existent project")
	}

	if err := ls.CreateProject(ctx, "real"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	exists, err = ls.ProjectExists(ctx, "real")
	if err != nil {
		t.Fatalf("ProjectExists after create: %v", err)
	}
	if !exists {
		t.Fatal("expected true after creating project")
	}
}

// --- ListProjects ---

func TestLocalStore_ListProjects(t *testing.T) {
	ls, _ := makeLocalStore(t)
	ctx := context.Background()

	list, err := ls.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects empty: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %v", list)
	}

	for _, id := range []string{"alpha", "beta", "gamma"} {
		if err := ls.CreateProject(ctx, id); err != nil {
			t.Fatalf("CreateProject %s: %v", id, err)
		}
	}
	list, err = ls.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	want := []string{"alpha", "beta", "gamma"}
	sort.Strings(list)
	if len(list) != len(want) {
		t.Fatalf("expected %v, got %v", want, list)
	}
	for i, w := range want {
		if list[i] != w {
			t.Errorf("index %d: want %q, got %q", i, w, list[i])
		}
	}
}

// --- WriteResultFile ---

func TestLocalStore_WriteResultFile(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	content := []byte("test-result-data")
	if err := ls.WriteResultFile(ctx, "proj", "result.xml", bytes.NewReader(content)); err != nil {
		t.Fatalf("WriteResultFile: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "proj", "results", "result.xml"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: want %q, got %q", content, got)
	}
}

// --- ListResultFiles ---

func TestLocalStore_ListResultFiles(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Initially empty
	files, err := ls.ListResultFiles(ctx, "proj")
	if err != nil {
		t.Fatalf("ListResultFiles empty: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected empty, got %v", files)
	}

	// Write two files
	resultsDir := filepath.Join(root, "proj", "results")
	writeFile(t, filepath.Join(resultsDir, "a.xml"), "a")
	writeFile(t, filepath.Join(resultsDir, "b.json"), "b")

	files, err = ls.ListResultFiles(ctx, "proj")
	if err != nil {
		t.Fatalf("ListResultFiles: %v", err)
	}
	sort.Strings(files)
	want := []string{"a.xml", "b.json"}
	if len(files) != len(want) {
		t.Fatalf("want %v, got %v", want, files)
	}
	for i, w := range want {
		if files[i] != w {
			t.Errorf("index %d: want %q, got %q", i, w, files[i])
		}
	}
}

// --- CleanResults ---

func TestLocalStore_CleanResults(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	resultsDir := filepath.Join(root, "proj", "results")
	writeFile(t, filepath.Join(resultsDir, "file.xml"), "data")

	if err := ls.CleanResults(ctx, "proj"); err != nil {
		t.Fatalf("CleanResults: %v", err)
	}

	entries, err := os.ReadDir(resultsDir)
	if err != nil {
		t.Fatalf("ReadDir after clean: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected empty results dir, got %d entries", len(entries))
	}

	// Non-existent results dir must not error
	ls2, _ := makeLocalStore(t)
	if err := ls2.CleanResults(ctx, "nonexistent"); err != nil {
		t.Fatalf("CleanResults non-existent: %v", err)
	}
}

// --- PrepareLocal ---

func TestLocalStore_PrepareLocal(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	localDir, err := ls.PrepareLocal(ctx, "proj")
	if err != nil {
		t.Fatalf("PrepareLocal: %v", err)
	}
	want := filepath.Join(root, "proj")
	if localDir != want {
		t.Errorf("want %q, got %q", want, localDir)
	}
	// results and reports dirs created
	for _, sub := range []string{"results", "reports"} {
		p := filepath.Join(localDir, sub)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected dir %q: %v", p, err)
		}
	}
}

// --- CleanupLocal ---

func TestLocalStore_CleanupLocal(t *testing.T) {
	ls, _ := makeLocalStore(t)
	if err := ls.CleanupLocal("/any/path"); err != nil {
		t.Fatalf("CleanupLocal: %v", err)
	}
}

// --- PublishReport ---

func TestLocalStore_PublishReport(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	latestDir := filepath.Join(root, "proj", "reports", "latest")
	// Populate only variable dirs in latest
	for _, d := range []string{"data", "widgets", "history"} {
		writeFile(t, filepath.Join(latestDir, d, "file.json"), `{"x":1}`)
	}
	// Also put a non-variable file/dir in latest — must NOT be copied
	writeFile(t, filepath.Join(latestDir, "index.html"), "<html/>")
	mkdirAll(t, filepath.Join(latestDir, "plugins", "myplug"))

	if err := ls.PublishReport(ctx, "proj", 1, filepath.Join(root, "proj")); err != nil {
		t.Fatalf("PublishReport: %v", err)
	}

	buildDir := filepath.Join(root, "proj", "reports", "1")
	// variable dirs copied
	for _, d := range []string{"data", "widgets", "history"} {
		p := filepath.Join(buildDir, d, "file.json")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %q in build dir: %v", p, err)
		}
	}
	// index.html must NOT be in build dir
	if _, err := os.Stat(filepath.Join(buildDir, "index.html")); !os.IsNotExist(err) {
		t.Error("index.html should not be copied to build dir")
	}
	// plugins dir must NOT be in build dir
	if _, err := os.Stat(filepath.Join(buildDir, "plugins")); !os.IsNotExist(err) {
		t.Error("plugins dir should not be copied to build dir")
	}
}

func TestLocalStore_PublishReport_EmptyLatest(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Create empty latest dir
	mkdirAll(t, filepath.Join(root, "proj", "reports", "latest"))

	if err := ls.PublishReport(ctx, "proj", 1, filepath.Join(root, "proj")); err != nil {
		t.Fatalf("PublishReport empty latest: %v", err)
	}
	// build dir should not be created
	if _, err := os.Stat(filepath.Join(root, "proj", "reports", "1")); !os.IsNotExist(err) {
		t.Error("build dir should not be created for empty latest")
	}
}

func TestLocalStore_PublishReport_MissingLatest(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// latest dir does not exist
	if err := ls.PublishReport(ctx, "proj", 1, filepath.Join(root, "proj")); err != nil {
		t.Fatalf("PublishReport missing latest: %v", err)
	}
}

// --- DeleteReport ---

func TestLocalStore_DeleteReport(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Create a numbered report dir
	reportDir := filepath.Join(root, "proj", "reports", "3")
	mkdirAll(t, reportDir)

	if err := ls.DeleteReport(ctx, "proj", "3"); err != nil {
		t.Fatalf("DeleteReport: %v", err)
	}
	if _, err := os.Stat(reportDir); !os.IsNotExist(err) {
		t.Error("report dir should be removed")
	}

	// ErrReportNotFound for missing
	err := ls.DeleteReport(ctx, "proj", "99")
	if !errors.Is(err, ErrReportNotFound) {
		t.Errorf("expected ErrReportNotFound, got %v", err)
	}

	// ErrReportIDInvalid for "latest"
	err = ls.DeleteReport(ctx, "proj", "latest")
	if !errors.Is(err, ErrReportIDInvalid) {
		t.Errorf("expected ErrReportIDInvalid for 'latest', got %v", err)
	}

	// ErrReportIDEmpty for ""
	err = ls.DeleteReport(ctx, "proj", "")
	if !errors.Is(err, ErrReportIDEmpty) {
		t.Errorf("expected ErrReportIDEmpty, got %v", err)
	}
}

// --- PruneReportDirs ---

func TestLocalStore_PruneReportDirs(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	reportsDir := filepath.Join(root, "proj", "reports")
	for _, n := range []int{1, 2, 3} {
		mkdirAll(t, filepath.Join(reportsDir, strconv.Itoa(n)))
	}

	if err := ls.PruneReportDirs(ctx, "proj", []int{1, 3}); err != nil {
		t.Fatalf("PruneReportDirs: %v", err)
	}

	for _, n := range []int{1, 3} {
		if _, err := os.Stat(filepath.Join(reportsDir, strconv.Itoa(n))); !os.IsNotExist(err) {
			t.Errorf("build dir %d should be removed", n)
		}
	}
	// dir 2 must still exist
	if _, err := os.Stat(filepath.Join(reportsDir, "2")); err != nil {
		t.Error("build dir 2 should still exist")
	}
}

// --- KeepHistory ---

func TestLocalStore_KeepHistory_Enabled(t *testing.T) {
	cfg, root := newTestConfig(t)
	cfg.KeepHistory = true
	ls := NewLocalStore(cfg)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Populate latest history
	latestHistoryDir := filepath.Join(root, "proj", "reports", "latest", "history")
	writeFile(t, filepath.Join(latestHistoryDir, "trend.json"), `{"x":1}`)

	if err := ls.KeepHistory(ctx, "proj"); err != nil {
		t.Fatalf("KeepHistory: %v", err)
	}

	dest := filepath.Join(root, "proj", "results", "history", "trend.json")
	if _, err := os.Stat(dest); err != nil {
		t.Errorf("expected history file copied to results: %v", err)
	}
}

func TestLocalStore_KeepHistory_Disabled(t *testing.T) {
	cfg, root := newTestConfig(t)
	cfg.KeepHistory = false
	ls := NewLocalStore(cfg)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Create results/history dir
	historyDir := filepath.Join(root, "proj", "results", "history")
	mkdirAll(t, historyDir)
	writeFile(t, filepath.Join(historyDir, "old.json"), `{}`)

	if err := ls.KeepHistory(ctx, "proj"); err != nil {
		t.Fatalf("KeepHistory disabled: %v", err)
	}

	// history dir must be removed entirely
	if _, err := os.Stat(historyDir); !os.IsNotExist(err) {
		t.Error("history dir should be removed when KeepHistory=false")
	}
}

// --- CleanHistory ---

func TestLocalStore_CleanHistory(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	reportsDir := filepath.Join(root, "proj", "reports")
	latestDir := filepath.Join(reportsDir, "latest")
	historyDir := filepath.Join(root, "proj", "results", "history")

	// Populate latest with some files
	writeFile(t, filepath.Join(latestDir, "index.html"), "<html/>")

	// Numbered report dirs: 1 and 2 should be removed; 0 should be kept
	for _, n := range []string{"1", "2"} {
		mkdirAll(t, filepath.Join(reportsDir, n))
	}
	mkdirAll(t, filepath.Join(reportsDir, "0"))

	// Populate results/history
	mkdirAll(t, historyDir)
	writeFile(t, filepath.Join(historyDir, "old.json"), `{}`)

	// Populate executor.json
	executorPath := filepath.Join(root, "proj", "results", "executor.json")
	writeFile(t, executorPath, `{"name":"ci"}`)

	if err := ls.CleanHistory(ctx, "proj"); err != nil {
		t.Fatalf("CleanHistory: %v", err)
	}

	// latest must be empty
	entries, err := os.ReadDir(latestDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("ReadDir latest: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected latest to be empty, got %d entries", len(entries))
	}

	// numbered dirs 1, 2 must be removed
	for _, n := range []string{"1", "2"} {
		if _, err := os.Stat(filepath.Join(reportsDir, n)); !os.IsNotExist(err) {
			t.Errorf("build dir %s should be removed", n)
		}
	}
	// dir 0 must remain
	if _, err := os.Stat(filepath.Join(reportsDir, "0")); err != nil {
		t.Error("build dir 0 should remain")
	}

	// results/history must be empty
	histEntries, err := os.ReadDir(historyDir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("ReadDir history: %v", err)
	}
	if len(histEntries) != 0 {
		t.Errorf("expected history to be empty, got %d entries", len(histEntries))
	}

	// executor.json must be cleared (empty content)
	data, err := os.ReadFile(executorPath)
	if err != nil {
		t.Fatalf("ReadFile executor.json: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected executor.json to be empty, got %q", data)
	}
}

// --- ReadBuildStats ---

func TestLocalStore_ReadBuildStats_Summary(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	widgetsDir := filepath.Join(root, "proj", "reports", "1", "widgets")
	mkdirAll(t, widgetsDir)
	summary := map[string]any{
		"statistic": map[string]int{
			"passed":  10,
			"failed":  2,
			"broken":  1,
			"skipped": 3,
			"unknown": 0,
			"total":   16,
		},
		"time": map[string]int64{"duration": 5000},
	}
	data, _ := json.Marshal(summary)
	writeFile(t, filepath.Join(widgetsDir, "summary.json"), string(data))

	stats, err := ls.ReadBuildStats(ctx, "proj", 1)
	if err != nil {
		t.Fatalf("ReadBuildStats summary: %v", err)
	}
	if stats.Passed != 10 || stats.Failed != 2 || stats.Total != 16 || stats.DurationMs != 5000 {
		t.Errorf("unexpected stats: %+v", stats)
	}
}

func TestLocalStore_ReadBuildStats_Statistic(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	widgetsDir := filepath.Join(root, "proj", "reports", "2", "widgets")
	mkdirAll(t, widgetsDir)
	stat := map[string]int{
		"passed":  5,
		"failed":  1,
		"broken":  0,
		"skipped": 2,
		"unknown": 0,
		"total":   8,
	}
	data, _ := json.Marshal(stat)
	writeFile(t, filepath.Join(widgetsDir, "statistic.json"), string(data))

	stats, err := ls.ReadBuildStats(ctx, "proj", 2)
	if err != nil {
		t.Fatalf("ReadBuildStats statistic: %v", err)
	}
	if stats.Passed != 5 || stats.Total != 8 {
		t.Errorf("unexpected stats: %+v", stats)
	}
}

func TestLocalStore_ReadBuildStats_NotFound(t *testing.T) {
	ls, _ := makeLocalStore(t)
	ctx := context.Background()

	_, err := ls.ReadBuildStats(ctx, "proj", 999)
	if !errors.Is(err, ErrStatsNotFound) {
		t.Errorf("expected ErrStatsNotFound, got %v", err)
	}
}

// --- ReadFile ---

func TestLocalStore_ReadFile(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	target := filepath.Join(root, "proj", "reports", "latest", "index.html")
	writeFile(t, target, "<html>hello</html>")

	got, err := ls.ReadFile(ctx, "proj", "reports/latest/index.html")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(got) != "<html>hello</html>" {
		t.Errorf("content mismatch: %q", got)
	}
}

// --- ReadDir ---

func TestLocalStore_ReadDir(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	reportsDir := filepath.Join(root, "proj", "reports")
	mkdirAll(t, filepath.Join(reportsDir, "subdir"))
	writeFile(t, filepath.Join(reportsDir, "file.txt"), "hello")

	entries, err := ls.ReadDir(ctx, "proj", "reports")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d: %v", len(entries), entries)
	}
	// find the file entry
	var found bool
	for _, e := range entries {
		if e.Name == "file.txt" && !e.IsDir {
			found = true
		}
		if e.Name == "subdir" && !e.IsDir {
			t.Error("subdir should be IsDir=true")
		}
	}
	if !found {
		t.Error("file.txt not found in ReadDir results")
	}
}

// --- OpenReportFile ---

func TestLocalStore_OpenReportFile(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	reportDir := filepath.Join(root, "proj", "reports", "1")
	writeFile(t, filepath.Join(reportDir, "index.html"), "<html/>")

	rc, contentType, err := ls.OpenReportFile(ctx, "proj", "1", "index.html")
	if err != nil {
		t.Fatalf("OpenReportFile: %v", err)
	}
	defer func() { _ = rc.Close() }()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "<html/>" {
		t.Errorf("content mismatch: %q", data)
	}
	if contentType == "" {
		t.Error("expected non-empty content type")
	}
}

// --- ListReportBuilds ---

func TestLocalStore_ListReportBuilds(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	reportsDir := filepath.Join(root, "proj", "reports")
	// Create numbered dirs in non-sorted order, plus "latest" which must be excluded
	for _, n := range []string{"3", "1", "10", "2", "latest"} {
		mkdirAll(t, filepath.Join(reportsDir, n))
	}

	builds, err := ls.ListReportBuilds(ctx, "proj")
	if err != nil {
		t.Fatalf("ListReportBuilds: %v", err)
	}
	// Must be sorted ascending, numerically
	want := []int{1, 2, 3, 10}
	if len(builds) != len(want) {
		t.Fatalf("want %v, got %v", want, builds)
	}
	for i, w := range want {
		if builds[i] != w {
			t.Errorf("index %d: want %d, got %d", i, w, builds[i])
		}
	}
}

// --- LatestReportExists ---

func TestLocalStore_LatestReportExists(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	exists, err := ls.LatestReportExists(ctx, "proj")
	if err != nil {
		t.Fatalf("LatestReportExists: %v", err)
	}
	if exists {
		t.Fatal("expected false before creating latest")
	}

	mkdirAll(t, filepath.Join(root, "proj", "reports", "latest"))
	exists, err = ls.LatestReportExists(ctx, "proj")
	if err != nil {
		t.Fatalf("LatestReportExists after create: %v", err)
	}
	if !exists {
		t.Fatal("expected true after creating latest dir")
	}
}

// --- DeleteProject ---

func TestLocalStore_DeleteProject(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	projDir := filepath.Join(root, "proj")
	if _, err := os.Stat(projDir); err != nil {
		t.Fatalf("project dir should exist before delete: %v", err)
	}

	if err := ls.DeleteProject(ctx, "proj"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	if _, err := os.Stat(projDir); !os.IsNotExist(err) {
		t.Error("project dir should be removed after DeleteProject")
	}

	// Non-existent project must return ErrProjectNotFound.
	if err := ls.DeleteProject(ctx, "ghost"); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("expected ErrProjectNotFound, got %v", err)
	}
}

// --- ResultsDirHash ---

func TestLocalStore_ResultsDirHash(t *testing.T) {
	ls, root := makeLocalStore(t)
	ctx := context.Background()

	if err := ls.CreateProject(ctx, "proj"); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	resultsDir := filepath.Join(root, "proj", "results")

	// Hash of empty dir should be consistent
	h1, err := ls.ResultsDirHash(ctx, "proj")
	if err != nil {
		t.Fatalf("ResultsDirHash empty: %v", err)
	}
	h2, err := ls.ResultsDirHash(ctx, "proj")
	if err != nil {
		t.Fatalf("ResultsDirHash empty repeat: %v", err)
	}
	if h1 != h2 {
		t.Error("hash not consistent for empty dir")
	}

	// Add a file — hash must change
	writeFile(t, filepath.Join(resultsDir, "result.xml"), "data")
	h3, err := ls.ResultsDirHash(ctx, "proj")
	if err != nil {
		t.Fatalf("ResultsDirHash with file: %v", err)
	}
	if h3 == h1 {
		t.Error("hash should change when file added")
	}

	// executor.json and allurereport.config.json must be ignored
	writeFile(t, filepath.Join(resultsDir, "executor.json"), `{"x":1}`)
	writeFile(t, filepath.Join(resultsDir, "allurereport.config.json"), `{}`)
	h4, err := ls.ResultsDirHash(ctx, "proj")
	if err != nil {
		t.Fatalf("ResultsDirHash with ignored files: %v", err)
	}
	if h4 != h3 {
		t.Error("hash should not change when ignored files added")
	}
}
