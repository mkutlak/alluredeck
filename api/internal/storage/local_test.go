package storage

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// newLocal returns a LocalStore rooted at a fresh temp dir.
func newLocal(t *testing.T) (*LocalStore, string) {
	t.Helper()
	dir := t.TempDir()
	return NewLocalStore(&config.Config{ProjectsPath: dir}), dir
}

// mkdirAll creates path and its parents.
func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdirAll %q: %v", path, err)
	}
}

// writeFile writes content to path, creating parent dirs as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writeFile %q: %v", path, err)
	}
}

// checkExists asserts, for each path under root, whether it exists.
func checkExists(t *testing.T, root string, want map[string]bool) {
	t.Helper()
	for rel, wantExists := range want {
		_, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel)))
		if exists := err == nil; exists != wantExists {
			t.Errorf("%s exists = %v, want %v (stat: %v)", filepath.Join(root, rel), exists, wantExists, err)
		}
	}
}

// TestLocalStore_HealthCheck probes by writing, not by stat: the e2e stack
// broke on a projects dir that stats fine but is owned by another uid
// (fix 7741a20).
func TestLocalStore_HealthCheck(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		setup   func(t *testing.T, dir string) string // returns ProjectsPath
		wantErr bool
	}{
		{"writable dir", func(_ *testing.T, dir string) string { return dir }, false},
		{"missing dir", func(_ *testing.T, dir string) string { return filepath.Join(dir, "not-mounted") }, true},
		{"read-only dir", func(t *testing.T, dir string) string {
			if os.Geteuid() == 0 {
				t.Skip("running as root bypasses directory mode bits")
			}
			if err := os.Chmod(dir, 0o555); err != nil {
				t.Fatalf("chmod %q: %v", dir, err)
			}
			// Restore write permission so t.TempDir() teardown can remove the dir.
			t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
			return dir
		}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ls := NewLocalStore(&config.Config{ProjectsPath: tc.setup(t, t.TempDir())})
			if err := ls.HealthCheck(context.Background()); (err != nil) != tc.wantErr {
				t.Errorf("HealthCheck() = %v, want error: %v", err, tc.wantErr)
			}
		})
	}
}

// TestLocalStore_ProjectLifecycle creates (idempotently), lists, prepares and
// deletes project dirs. LocalStore generates in place, so CleanupLocal must
// leave the project dir alone.
func TestLocalStore_ProjectLifecycle(t *testing.T) {
	t.Parallel()
	ls, root := newLocal(t)
	ctx := context.Background()

	if list, err := ls.ListProjects(ctx); err != nil || len(list) != 0 {
		t.Fatalf("ListProjects on an empty root = %v, %v", list, err)
	}
	for _, id := range []string{"gamma", "alpha", "beta", "gamma"} {
		if err := ls.CreateProject(ctx, id); err != nil {
			t.Fatalf("CreateProject(%s): %v", id, err)
		}
	}
	if list, err := ls.ListProjects(ctx); err != nil || !slices.Equal(list, []string{"alpha", "beta", "gamma"}) {
		t.Errorf("ListProjects = %v, %v", list, err)
	}
	dir, err := ls.PrepareLocal(ctx, "delta", nil)
	if err != nil || dir != filepath.Join(root, "delta") {
		t.Fatalf("PrepareLocal = %q, %v; want %q", dir, err, filepath.Join(root, "delta"))
	}
	if err := ls.CleanupLocal(dir); err != nil {
		t.Fatalf("CleanupLocal: %v", err)
	}
	checkExists(t, root, map[string]bool{"alpha/results": true, "alpha/reports": true, "delta/results": true, "delta/reports": true})

	for id, want := range map[string]bool{"alpha": true, "ghost": false} {
		if ok, err := ls.ProjectExists(ctx, id); err != nil || ok != want {
			t.Errorf("ProjectExists(%s) = %v, %v; want %v", id, ok, err, want)
		}
	}
	if err := ls.DeleteProject(ctx, "alpha"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}
	checkExists(t, root, map[string]bool{"alpha": false})
	if err := ls.DeleteProject(ctx, "ghost"); !errors.Is(err, ErrProjectNotFound) {
		t.Errorf("DeleteProject(ghost) = %v, want ErrProjectNotFound", err)
	}
}

// TestLocalStore_Results writes files under results/<batch>/, lists only the
// batch's files, and CleanResults empties results/ (a missing project is fine).
func TestLocalStore_Results(t *testing.T) {
	t.Parallel()
	ls, root := newLocal(t)
	ctx := context.Background()
	batchDir := filepath.Join(root, "proj", "results", "batch1")

	if files, err := ls.ListResultFiles(ctx, "proj", "batch1"); err != nil || len(files) != 0 {
		t.Fatalf("ListResultFiles before any upload = %v, %v", files, err)
	}
	for _, name := range []string{"b.json", "a.xml"} {
		if err := ls.WriteResultFile(ctx, "proj", "batch1", name, strings.NewReader("data-"+name)); err != nil {
			t.Fatalf("WriteResultFile(%s): %v", name, err)
		}
	}
	mkdirAll(t, filepath.Join(batchDir, "subdir"))
	files, err := ls.ListResultFiles(ctx, "proj", "batch1")
	sort.Strings(files)
	if err != nil || !slices.Equal(files, []string{"a.xml", "b.json"}) {
		t.Errorf("ListResultFiles = %v, %v", files, err)
	}
	if data, err := os.ReadFile(filepath.Join(batchDir, "a.xml")); err != nil || string(data) != "data-a.xml" {
		t.Errorf("results/batch1/a.xml = %q, %v", data, err)
	}

	if err := ls.CleanResults(ctx, "proj"); err != nil {
		t.Fatalf("CleanResults: %v", err)
	}
	if entries, err := os.ReadDir(filepath.Join(root, "proj", "results")); err != nil || len(entries) != 0 {
		t.Errorf("results/ after CleanResults: %d entries, %v", len(entries), err)
	}
	if err := ls.CleanResults(ctx, "nonexistent"); err != nil {
		t.Errorf("CleanResults(nonexistent): %v", err)
	}
}

// TestLocalStore_PublishReport snapshots only the variable dirs of
// reports/latest into reports/<build>; static assets stay in latest, where the
// overlay handler serves them from.
func TestLocalStore_PublishReport(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		latest []string        // files under reports/latest; nil = no latest dir
		want   map[string]bool // paths under reports/1 → exists
	}{
		{"copies only the variable dirs", []string{"data/f.json", "widgets/f.json", "history/f.json", "index.html", "plugins/p/x.js"},
			map[string]bool{"data/f.json": true, "widgets/f.json": true, "history/f.json": true, "index.html": false, "plugins": false}},
		{"empty latest creates no build dir", []string{}, map[string]bool{"": false}},
		{"missing latest is not an error", nil, map[string]bool{"": false}},
	}
	for _, tc := range tests {
		ls, root := newLocal(t)
		latest := filepath.Join(root, "proj", "reports", "latest")
		if tc.latest != nil {
			mkdirAll(t, latest)
		}
		for _, f := range tc.latest {
			writeFile(t, filepath.Join(latest, filepath.FromSlash(f)), f)
		}
		if err := ls.PublishReport(context.Background(), "proj", 1, filepath.Join(root, "proj"), nil); err != nil {
			t.Fatalf("%s: PublishReport: %v", tc.name, err)
		}
		checkExists(t, filepath.Join(root, "proj", "reports", "1"), tc.want)
	}
}

func TestLocalStore_DeleteReport(t *testing.T) {
	t.Parallel()
	ls, root := newLocal(t)
	mkdirAll(t, filepath.Join(root, "proj", "reports", "3"))
	for _, tc := range []struct {
		id   string
		want error
	}{
		{"3", nil},
		{"99", ErrReportNotFound},
		{"latest", ErrReportIDInvalid},
		{"", ErrReportIDEmpty},
	} {
		if err := ls.DeleteReport(context.Background(), "proj", tc.id); !errors.Is(err, tc.want) {
			t.Errorf("DeleteReport(%q) = %v, want %v", tc.id, err, tc.want)
		}
	}
	checkExists(t, root, map[string]bool{"proj/reports/3": false})
}

// TestLocalStore_ReportDirs lists numbered report dirs in numeric order
// (never "latest"), reports whether latest exists, and prunes build dirs.
func TestLocalStore_ReportDirs(t *testing.T) {
	t.Parallel()
	ls, root := newLocal(t)
	ctx := context.Background()

	if ok, err := ls.LatestReportExists(ctx, "proj"); err != nil || ok {
		t.Errorf("LatestReportExists before latest = %v, %v", ok, err)
	}
	for _, n := range []string{"3", "1", "10", "2", "latest"} {
		mkdirAll(t, filepath.Join(root, "proj", "reports", n))
	}
	if ok, err := ls.LatestReportExists(ctx, "proj"); err != nil || !ok {
		t.Errorf("LatestReportExists = %v, %v", ok, err)
	}
	if builds, err := ls.ListReportBuilds(ctx, "proj"); err != nil || !slices.Equal(builds, []int{1, 2, 3, 10}) {
		t.Errorf("ListReportBuilds = %v, %v", builds, err)
	}
	if err := ls.PruneReportDirs(ctx, "proj", []int{1, 3}); err != nil {
		t.Fatalf("PruneReportDirs: %v", err)
	}
	if builds, _ := ls.ListReportBuilds(ctx, "proj"); !slices.Equal(builds, []int{2, 10}) {
		t.Errorf("ListReportBuilds after pruning 1 and 3 = %v", builds)
	}
}

// TestLocalStore_KeepHistory copies reports/latest/history into
// results/history when KEEP_HISTORY is on, and removes results/history when off.
func TestLocalStore_KeepHistory(t *testing.T) {
	t.Parallel()
	for _, keep := range []bool{true, false} {
		root := t.TempDir()
		ls := NewLocalStore(&config.Config{ProjectsPath: root, KeepHistory: keep})
		writeFile(t, filepath.Join(root, "proj", "reports", "latest", "history", "trend.json"), `{"x":1}`)
		if !keep {
			writeFile(t, filepath.Join(root, "proj", "results", "history", "old.json"), `{}`)
		}
		if err := ls.KeepHistory(context.Background(), "proj", ""); err != nil {
			t.Fatalf("keep=%t: KeepHistory: %v", keep, err)
		}
		checkExists(t, root, map[string]bool{"proj/results/history/trend.json": keep, "proj/results/history": keep})
	}
}

// TestLocalStore_CleanHistory empties latest and results/history, removes
// every numbered build except 0, and truncates executor.json.
func TestLocalStore_CleanHistory(t *testing.T) {
	t.Parallel()
	ls, root := newLocal(t)
	proj := filepath.Join(root, "proj")
	writeFile(t, filepath.Join(proj, "reports", "latest", "index.html"), "<html/>")
	for _, n := range []string{"0", "1", "2"} {
		mkdirAll(t, filepath.Join(proj, "reports", n))
	}
	writeFile(t, filepath.Join(proj, "results", "history", "old.json"), `{}`)
	executor := filepath.Join(proj, "results", "executor.json")
	writeFile(t, executor, `{"name":"ci"}`)

	if err := ls.CleanHistory(context.Background(), "proj"); err != nil {
		t.Fatalf("CleanHistory: %v", err)
	}
	checkExists(t, proj, map[string]bool{
		"reports/latest/index.html": false, "reports/0": true, "reports/1": false, "reports/2": false,
		"results/history/old.json": false,
	})
	if data, err := os.ReadFile(executor); err != nil || len(data) != 0 {
		t.Errorf("executor.json = %q, %v; want empty", data, err)
	}
}

func TestLocalStore_ReadBuildStats(t *testing.T) {
	t.Parallel()
	ls, root := newLocal(t)
	reports := filepath.Join(root, "proj", "reports")
	// Build 1 is Allure 2: widgets/summary.json carries counts and duration.
	writeFile(t, filepath.Join(reports, "1", "widgets", "summary.json"),
		`{"statistic":{"passed":10,"failed":2,"broken":1,"skipped":3,"unknown":0,"total":16},"time":{"duration":5000}}`)
	// Build 2 is Allure 3: statistic.json has no timing, so the duration is the
	// wall clock from the earliest start to the latest stop (not the 6000ms sum).
	writeFile(t, filepath.Join(reports, "2", "widgets", "statistic.json"),
		`{"passed":3,"failed":1,"broken":0,"skipped":0,"unknown":0,"total":4}`)
	writeFile(t, filepath.Join(reports, "2", "data", "test-results", "a.json"), `{"start":1700000000000,"stop":1700000002000}`)
	writeFile(t, filepath.Join(reports, "2", "data", "test-results", "b.json"), `{"start":1700000001000,"stop":1700000005000}`)

	for _, tc := range []struct {
		build   int
		want    BuildStats
		wantErr error
	}{
		{1, BuildStats{Passed: 10, Failed: 2, Broken: 1, Skipped: 3, Total: 16, DurationMs: 5000}, nil},
		{2, BuildStats{Passed: 3, Failed: 1, Total: 4, DurationMs: 5000}, nil},
		{999, BuildStats{}, ErrStatsNotFound},
	} {
		got, err := ls.ReadBuildStats(context.Background(), "proj", tc.build)
		if got != tc.want || !errors.Is(err, tc.wantErr) {
			t.Errorf("ReadBuildStats(%d) = %+v, %v; want %+v, %v", tc.build, got, err, tc.want, tc.wantErr)
		}
	}
}

// TestLocalStore_Readers reads project-relative files and dirs and opens
// report files with a MIME type taken from the extension.
func TestLocalStore_Readers(t *testing.T) {
	t.Parallel()
	ls, root := newLocal(t)
	ctx := context.Background()
	writeFile(t, filepath.Join(root, "proj", "reports", "1", "index.html"), "<html/>")
	writeFile(t, filepath.Join(root, "proj", "reports", "file.txt"), "hello")

	if data, err := ls.ReadFile(ctx, "proj", "reports/1/index.html"); err != nil || string(data) != "<html/>" {
		t.Errorf("ReadFile = %q, %v", data, err)
	}
	entries, err := ls.ReadDir(ctx, "proj", "reports")
	isDir := map[string]bool{}
	for _, e := range entries {
		isDir[e.Name] = e.IsDir
	}
	if err != nil || !reflect.DeepEqual(isDir, map[string]bool{"1": true, "file.txt": false}) {
		t.Errorf("ReadDir = %+v, %v", entries, err)
	}

	rc, contentType, err := ls.OpenReportFile(ctx, "proj", "1", "index.html")
	if err != nil {
		t.Fatalf("OpenReportFile: %v", err)
	}
	defer func() { _ = rc.Close() }()
	if data, _ := io.ReadAll(rc); string(data) != "<html/>" || !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("OpenReportFile = %q (%s)", data, contentType)
	}
}

// TestLocalStore_ResultsDirHash changes when results change, but not for the
// executor.json and allurereport.config.json that report generation writes.
func TestLocalStore_ResultsDirHash(t *testing.T) {
	t.Parallel()
	ls, root := newLocal(t)
	ctx := context.Background()
	resultsDir := filepath.Join(root, "proj", "results")
	mkdirAll(t, resultsDir)
	hash := func() string {
		t.Helper()
		h, err := ls.ResultsDirHash(ctx, "proj")
		if err != nil {
			t.Fatalf("ResultsDirHash: %v", err)
		}
		return h
	}

	empty := hash()
	if hash() != empty {
		t.Error("hash not stable for an unchanged dir")
	}
	writeFile(t, filepath.Join(resultsDir, "result.xml"), "data")
	withResult := hash()
	if withResult == empty {
		t.Error("hash should change when a result is added")
	}
	writeFile(t, filepath.Join(resultsDir, "executor.json"), `{"x":1}`)
	writeFile(t, filepath.Join(resultsDir, "allurereport.config.json"), `{}`)
	if hash() != withResult {
		t.Error("hash should ignore executor.json and allurereport.config.json")
	}
}

// TestLocalStore_Playwright walks one project through the Playwright report
// lifecycle under playwright-reports/: upload to latest, snapshot latest into
// a numbered build, read it back, and clean latest. Missing dirs are no-ops.
func TestLocalStore_Playwright(t *testing.T) {
	t.Parallel()
	ls, root := newLocal(t)
	ctx := context.Background()
	reports := filepath.Join(root, "proj1", "playwright-reports")

	if files, err := ls.ListPlaywrightDataFiles(ctx, "proj1", 2); err != nil || len(files) != 0 {
		t.Errorf("ListPlaywrightDataFiles without a report = %v, %v", files, err)
	}
	if ok, err := ls.PlaywrightReportExists(ctx, "proj1", 2); err != nil || ok {
		t.Errorf("PlaywrightReportExists without a report = %v, %v", ok, err)
	}
	if err := ls.CopyPlaywrightLatestToBuild(ctx, "proj1", 1); err != nil {
		t.Errorf("CopyPlaywrightLatestToBuild without latest: %v", err)
	}
	if err := ls.CleanPlaywrightLatest(ctx, "proj1"); err != nil {
		t.Errorf("CleanPlaywrightLatest without latest: %v", err)
	}
	checkExists(t, reports, map[string]bool{"1": false})

	for _, f := range []string{"latest/index.html", "latest/data/a.json", "latest/data/b.json"} {
		if err := ls.WritePlaywrightFile(ctx, "proj1", f, strings.NewReader("content of "+f)); err != nil {
			t.Fatalf("WritePlaywrightFile(%s): %v", f, err)
		}
	}
	mkdirAll(t, filepath.Join(reports, "latest", "data", "subdir"))
	if err := ls.CopyPlaywrightLatestToBuild(ctx, "proj1", 2); err != nil {
		t.Fatalf("CopyPlaywrightLatestToBuild: %v", err)
	}
	if ok, err := ls.PlaywrightReportExists(ctx, "proj1", 2); err != nil || !ok {
		t.Errorf("PlaywrightReportExists after the copy = %v, %v", ok, err)
	}
	files, err := ls.ListPlaywrightDataFiles(ctx, "proj1", 2)
	sort.Strings(files)
	if err != nil || !slices.Equal(files, []string{"a.json", "b.json"}) {
		t.Errorf("ListPlaywrightDataFiles = %v, %v", files, err)
	}

	rc, contentType, err := ls.ReadPlaywrightFile(ctx, "proj1", "2/index.html")
	if err != nil {
		t.Fatalf("ReadPlaywrightFile: %v", err)
	}
	data, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(data) != "content of latest/index.html" || !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("ReadPlaywrightFile = %q (%s)", data, contentType)
	}
	if _, _, err := ls.ReadPlaywrightFile(ctx, "proj1", "99/index.html"); err == nil {
		t.Error("ReadPlaywrightFile of a missing file: want an error")
	}

	if err := ls.CleanPlaywrightLatest(ctx, "proj1"); err != nil {
		t.Fatalf("CleanPlaywrightLatest: %v", err)
	}
	if entries, err := os.ReadDir(filepath.Join(reports, "latest")); len(entries) != 0 || (err != nil && !os.IsNotExist(err)) {
		t.Errorf("latest after CleanPlaywrightLatest: %d entries, %v", len(entries), err)
	}
}
