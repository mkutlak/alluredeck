package failure

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/llm"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

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
		llm         *testutil.StubSummarizer
		tweak       func(*testutil.MockStores)
		wantEnabled bool
		wantHyp     string // "" = no summary
		wantSoftErr bool
		wantCalls   int
	}{
		{name: "disabled never calls the llm", cfg: config.LLMConfig{Enabled: false}, llm: &testutil.StubSummarizer{}},
		{
			// A nil Evidence list is normalized to [] so the JSON shape matches a cache hit.
			name: "cache miss generates and persists", cfg: enabledCfg(),
			llm:         &testutil.StubSummarizer{Result: llm.Summary{Hypothesis: "prod bug", Category: "product_bug", Confidence: "medium"}},
			wantEnabled: true, wantHyp: "prod bug", wantCalls: 1,
		},
		{name: "llm error is soft", cfg: enabledCfg(), llm: &testutil.StubSummarizer{Err: errors.New("boom")}, wantEnabled: true, wantSoftErr: true, wantCalls: 1},
		{name: "no failure evidence: no llm call, no row", cfg: enabledCfg(), llm: &testutil.StubSummarizer{}, tweak: noEvidence, wantEnabled: true, wantSoftErr: true},
		{
			name: "blank hypothesis is a soft error", cfg: enabledCfg(), llm: &testutil.StubSummarizer{Result: llm.Summary{Hypothesis: "   \n\t  ", Category: "flake"}},
			wantEnabled: true, wantSoftErr: true, wantCalls: 1,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			summaries := testutil.NewMemFailureSummaryStore()
			svc := serviceFixture(t, tc.cfg, tc.llm, summaries, tc.tweak)

			res, err := svc.SummaryFor(context.Background(), 1, 100, "h1")
			if err != nil {
				t.Fatalf("SummaryFor must not hard-fail, got %v", err)
			}
			if res.Enabled != tc.wantEnabled || res.Cached || (res.Err != nil) != tc.wantSoftErr {
				t.Errorf("result = {Enabled: %v, Cached: %v, Err: %v}, want {Enabled: %v, Cached: false, soft error: %v}",
					res.Enabled, res.Cached, res.Err, tc.wantEnabled, tc.wantSoftErr)
			}
			if n := tc.llm.Calls(); n != tc.wantCalls {
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
	fake := &testutil.StubSummarizer{Result: llm.Summary{Hypothesis: "h", Category: "flake"}}
	summaries := testutil.NewMemFailureSummaryStore()
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
			row, _ := summaries.Get(context.Background(), 100, "h1")
			row.InputHash = "stale-hash-does-not-match"
			if err := summaries.Upsert(context.Background(), *row); err != nil {
				t.Fatal(err)
			}
		}
		res, err := svc.SummaryFor(context.Background(), 1, 100, "h1")
		if err != nil {
			t.Fatalf("%s: SummaryFor: %v", s.name, err)
		}
		if res.Cached != s.wantCached || fake.Calls() != s.wantLLM || compareCalls != s.wantCompare {
			t.Errorf("%s: cached=%v llm=%d diff=%d, want cached=%v llm=%d diff=%d",
				s.name, res.Cached, fake.Calls(), compareCalls, s.wantCached, s.wantLLM, s.wantCompare)
		}
	}
}
