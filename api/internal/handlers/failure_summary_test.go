package handlers

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/failure"
	"github.com/mkutlak/alluredeck/api/internal/llm"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestFailureSummary(t *testing.T) {
	enabled := config.LLMConfig{Enabled: true, Provider: "openai", Model: "llama3.1", BaseURL: "http://x/v1"}
	summary := llm.Summary{Hypothesis: "The product returned 500.", Category: "product_bug", Confidence: "medium", Evidence: []string{"status 500 from /users"}}
	tests := []struct {
		name      string
		cfg       config.LLMConfig
		client    *testutil.StubSummarizer // nil = a stub returning a zero summary
		buildID   string
		historyID string
		want      int
		wantJSON  map[string]any
	}{
		{name: "disabled", buildID: "100", historyID: "h1", want: http.StatusOK, wantJSON: map[string]any{
			"data.enabled": false, "data.summary": nil, "metadata.message": "LLM summaries are disabled",
		}},
		// The test never passed before, so last_good is omitted.
		{name: "generated", cfg: enabled, client: &testutil.StubSummarizer{Result: summary}, buildID: "100", historyID: "h1", want: http.StatusOK, wantJSON: map[string]any{
			"data.enabled": true, "data.build_id": 100, "data.history_id": "h1", "data.model": "llama3.1", "data.disclaimer": aiDisclaimer,
			"data.summary.category": "product_bug", "data.summary.hypothesis": summary.Hypothesis, "data.summary.evidence#": 1, "data.last_good": nil,
		}},
		// A generation failure is soft: never a 5xx, summary null, error set.
		{name: "llm error", cfg: enabled, client: &testutil.StubSummarizer{Err: errors.New("upstream down")}, buildID: "100", historyID: "h1", want: http.StatusOK, wantJSON: map[string]any{
			"data.enabled": true, "data.summary": nil, "data.error": "generation failed",
		}},
		{name: "bad build id", cfg: enabled, buildID: "not-a-number", historyID: "h1", want: http.StatusBadRequest},
		{name: "missing history id", cfg: enabled, buildID: "100", want: http.StatusBadRequest},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mocks := testutil.New()
			mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, id int64) (store.Build, error) {
				if id != 100 {
					return store.Build{}, store.ErrBuildNotFound
				}
				return store.Build{ID: id, BuildNumber: 28, BranchID: new(int64(3))}, nil
			}
			mocks.TestResults.GetFailedStepPathFn = func(context.Context, int64, int64, string) ([]string, string, error) {
				return []string{"Test Body"}, "boom", nil
			}
			mocks.TestResults.GetLastPassingBuildFn = func(context.Context, int64, string, *int64, int) (*store.TestHistoryEntry, error) {
				return nil, nil
			}
			svc := failure.NewService(failure.ServiceDeps{
				TestResults: mocks.TestResults, Attachments: mocks.Attachments, Builds: mocks.Builds,
				Summaries: mocks.FailureSummaries, LLM: cmp.Or(tc.client, &testutil.StubSummarizer{}), Config: tc.cfg, Logger: zap.NewNop(),
			})
			h := NewFailureSummaryHandler(svc, mocks.Projects, zap.NewNop())
			code, body := serveJSON(t, h.GetFailureSummary, http.MethodGet, "/api/v1/projects/1/builds/x/tests/x/failure-summary", "",
				"project_id", "1", "build_id", tc.buildID, "history_id", tc.historyID)
			if code != tc.want {
				t.Fatalf("status = %d, want %d: %v", code, tc.want, body)
			}
			wantJSON(t, body, tc.wantJSON)
		})
	}
}
