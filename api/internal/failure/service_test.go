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

// fakeLLM is a Summarizer double that records calls and can be made to fail.
type fakeLLM struct {
	mu           sync.Mutex
	calls        int
	result       llm.Summary
	err          error
	failIfCalled *testing.T // when set, the test fails if Summarize is invoked
}

func (f *fakeLLM) Summarize(_ context.Context, _ llm.Prompt) (llm.Summary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failIfCalled != nil {
		f.failIfCalled.Errorf("llm.Summarize must not be called on this path")
	}
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

func (m *memSummaryStore) key(buildID int64, historyID string) string {
	return historyID // buildID is constant per test
}

func (m *memSummaryStore) Get(_ context.Context, buildID int64, historyID string) (*store.FailureSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[m.key(buildID, historyID)]
	if !ok {
		return nil, nil
	}
	cp := r
	return &cp, nil
}

func (m *memSummaryStore) Upsert(_ context.Context, s store.FailureSummary) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[m.key(s.BuildID, s.HistoryID)] = s
	return nil
}

// serviceFixture wires a Service against testutil doubles plus the given llm and
// summary store. The build+step-path mocks make evidence deterministic.
func serviceFixture(t *testing.T, cfg config.LLMConfig, client Summarizer, summaries store.FailureSummaryStorer) *Service {
	t.Helper()
	mocks := testutil.New()
	branchID := int64(3)
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, id int64) (store.Build, error) {
		bn := 28
		return store.Build{ID: id, BuildNumber: bn, BranchID: &branchID}, nil
	}
	mocks.TestResults.GetFailedStepPathFn = func(_ context.Context, _ int64, _ int64, _ string) ([]string, string, error) {
		return []string{"Test Body", "Call API"}, "status 500 from /users", nil
	}
	// No last-good, no attachments → evidence stays deterministic.
	mocks.TestResults.GetLastPassingBuildFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) (*store.TestHistoryEntry, error) {
		return nil, nil
	}
	return NewService(ServiceDeps{
		TestResults: mocks.TestResults,
		Attachments: mocks.Attachments,
		Builds:      mocks.Builds,
		Summaries:   summaries,
		Blobs:       nil,
		LLM:         client,
		Config:      cfg,
		Logger:      zap.NewNop(),
	})
}

func enabledCfg() config.LLMConfig {
	return config.LLMConfig{Enabled: true, Provider: "openai", Model: "llama3.1", BaseURL: "http://x/v1"}
}

func TestSummaryFor_Disabled(t *testing.T) {
	fake := &fakeLLM{failIfCalled: t}
	svc := serviceFixture(t, config.LLMConfig{Enabled: false}, fake, newMemSummaryStore())

	res, err := svc.SummaryFor(context.Background(), 1, 100, "h1")
	if err != nil {
		t.Fatalf("SummaryFor: %v", err)
	}
	if res.Enabled {
		t.Errorf("disabled config must yield Enabled=false, got %+v", res)
	}
	if res.Summary != nil {
		t.Errorf("disabled path must not produce a summary, got %+v", res.Summary)
	}
	if fake.callCount() != 0 {
		t.Errorf("disabled path must never call the llm, got %d calls", fake.callCount())
	}
}

func TestSummaryFor_CacheMissGeneratesAndUpserts(t *testing.T) {
	fake := &fakeLLM{result: llm.Summary{Hypothesis: "prod bug", Category: "product_bug", Confidence: "medium", Evidence: []string{"e1"}}}
	summaries := newMemSummaryStore()
	svc := serviceFixture(t, enabledCfg(), fake, summaries)

	res, err := svc.SummaryFor(context.Background(), 1, 100, "h1")
	if err != nil {
		t.Fatalf("SummaryFor: %v", err)
	}
	if !res.Enabled || res.Cached {
		t.Errorf("first call must be Enabled and not Cached, got %+v", res)
	}
	if res.Summary == nil || res.Summary.Hypothesis != "prod bug" {
		t.Errorf("summary: got %+v", res.Summary)
	}
	if fake.callCount() != 1 {
		t.Errorf("llm calls: got %d, want 1", fake.callCount())
	}
	// The generated row must have been persisted.
	if got, _ := summaries.Get(context.Background(), 100, "h1"); got == nil {
		t.Error("cache miss must upsert the generated summary")
	}
}

func TestSummaryFor_CacheHitSkipsLLM(t *testing.T) {
	fake := &fakeLLM{result: llm.Summary{Hypothesis: "prod bug", Category: "product_bug"}}
	summaries := newMemSummaryStore()
	svc := serviceFixture(t, enabledCfg(), fake, summaries)
	ctx := context.Background()

	// First call generates + caches.
	if _, err := svc.SummaryFor(ctx, 1, 100, "h1"); err != nil {
		t.Fatalf("first SummaryFor: %v", err)
	}
	// Second call with identical evidence must hit the cache (no new llm call).
	res, err := svc.SummaryFor(ctx, 1, 100, "h1")
	if err != nil {
		t.Fatalf("second SummaryFor: %v", err)
	}
	if !res.Cached {
		t.Errorf("second call must be Cached, got %+v", res)
	}
	if fake.callCount() != 1 {
		t.Errorf("llm calls: got %d, want 1 (cache must serve the repeat)", fake.callCount())
	}
}

func TestSummaryFor_StaleHashRegenerates(t *testing.T) {
	fake := &fakeLLM{result: llm.Summary{Hypothesis: "v1", Category: "flake"}}
	summaries := newMemSummaryStore()
	svc := serviceFixture(t, enabledCfg(), fake, summaries)
	ctx := context.Background()

	if _, err := svc.SummaryFor(ctx, 1, 100, "h1"); err != nil {
		t.Fatalf("first SummaryFor: %v", err)
	}
	// Corrupt the cached input_hash so the next call sees a stale entry.
	summaries.mu.Lock()
	row := summaries.rows["h1"]
	row.InputHash = "stale-hash-does-not-match"
	summaries.rows["h1"] = row
	summaries.mu.Unlock()

	if _, err := svc.SummaryFor(ctx, 1, 100, "h1"); err != nil {
		t.Fatalf("second SummaryFor: %v", err)
	}
	if fake.callCount() != 2 {
		t.Errorf("stale hash must regenerate: llm calls got %d, want 2", fake.callCount())
	}
}

func TestSummaryFor_LLMErrorIsSoft(t *testing.T) {
	fake := &fakeLLM{err: errors.New("boom")}
	svc := serviceFixture(t, enabledCfg(), fake, newMemSummaryStore())

	res, err := svc.SummaryFor(context.Background(), 1, 100, "h1")
	if err != nil {
		t.Fatalf("LLM failure must not be a hard error, got %v", err)
	}
	if !res.Enabled {
		t.Errorf("Enabled must remain true on soft error, got %+v", res)
	}
	if res.Summary != nil {
		t.Errorf("no summary on generation failure, got %+v", res.Summary)
	}
	if res.Err == nil {
		t.Error("soft generation error must be surfaced in Result.Err")
	}
}

// TestSummaryFor_NoEvidenceGate_NoLLMCallNoUpsert is a regression test for a
// paid-LLM-call abuse vector: an authenticated viewer could otherwise request
// a failure-summary for ANY (build_id, history_id) pair — including a test
// that never failed, or a bogus history_id — triggering a real (paid) LLM
// call and a junk failure_summaries row on every single request, with no
// gate and no rate limit. The service must refuse to generate — and must
// never touch the LLM client or the store's Upsert — when assembleEvidence
// finds no objective failure evidence (no error message, no failed-step
// path).
func TestSummaryFor_NoEvidenceGate_NoLLMCallNoUpsert(t *testing.T) {
	fake := &fakeLLM{failIfCalled: t}
	summaries := newMemSummaryStore()

	mocks := testutil.New()
	branchID := int64(3)
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, id int64) (store.Build, error) {
		return store.Build{ID: id, BuildNumber: 28, BranchID: &branchID}, nil
	}
	// No error message, no failed-step path: this test has no failure
	// evidence (e.g. it currently passes, or history_id is bogus/unrelated).
	mocks.TestResults.GetFailedStepPathFn = func(_ context.Context, _ int64, _ int64, _ string) ([]string, string, error) {
		return nil, "", nil
	}
	mocks.TestResults.GetLastPassingBuildFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) (*store.TestHistoryEntry, error) {
		return nil, nil
	}

	svc := NewService(ServiceDeps{
		TestResults: mocks.TestResults,
		Attachments: mocks.Attachments,
		Builds:      mocks.Builds,
		Summaries:   summaries,
		LLM:         fake,
		Config:      enabledCfg(),
		Logger:      zap.NewNop(),
	})

	res, err := svc.SummaryFor(context.Background(), 1, 100, "h-no-evidence")
	if err != nil {
		t.Fatalf("SummaryFor: %v", err)
	}
	if !res.Enabled {
		t.Errorf("Enabled must stay true (the feature itself is on), got %+v", res)
	}
	if res.Summary != nil {
		t.Errorf("no summary may be produced without failure evidence, got %+v", res.Summary)
	}
	if res.Err == nil {
		t.Error("want a soft error explaining no evidence was found")
	}
	if fake.callCount() != 0 {
		t.Errorf("llm must never be called without failure evidence: got %d calls, want 0", fake.callCount())
	}
	if got, _ := summaries.Get(context.Background(), 100, "h-no-evidence"); got != nil {
		t.Errorf("no row may be upserted without failure evidence, got %+v", got)
	}
}

// TestSummaryFor_BlankHypothesis_NotCached is a regression test: an LLM that
// returns HTTP 200 with an empty/whitespace-only hypothesis must not poison
// the cache. It must be treated as a soft generation error so the next
// request retries instead of serving (and re-serving forever) a blank
// summary.
func TestSummaryFor_BlankHypothesis_NotCached(t *testing.T) {
	fake := &fakeLLM{result: llm.Summary{Hypothesis: "   \n\t  ", Category: "flake"}}
	summaries := newMemSummaryStore()
	svc := serviceFixture(t, enabledCfg(), fake, summaries)

	res, err := svc.SummaryFor(context.Background(), 1, 100, "h1")
	if err != nil {
		t.Fatalf("SummaryFor: %v", err)
	}
	if res.Summary != nil {
		t.Errorf("blank hypothesis must not be surfaced as a summary, got %+v", res.Summary)
	}
	if res.Err == nil {
		t.Error("blank hypothesis must be reported as a soft error")
	}
	if got, _ := summaries.Get(context.Background(), 100, "h1"); got != nil {
		t.Errorf("blank hypothesis must not be cached, got %+v", got)
	}
	if fake.callCount() != 1 {
		t.Errorf("llm calls: got %d, want 1", fake.callCount())
	}
}

// TestSummaryFor_FreshEvidenceNeverNil verifies a freshly generated summary
// normalizes a nil Evidence slice to an empty (non-nil) one, so the JSON shape
// (`[]`, never `null`) matches what a subsequent cache-hit read would produce.
func TestSummaryFor_FreshEvidenceNeverNil(t *testing.T) {
	fake := &fakeLLM{result: llm.Summary{Hypothesis: "no bullets here", Category: "flake", Evidence: nil}}
	svc := serviceFixture(t, enabledCfg(), fake, newMemSummaryStore())

	res, err := svc.SummaryFor(context.Background(), 1, 100, "h1")
	if err != nil {
		t.Fatalf("SummaryFor: %v", err)
	}
	if res.Summary == nil {
		t.Fatal("expected a summary")
	}
	if res.Summary.Evidence == nil {
		t.Error("Evidence must never be nil on the fresh-generation path")
	}
	if len(res.Summary.Evidence) != 0 {
		t.Errorf("Evidence: got %v, want empty", res.Summary.Evidence)
	}
}

// TestSummaryFor_CacheHit_SkipsDiffComparison verifies the expensive
// whole-build last-good→current diff (CompareBuildsByHistoryID) is computed
// at most once, on the cache-MISS generation path, and never re-run on a
// cache hit. The diff only enriches the prompt and is not part of
// input_hash, so running it on every request (including cache hits) would be
// pure waste.
func TestSummaryFor_CacheHit_SkipsDiffComparison(t *testing.T) {
	fake := &fakeLLM{result: llm.Summary{Hypothesis: "h", Category: "flake"}}
	summaries := newMemSummaryStore()

	mocks := testutil.New()
	branchID := int64(3)
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, id int64) (store.Build, error) {
		return store.Build{ID: id, BuildNumber: 28, BranchID: &branchID}, nil
	}
	mocks.TestResults.GetFailedStepPathFn = func(_ context.Context, _ int64, _ int64, _ string) ([]string, string, error) {
		return []string{"Test Body"}, "boom", nil
	}
	mocks.TestResults.GetLastPassingBuildFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) (*store.TestHistoryEntry, error) {
		return &store.TestHistoryEntry{BuildID: 80, BuildNumber: 25, Status: "passed"}, nil
	}
	compareCalls := 0
	mocks.TestResults.CompareBuildsByHistoryIDFn = func(_ context.Context, _ int64, _, _ int64) ([]store.DiffEntry, error) {
		compareCalls++
		return nil, nil
	}

	svc := NewService(ServiceDeps{
		TestResults: mocks.TestResults, Attachments: mocks.Attachments, Builds: mocks.Builds,
		Summaries: summaries, LLM: fake, Config: enabledCfg(), Logger: zap.NewNop(),
	})

	ctx := context.Background()
	if _, err := svc.SummaryFor(ctx, 1, 100, "h1"); err != nil {
		t.Fatalf("first SummaryFor: %v", err)
	}
	if compareCalls != 1 {
		t.Errorf("cache-miss generation must compute the diff exactly once: got %d calls, want 1", compareCalls)
	}
	if _, err := svc.SummaryFor(ctx, 1, 100, "h1"); err != nil {
		t.Fatalf("second SummaryFor: %v", err)
	}
	if compareCalls != 1 {
		t.Errorf("cache hit must not recompute the diff: got %d calls, want still 1", compareCalls)
	}
	if fake.callCount() != 1 {
		t.Errorf("llm calls: got %d, want 1 (second call must be served from cache)", fake.callCount())
	}
}
