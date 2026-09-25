package failure

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/llm"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// fakeLLM is a Summarizer double that counts calls and returns a canned result.
type fakeLLM struct {
	mu     sync.Mutex
	calls  int
	result llm.Summary
	err    error
}

func (f *fakeLLM) Summarize(_ context.Context, _ llm.Prompt) (llm.Summary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.result, f.err
}

func (f *fakeLLM) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// memSummaryStore is a stateful in-memory store.FailureSummaryStorer for the
// cache-behavior tests. It lets the test exercise the real hash comparison
// without knowing the hash the service computes.
type memSummaryStore struct {
	mu   sync.Mutex
	rows map[string]store.FailureSummary
}

var _ store.FailureSummaryStorer = (*memSummaryStore)(nil)

func newMemSummaryStore() *memSummaryStore {
	return &memSummaryStore{rows: map[string]store.FailureSummary{}}
}

func (m *memSummaryStore) Get(_ context.Context, _ int64, historyID string) (*store.FailureSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[historyID] // buildID is constant per test
	if !ok {
		return nil, nil
	}
	return &r, nil
}

func (m *memSummaryStore) Upsert(_ context.Context, s store.FailureSummary) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[s.HistoryID] = s
	return nil
}

// serviceFixture wires a Service against testutil doubles plus the given llm and
// summary store. By default the failed step path and error message are fixed
// and there is no last-good build or attachment, so evidence is deterministic;
// tweak adjusts the mocks.
func serviceFixture(t *testing.T, cfg config.LLMConfig, client Summarizer, summaries store.FailureSummaryStorer, tweak func(*testutil.MockStores)) *Service {
	t.Helper()
	mocks := testutil.New()
	branchID := int64(3)
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, id int64) (store.Build, error) {
		return store.Build{ID: id, BuildNumber: 28, BranchID: &branchID}, nil
	}
	mocks.TestResults.GetFailedStepPathFn = func(context.Context, int64, int64, string) ([]string, string, error) {
		return []string{"Test Body", "Call API"}, "status 500 from /users", nil
	}
	mocks.TestResults.GetLastPassingBuildFn = func(context.Context, int64, string, *int64, int) (*store.TestHistoryEntry, error) {
		return nil, nil
	}
	if tweak != nil {
		tweak(mocks)
	}
	return NewService(ServiceDeps{
		TestResults: mocks.TestResults, Attachments: mocks.Attachments, Builds: mocks.Builds,
		Summaries: summaries, LLM: client, Config: cfg, Logger: zap.NewNop(),
	})
}

func enabledCfg() config.LLMConfig {
	return config.LLMConfig{Enabled: true, Provider: "openai", Model: "llama3.1", BaseURL: "http://x/v1"}
}

// TestSummaryFor covers a single SummaryFor call. Generation failures are soft
// (Result.Err, never a hard error) and never cached. Two paths are regression
// guards: without objective failure evidence (a passing test or a bogus
// history_id) an authenticated viewer could otherwise trigger a paid LLM call
// and a junk row on every request; and an HTTP-200 blank hypothesis must not
// poison the cache forever.
func TestSummaryFor(t *testing.T) {
	noEvidence := func(m *testutil.MockStores) {
		m.TestResults.GetFailedStepPathFn = func(context.Context, int64, int64, string) ([]string, string, error) {
			return nil, "", nil
		}
	}
	tests := []struct {
		name        string
		cfg         config.LLMConfig
		llm         *fakeLLM
		tweak       func(*testutil.MockStores)
		wantEnabled bool
		wantHyp     string // "" = no summary
		wantSoftErr bool
		wantCalls   int
	}{
		{name: "disabled never calls the llm", cfg: config.LLMConfig{Enabled: false}, llm: &fakeLLM{}},
		{
			// A nil Evidence list is normalized to [] so the JSON shape matches a cache hit.
			name: "cache miss generates and persists", cfg: enabledCfg(),
			llm:         &fakeLLM{result: llm.Summary{Hypothesis: "prod bug", Category: "product_bug", Confidence: "medium"}},
			wantEnabled: true, wantHyp: "prod bug", wantCalls: 1,
		},
		{name: "llm error is soft", cfg: enabledCfg(), llm: &fakeLLM{err: errors.New("boom")}, wantEnabled: true, wantSoftErr: true, wantCalls: 1},
		{name: "no failure evidence: no llm call, no row", cfg: enabledCfg(), llm: &fakeLLM{}, tweak: noEvidence, wantEnabled: true, wantSoftErr: true},
		{
			name: "blank hypothesis is a soft error", cfg: enabledCfg(), llm: &fakeLLM{result: llm.Summary{Hypothesis: "   \n\t  ", Category: "flake"}},
			wantEnabled: true, wantSoftErr: true, wantCalls: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			summaries := newMemSummaryStore()
			svc := serviceFixture(t, tc.cfg, tc.llm, summaries, tc.tweak)

			res, err := svc.SummaryFor(context.Background(), 1, 100, "h1")
			if err != nil {
				t.Fatalf("SummaryFor must not hard-fail, got %v", err)
			}
			if res.Enabled != tc.wantEnabled || res.Cached || (res.Err != nil) != tc.wantSoftErr {
				t.Errorf("result = {Enabled: %v, Cached: %v, Err: %v}, want {Enabled: %v, Cached: false, soft error: %v}",
					res.Enabled, res.Cached, res.Err, tc.wantEnabled, tc.wantSoftErr)
			}
			if n := tc.llm.callCount(); n != tc.wantCalls {
				t.Errorf("llm calls: got %d, want %d", n, tc.wantCalls)
			}
			cached, _ := summaries.Get(context.Background(), 100, "h1")
			if tc.wantHyp == "" {
				if res.Summary != nil || cached != nil {
					t.Errorf("want no summary and no cached row, got summary %+v, row %+v", res.Summary, cached)
				}
				return
			}
			if res.Summary == nil || res.Summary.Hypothesis != tc.wantHyp || cached == nil {
				t.Fatalf("want a persisted summary %q, got %+v (row %+v)", tc.wantHyp, res.Summary, cached)
			}
			if res.Summary.Evidence == nil || len(res.Summary.Evidence) != 0 {
				t.Errorf("Evidence must be an empty, non-nil list, got %#v", res.Summary.Evidence)
			}
		})
	}
}

// TestSummaryFor_CacheLifecycle verifies a repeat request with identical
// evidence is served from cache — no LLM call and no recomputation of the
// whole-build last-good diff (it only enriches the prompt and is not part of
// input_hash) — while a stale input_hash regenerates.
func TestSummaryFor_CacheLifecycle(t *testing.T) {
	fake := &fakeLLM{result: llm.Summary{Hypothesis: "h", Category: "flake"}}
	summaries := newMemSummaryStore()
	compareCalls := 0
	svc := serviceFixture(t, enabledCfg(), fake, summaries, func(m *testutil.MockStores) {
		m.TestResults.GetLastPassingBuildFn = func(context.Context, int64, string, *int64, int) (*store.TestHistoryEntry, error) {
			return &store.TestHistoryEntry{BuildID: 80, BuildNumber: 25, Status: "passed"}, nil
		}
		m.TestResults.CompareBuildsByHistoryIDFn = func(context.Context, int64, int64, int64) ([]store.DiffEntry, error) {
			compareCalls++
			return nil, nil
		}
	})

	steps := []struct {
		name                  string
		staleHash, wantCached bool
		wantLLM, wantCompare  int
	}{
		{name: "miss generates and computes the diff once", wantLLM: 1, wantCompare: 1},
		{name: "hit skips the llm and the diff", wantCached: true, wantLLM: 1, wantCompare: 1},
		{name: "stale input_hash regenerates", staleHash: true, wantLLM: 2, wantCompare: 2},
	}
	for _, s := range steps {
		if s.staleHash {
			summaries.mu.Lock()
			row := summaries.rows["h1"]
			row.InputHash = "stale-hash-does-not-match"
			summaries.rows["h1"] = row
			summaries.mu.Unlock()
		}
		res, err := svc.SummaryFor(context.Background(), 1, 100, "h1")
		if err != nil {
			t.Fatalf("%s: SummaryFor: %v", s.name, err)
		}
		if res.Cached != s.wantCached || fake.callCount() != s.wantLLM || compareCalls != s.wantCompare {
			t.Errorf("%s: cached=%v llm=%d diff=%d, want cached=%v llm=%d diff=%d",
				s.name, res.Cached, fake.callCount(), compareCalls, s.wantCached, s.wantLLM, s.wantCompare)
		}
	}
}
