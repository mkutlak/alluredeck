package runner

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
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

// TestStoreAndPruneBuild_TestResultWrites pins how one build's raw results
// (results/<batch>/*-result.json) and the generated report's stability entries
// reach test_results. The Allure generator rewrites historyId in the report it
// emits, so the stability row (InsertBatch) and the enriched row
// (InsertBatchFull) used to land on two different (build_id, history_id) keys —
// two rows per test. The stability row must adopt the raw historyId, except
// when a fullName is ambiguous on either side (parameterized tests, or several
// report entries under one raw id), where adopting it would collapse distinct
// tests onto one row.
func TestStoreAndPruneBuild_TestResultWrites(t *testing.T) {
	const fullName = "tests/ui/login.feature.spec.js:13:7"
	const rawID = "b7000c60a5af288ebdb10f8e8b616917:d93c9637fa0175fc31b7453a429c6565"
	raw := func(hid, param string) map[string]any {
		m := map[string]any{"name": "case", "fullName": fullName, "historyId": hid, "status": "failed",
			"statusDetails": map[string]any{"message": "boom"}, "start": int64(1000), "stop": int64(2000)}
		if param != "" {
			m["parameters"] = []map[string]any{{"name": "n", "value": param}}
		}
		return m
	}
	gen := func(name, hid string) map[string]any {
		return map[string]any{"name": name, "fullName": fullName, "historyId": hid, "status": "failed"}
	}
	retried := gen("case", "814174902e63412d15098d00d79edd81.d93c9637fa0175fc31b7453a429c6565")
	retried["retriesCount"] = 3

	tests := []struct {
		name        string
		raw         []map[string]any
		stability   []map[string]any
		fullErr     error
		wantBatch   []string // stability-row history ids, sorted
		wantFull    []string // enriched-row history ids, sorted
		wantRetries int
	}{
		{
			name: "stability row adopts the raw historyId", raw: []map[string]any{raw(rawID, "")},
			stability: []map[string]any{retried}, wantBatch: []string{rawID}, wantFull: []string{rawID}, wantRetries: 3,
		},
		{
			// One stability entry, so only the raw-side guard can keep gen-a.
			name:      "ambiguous raw fullName keeps the generated id",
			raw:       []map[string]any{raw("raw-a", "1"), raw("raw-b", "2")},
			stability: []map[string]any{gen("case", "gen-a")},
			wantBatch: []string{"gen-a"}, wantFull: []string{"raw-a", "raw-b"},
		},
		{
			name:      "ambiguous stability fullName keeps generated ids",
			raw:       []map[string]any{raw("raw-only", "")},
			stability: []map[string]any{gen("case one", "gen-a"), gen("case two", "gen-b")},
			wantBatch: []string{"gen-a", "gen-b"}, wantFull: []string{"raw-only"},
		},
		{
			name:      "no raw results skips enrichment",
			stability: []map[string]any{gen("solo", "gen-only")}, wantBatch: []string{"gen-only"},
		},
		{
			name: "enrichment failure is non-fatal", raw: []map[string]any{raw(rawID, "")}, fullErr: errors.New("db unavailable"),
			stability: []map[string]any{gen("case", "gen-x")}, wantBatch: []string{rawID}, wantFull: []string{rawID},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			resultsDir := filepath.Join(tmpDir, "results", "batch1")
			if err := os.MkdirAll(resultsDir, 0o755); err != nil {
				t.Fatal(err)
			}
			for i, r := range tc.raw {
				data, _ := json.Marshal(r)
				if err := os.WriteFile(filepath.Join(resultsDir, string(rune('a'+i))+"-result.json"), data, 0o644); err != nil {
					t.Fatal(err)
				}
			}

			var batch []store.TestResult
			var full []*parser.Result
			var fullBuildID, fullProjectID int64
			mocks := testutil.New()
			mocks.TestResults.GetBuildIDFn = func(context.Context, int64, int) (int64, error) { return 42, nil }
			mocks.TestResults.InsertBatchFn = func(_ context.Context, rs []store.TestResult) error { batch = rs; return nil }
			mocks.TestResults.InsertBatchFullFn = func(_ context.Context, buildID, projectID int64, rs []*parser.Result) error {
				full, fullBuildID, fullProjectID = rs, buildID, projectID
				return tc.fullErr
			}
			a := &Allure{
				cfg: &config.Config{ProjectsPath: tmpDir}, store: stabilityStore(t, tc.stability),
				buildStore: mocks.Builds, testResultStore: mocks.TestResults, logger: zap.NewNop(),
			}

			if err := a.storeAndPruneBuild(context.Background(), 7, "proj", "proj", "batch1", tmpDir, 1, store.CIMetadata{}, nil); err != nil {
				t.Fatalf("storeAndPruneBuild: %v", err)
			}

			var gotBatch, gotFull []string
			for _, r := range batch {
				gotBatch = append(gotBatch, r.HistoryID)
				if r.Retries != tc.wantRetries {
					t.Errorf("stability row %q retries = %d, want %d", r.HistoryID, r.Retries, tc.wantRetries)
				}
			}
			for _, r := range full {
				gotFull = append(gotFull, r.HistoryID)
			}
			slices.Sort(gotBatch)
			slices.Sort(gotFull)
			if !slices.Equal(gotBatch, tc.wantBatch) {
				t.Errorf("InsertBatch history ids = %q, want %q", gotBatch, tc.wantBatch)
			}
			if !slices.Equal(gotFull, tc.wantFull) {
				t.Errorf("InsertBatchFull history ids = %q, want %q", gotFull, tc.wantFull)
			}
			if len(full) > 0 && (fullBuildID != 42 || fullProjectID != 7) {
				t.Errorf("InsertBatchFull(build %d, project %d), want (42, 7)", fullBuildID, fullProjectID)
			}
		})
	}
}
