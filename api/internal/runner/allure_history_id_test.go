package runner

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/parser"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// stabilityStore is a storage.MockStore that serves one generated-report
// stability file per entry from reports/latest/data/test-results/.
func stabilityStore(t *testing.T, entries []map[string]any) *storage.MockStore {
	t.Helper()
	files := make(map[string][]byte, len(entries))
	dir := make([]storage.DirEntry, 0, len(entries))
	for i, e := range entries {
		name := "entry" + string(rune('a'+i)) + ".json"
		data, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("marshal stability entry: %v", err)
		}
		files["reports/latest/data/test-results/"+name] = data
		dir = append(dir, storage.DirEntry{Name: name})
	}
	return &storage.MockStore{
		ReadBuildStatsFn: func(_ context.Context, _ string, _ int) (storage.BuildStats, error) {
			return storage.BuildStats{Total: len(entries), Passed: len(entries)}, nil
		},
		ReadDirFn: func(_ context.Context, _ string, _ string) ([]storage.DirEntry, error) {
			return dir, nil
		},
		ReadFileFn: func(_ context.Context, _ string, relPath string) ([]byte, error) {
			data, ok := files[relPath]
			if !ok {
				return nil, os.ErrNotExist
			}
			return data, nil
		},
	}
}

// writeRawResult writes one raw Allure *-result.json into dir.
func writeRawResult(t *testing.T, dir, filename string, payload map[string]any) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal raw result: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, filename), data, 0o644); err != nil {
		t.Fatalf("write %s: %v", filename, err)
	}
}

// newCapturingAllure wires an Allure runner whose test-result writes are captured.
func newCapturingAllure(t *testing.T, tmpDir string, st storage.Store,
	batch *[]store.TestResult, full *[]*parser.Result,
) *Allure {
	t.Helper()
	mocks := testutil.New()
	mocks.TestResults.GetBuildIDFn = func(_ context.Context, _ int64, _ int) (int64, error) {
		return 42, nil
	}
	mocks.TestResults.InsertBatchFn = func(_ context.Context, results []store.TestResult) error {
		*batch = results
		return nil
	}
	mocks.TestResults.InsertBatchFullFn = func(_ context.Context, _ int64, _ int64, results []*parser.Result) error {
		*full = results
		return nil
	}
	return &Allure{
		cfg:             &config.Config{ProjectsPath: tmpDir},
		store:           st,
		buildStore:      mocks.Builds,
		testResultStore: mocks.TestResults,
		logger:          zap.NewNop(),
	}
}

// TestStoreAndPruneBuild_StabilityRowsAdoptRawHistoryID is the regression test
// for the double-ingestion bug: the Allure generator rewrites historyId in the
// report it emits, so the stability row written by InsertBatch and the enriched
// row written by InsertBatchFull landed on two different (build_id, history_id)
// keys — two rows per test in one build. Both writes must use the raw result's
// historyId so they collapse onto a single row.
func TestStoreAndPruneBuild_StabilityRowsAdoptRawHistoryID(t *testing.T) {
	tmpDir := t.TempDir()
	const fullName = "tests/ui/login.feature.spec.js:13:7"
	const rawHistoryID = "b7000c60a5af288ebdb10f8e8b616917:d93c9637fa0175fc31b7453a429c6565"
	const generatedHistoryID = "814174902e63412d15098d00d79edd81.d93c9637fa0175fc31b7453a429c6565"

	writeRawResult(t, filepath.Join(tmpDir, "results", "batch1"), "a-result.json", map[string]any{
		"name":          "should log in",
		"fullName":      fullName,
		"historyId":     rawHistoryID,
		"status":        "failed",
		"statusDetails": map[string]any{"message": "boom"},
		"start":         int64(1000),
		"stop":          int64(2000),
	})

	st := stabilityStore(t, []map[string]any{{
		"name":         "should log in",
		"fullName":     fullName,
		"historyId":    generatedHistoryID,
		"status":       "failed",
		"retriesCount": 3,
	}})

	var batch []store.TestResult
	var full []*parser.Result
	a := newCapturingAllure(t, tmpDir, st, &batch, &full)

	if err := a.storeAndPruneBuild(context.Background(), 1, "proj", "proj", "batch1", tmpDir, 1, store.CIMetadata{}, nil); err != nil {
		t.Fatalf("storeAndPruneBuild: %v", err)
	}

	if len(batch) != 1 {
		t.Fatalf("InsertBatch: got %d rows, want 1", len(batch))
	}
	if batch[0].HistoryID != rawHistoryID {
		t.Errorf("stability row history_id = %q, want the raw result's %q", batch[0].HistoryID, rawHistoryID)
	}
	// Stability-only fields must survive the rewrite.
	if batch[0].Retries != 3 {
		t.Errorf("stability row retries = %d, want 3", batch[0].Retries)
	}

	if len(full) != 1 {
		t.Fatalf("InsertBatchFull: got %d rows, want 1", len(full))
	}
	if full[0].HistoryID != batch[0].HistoryID {
		t.Errorf("enrichment history_id = %q, stability history_id = %q — must match so both writes hit one row",
			full[0].HistoryID, batch[0].HistoryID)
	}
}

// TestStoreAndPruneBuild_AmbiguousFullNameKeepsGeneratedHistoryID verifies the
// guard for parameterized tests: several raw results share one fullName but
// carry distinct historyIds, so there is no single id to adopt. Those stability
// rows keep the generated historyId rather than being collapsed onto one row.
func TestStoreAndPruneBuild_AmbiguousFullNameKeepsGeneratedHistoryID(t *testing.T) {
	tmpDir := t.TempDir()
	const fullName = "tests/api/datadriven.spec.js:9:3"
	resultsDir := filepath.Join(tmpDir, "results", "batch1")

	writeRawResult(t, resultsDir, "a-result.json", map[string]any{
		"name": "case", "fullName": fullName, "historyId": "raw-a", "status": "passed",
		"parameters": []map[string]any{{"name": "n", "value": "1"}},
	})
	writeRawResult(t, resultsDir, "b-result.json", map[string]any{
		"name": "case", "fullName": fullName, "historyId": "raw-b", "status": "passed",
		"parameters": []map[string]any{{"name": "n", "value": "2"}},
	})

	st := stabilityStore(t, []map[string]any{
		{"name": "case", "fullName": fullName, "historyId": "gen-a", "status": "passed"},
		{"name": "case", "fullName": fullName, "historyId": "gen-b", "status": "passed"},
	})

	var batch []store.TestResult
	var full []*parser.Result
	a := newCapturingAllure(t, tmpDir, st, &batch, &full)

	if err := a.storeAndPruneBuild(context.Background(), 1, "proj", "proj", "batch1", tmpDir, 1, store.CIMetadata{}, nil); err != nil {
		t.Fatalf("storeAndPruneBuild: %v", err)
	}

	if len(batch) != 2 {
		t.Fatalf("InsertBatch: got %d rows, want 2", len(batch))
	}
	for _, r := range batch {
		if r.HistoryID != "gen-a" && r.HistoryID != "gen-b" {
			t.Errorf("ambiguous fullName must keep its generated history_id, got %q", r.HistoryID)
		}
	}
}

// TestStoreAndPruneBuild_AmbiguousStabilityFullNameKeepsGeneratedHistoryID is
// the mirror of the guard above, on the side it was missing from. Ambiguity was
// only ever detected among the RAW results, but the rewrite is applied to the
// STABILITY entries: when two of those share a fullName that maps to a single
// unambiguous raw historyId, both adopt it, both land on the same (build_id,
// history_id) key, and InsertBatch's latest-stop-wins upsert silently keeps one
// of the two tests. Neither entry may be rewritten in that case.
func TestStoreAndPruneBuild_AmbiguousStabilityFullNameKeepsGeneratedHistoryID(t *testing.T) {
	tmpDir := t.TempDir()
	const fullName = "tests/api/shared.spec.js:9:3"

	// Exactly ONE raw result for this fullName, so the raw-side index is
	// unambiguous and offers "raw-only" as the id to adopt.
	writeRawResult(t, filepath.Join(tmpDir, "results", "batch1"), "a-result.json", map[string]any{
		"name": "case", "fullName": fullName, "historyId": "raw-only", "status": "passed",
	})

	// ...but the generated report emits TWO entries under that fullName.
	st := stabilityStore(t, []map[string]any{
		{"name": "case one", "fullName": fullName, "historyId": "gen-a", "status": "passed"},
		{"name": "case two", "fullName": fullName, "historyId": "gen-b", "status": "failed"},
	})

	var batch []store.TestResult
	var full []*parser.Result
	a := newCapturingAllure(t, tmpDir, st, &batch, &full)

	if err := a.storeAndPruneBuild(context.Background(), 1, "proj", "proj", "batch1", tmpDir, 1, store.CIMetadata{}, nil); err != nil {
		t.Fatalf("storeAndPruneBuild: %v", err)
	}

	if len(batch) != 2 {
		t.Fatalf("InsertBatch: got %d rows, want 2", len(batch))
	}
	seen := map[string]bool{}
	for _, r := range batch {
		if r.HistoryID == "raw-only" {
			t.Errorf("stability rows sharing a fullName must keep their generated history_id; %q was rewritten to the raw id, collapsing two tests onto one row", r.TestName)
		}
		if seen[r.HistoryID] {
			t.Errorf("duplicate history_id %q across stability rows", r.HistoryID)
		}
		seen[r.HistoryID] = true
	}
}

// TestStoreAndPruneBuild_NoRawResultsKeepsGeneratedHistoryID verifies that a
// build with no parseable raw results (enrichment skipped) still writes its
// stability rows unchanged.
func TestStoreAndPruneBuild_NoRawResultsKeepsGeneratedHistoryID(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "results", "batch1"), 0o755); err != nil {
		t.Fatal(err)
	}

	st := stabilityStore(t, []map[string]any{
		{"name": "solo", "fullName": "spec.js:1:1", "historyId": "gen-only", "status": "passed"},
	})

	var batch []store.TestResult
	var full []*parser.Result
	a := newCapturingAllure(t, tmpDir, st, &batch, &full)

	if err := a.storeAndPruneBuild(context.Background(), 1, "proj", "proj", "batch1", tmpDir, 1, store.CIMetadata{}, nil); err != nil {
		t.Fatalf("storeAndPruneBuild: %v", err)
	}

	if len(batch) != 1 {
		t.Fatalf("InsertBatch: got %d rows, want 1", len(batch))
	}
	if batch[0].HistoryID != "gen-only" {
		t.Errorf("history_id = %q, want %q", batch[0].HistoryID, "gen-only")
	}
	if len(full) != 0 {
		t.Errorf("InsertBatchFull must not run without raw results, got %d rows", len(full))
	}
}

// TestPlaywrightRunner_IngestReport_OneRowPerTest guards the Playwright-only
// ingestion path: it has no Allure results to reconcile against, so it must
// keep writing test rows, and its stability and enrichment writes must share a
// history_id so each test lands on exactly one row.
func TestPlaywrightRunner_IngestReport_OneRowPerTest(t *testing.T) {
	projectsDir := t.TempDir()
	slug := "pw-single-row"

	pwLatestDir := filepath.Join(projectsDir, slug, "playwright-reports", "latest")
	mustWriteFile(t, filepath.Join(pwLatestDir, "index.html"), string(buildTestPlaywrightHTML(t)))

	cfg := &config.Config{ProjectsPath: projectsDir}
	mocks := testutil.New()
	mocks.Builds.ReserveBuildFn = func(_ context.Context, _ int64) (int, error) { return 1, nil }
	mocks.TestResults.GetBuildIDFn = func(_ context.Context, _ int64, _ int) (int64, error) { return 42, nil }
	mocks.Branches.GetOrCreateFn = func(_ context.Context, _ int64, _ string) (*store.Branch, bool, error) {
		return &store.Branch{ID: 1, Name: "main"}, false, nil
	}

	var batch []store.TestResult
	var full []*parser.Result
	mocks.TestResults.InsertBatchFn = func(_ context.Context, results []store.TestResult) error {
		batch = results
		return nil
	}
	mocks.TestResults.InsertBatchFullFn = func(_ context.Context, _ int64, _ int64, results []*parser.Result) error {
		full = results
		return nil
	}

	pr := NewPlaywrightRunner(PlaywrightRunnerDeps{
		Config:          cfg,
		Store:           storage.NewLocalStore(cfg),
		BuildStore:      mocks.Builds,
		Locker:          mocks.Locker,
		TestResultStore: mocks.TestResults,
		BranchStore:     mocks.Branches,
		Logger:          zap.NewNop(),
	})

	if _, err := pr.IngestReport(context.Background(), 20, slug, slug, "CI Runner", "", "", "", "", ""); err != nil {
		t.Fatalf("IngestReport: %v", err)
	}

	if len(batch) != 3 {
		t.Fatalf("InsertBatch: got %d rows, want 3", len(batch))
	}
	if len(full) != 3 {
		t.Fatalf("InsertBatchFull: got %d rows, want 3", len(full))
	}
	stability := make(map[string]string, len(batch))
	for _, r := range batch {
		stability[r.FullName] = r.HistoryID
	}
	for _, r := range full {
		id, ok := stability[r.FullName]
		if !ok {
			t.Errorf("enriched result %q has no stability row", r.FullName)
			continue
		}
		if id != r.HistoryID {
			t.Errorf("%q: stability history_id %q != enrichment history_id %q — would create a second row",
				r.FullName, id, r.HistoryID)
		}
	}
}
