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
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// loadAllure3Fixture copies the raw result files of testdata/allure3 into
// <projectDir>/results/batch1 and returns the generated report's
// data/test-results entries (superseded attempt first, as the plugin lists it).
func loadAllure3Fixture(t *testing.T, projectDir string) []map[string]any {
	t.Helper()
	var stability []map[string]any
	for _, f := range []string{"flaky-superseded.json", "flaky-final.json", "plain.json"} {
		data, err := os.ReadFile(filepath.Join("testdata", "allure3", "report", f))
		if err != nil {
			t.Fatal(err)
		}
		var m map[string]any
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		stability = append(stability, m)
	}

	resultsDir := filepath.Join(projectDir, "results", "batch1")
	if err := os.MkdirAll(resultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	raws, err := filepath.Glob(filepath.Join("testdata", "allure3", "results", "*-result.json"))
	if err != nil || len(raws) != 3 {
		t.Fatalf("raw results = %v, err = %v; want 3 files", raws, err)
	}
	for _, p := range raws {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(resultsDir, filepath.Base(p)), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return stability
}

// TestStoreAndPruneBuild_Allure3RetriedFlaky replays real Allure 3.22.1 output
// (testdata/allure3, generated with the awesome plugin from results/): a test
// that failed then passed with statusDetails.flaky=true on both attempts, plus
// a plain test. The plugin writes EVERY attempt to data/test-results/; the
// superseded one has isRetry=true yet still carries flaky=true and the same
// retriesCount, and no entry has a historyId. Allure's own statistic counts one
// flaky and one retried test, so must we, and the test must land as exactly one
// flaky row under the raw historyId — not two flaky rows under "" (the shared
// fullName reads as ambiguous) plus an unflagged third row from the raw results.
func TestStoreAndPruneBuild_Allure3RetriedFlaky(t *testing.T) {
	tmpDir := t.TempDir()
	stability := loadAllure3Fixture(t, tmpDir)

	var batch []store.TestResult
	var full []*parser.Result
	var stats store.BuildStats
	mocks := testutil.New()
	mocks.TestResults.GetBuildIDFn = func(context.Context, int64, int) (int64, error) { return 42, nil }
	mocks.TestResults.InsertBatchFn = func(_ context.Context, rs []store.TestResult) error { batch = rs; return nil }
	mocks.TestResults.InsertBatchFullFn = func(_ context.Context, _, _ int64, rs []*parser.Result) error { full = rs; return nil }
	mocks.Builds.UpdateBuildStatsFn = func(_ context.Context, _ int64, _ int, s store.BuildStats) error { stats = s; return nil }
	a := &Allure{
		cfg: &config.Config{ProjectsPath: tmpDir}, store: stabilityStore(t, stability),
		buildStore: mocks.Builds, testResultStore: mocks.TestResults, logger: zap.NewNop(),
	}

	if err := a.storeAndPruneBuild(context.Background(), 7, "proj", "proj", "batch1", tmpDir, 1, store.CIMetadata{}, nil); err != nil {
		t.Fatalf("storeAndPruneBuild: %v", err)
	}

	if stats.FlakyCount != 1 || stats.RetriedCount != 1 {
		t.Errorf("flaky_count = %d, retried_count = %d, want 1 and 1 (Allure's statistic skips isRetry attempts)", stats.FlakyCount, stats.RetriedCount)
	}
	if len(batch) != 2 {
		t.Fatalf("InsertBatch rows = %+v, want one per test (2)", batch)
	}
	byName := map[string]store.TestResult{}
	for _, r := range batch {
		byName[r.TestName] = r
	}
	if r := byName["flakyTest"]; !r.Flaky || r.Retries != 1 || r.HistoryID != "hid-flaky" || r.Status != "passed" {
		t.Errorf("flakyTest row = %+v, want flaky, retries 1, raw history id hid-flaky, final status passed", r)
	}
	if r := byName["plainTest"]; r.Flaky || r.HistoryID != "hid-plain" {
		t.Errorf("plainTest row = %+v, want not flaky, raw history id hid-plain", r)
	}
	if len(full) != 3 {
		t.Errorf("InsertBatchFull got %d raw results, want all 3 attempts untouched", len(full))
	}
}

// TestStoreAndPruneBuild_FlakyShapes pins that a flaky test is recorded
// whichever shape the generated report uses. Allure 3 writes `flaky` as a
// top-level field of each data/test-results/*.json entry and emits no
// statusDetails at all (verified against allure 3.22.1); the legacy shape nests
// it under statusDetails.flaky. Reading only the legacy field left
// test_results.flaky and builds.flaky_count at 0 for every Allure 3 report.
func TestStoreAndPruneBuild_FlakyShapes(t *testing.T) {
	entry := func(name string, extra map[string]any) map[string]any {
		m := map[string]any{"name": name, "fullName": "suite." + name, "historyId": "h-" + name, "status": "passed"}
		for k, v := range extra {
			m[k] = v
		}
		return m
	}
	tests := []struct {
		name      string
		stability []map[string]any
		wantFlaky map[string]bool // test name -> expected TestResult.Flaky
		wantCount int
	}{
		{
			name:      "allure 3 top-level flaky",
			stability: []map[string]any{entry("a3", map[string]any{"flaky": true}), entry("plain3", map[string]any{"flaky": false})},
			wantFlaky: map[string]bool{"a3": true, "plain3": false},
			wantCount: 1,
		},
		{
			name:      "legacy statusDetails flaky",
			stability: []map[string]any{entry("legacy", map[string]any{"statusDetails": map[string]any{"flaky": true}}), entry("plainl", nil)},
			wantFlaky: map[string]bool{"legacy": true, "plainl": false},
			wantCount: 1,
		},
		{
			name: "both shapes in one report",
			stability: []map[string]any{
				entry("top", map[string]any{"flaky": true}),
				entry("nested", map[string]any{"statusDetails": map[string]any{"flaky": true}}),
				entry("both", map[string]any{"flaky": true, "statusDetails": map[string]any{"flaky": true}}),
			},
			wantFlaky: map[string]bool{"top": true, "nested": true, "both": true},
			wantCount: 3,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			var batch []store.TestResult
			var stats store.BuildStats
			mocks := testutil.New()
			mocks.TestResults.GetBuildIDFn = func(context.Context, int64, int) (int64, error) { return 42, nil }
			mocks.TestResults.InsertBatchFn = func(_ context.Context, rs []store.TestResult) error { batch = rs; return nil }
			mocks.Builds.UpdateBuildStatsFn = func(_ context.Context, _ int64, _ int, s store.BuildStats) error { stats = s; return nil }
			a := &Allure{
				cfg: &config.Config{ProjectsPath: tmpDir}, store: stabilityStore(t, tc.stability),
				buildStore: mocks.Builds, testResultStore: mocks.TestResults, logger: zap.NewNop(),
			}

			if err := a.storeAndPruneBuild(context.Background(), 7, "proj", "proj", "batch1", tmpDir, 1, store.CIMetadata{}, nil); err != nil {
				t.Fatalf("storeAndPruneBuild: %v", err)
			}

			if stats.FlakyCount != tc.wantCount {
				t.Errorf("builds.flaky_count = %d, want %d", stats.FlakyCount, tc.wantCount)
			}
			if len(batch) != len(tc.wantFlaky) {
				t.Fatalf("InsertBatch rows = %d, want %d", len(batch), len(tc.wantFlaky))
			}
			for _, r := range batch {
				if want := tc.wantFlaky[r.TestName]; r.Flaky != want {
					t.Errorf("test %q Flaky = %v, want %v", r.TestName, r.Flaky, want)
				}
			}
		})
	}
}
