package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/failure"
	"github.com/mkutlak/alluredeck/api/internal/llm"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// stubSummarizer is a failure.Summarizer double for handler tests.
type stubSummarizer struct {
	result llm.Summary
	err    error
}

func (s stubSummarizer) Summarize(_ context.Context, _ llm.Prompt) (llm.Summary, error) {
	return s.result, s.err
}

// newFailureSummaryHandler wires a handler around a real failure.Service backed
// by testutil doubles. cfg controls the enabled/disabled path; client provides
// the canned generation.
func newFailureSummaryHandler(t *testing.T, cfg config.LLMConfig, client failure.Summarizer) *FailureSummaryHandler {
	t.Helper()
	mocks := testutil.New()
	branchID := int64(3)
	mocks.Builds.GetBuildByIDFn = func(_ context.Context, _, id int64) (store.Build, error) {
		if id != 100 {
			return store.Build{}, store.ErrBuildNotFound
		}
		return store.Build{ID: id, BuildNumber: 28, BranchID: &branchID}, nil
	}
	mocks.TestResults.GetFailedStepPathFn = func(_ context.Context, _ int64, _ int64, _ string) ([]string, string, error) {
		return []string{"Test Body"}, "boom", nil
	}
	mocks.TestResults.GetLastPassingBuildFn = func(_ context.Context, _ int64, _ string, _ *int64, _ int) (*store.TestHistoryEntry, error) {
		return nil, nil
	}
	svc := failure.NewService(failure.ServiceDeps{
		TestResults: mocks.TestResults,
		Attachments: mocks.Attachments,
		Builds:      mocks.Builds,
		Summaries:   mocks.FailureSummaries,
		Blobs:       nil,
		LLM:         client,
		Config:      cfg,
		Logger:      zap.NewNop(),
	})
	return NewFailureSummaryHandler(svc, mocks.Projects, zap.NewNop())
}

func doFailureSummary(t *testing.T, h *FailureSummaryHandler, projectID, buildID, historyID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet,
		"/projects/"+projectID+"/builds/"+buildID+"/tests/"+historyID+"/failure-summary", nil)
	req.SetPathValue("project_id", projectID)
	req.SetPathValue("build_id", buildID)
	req.SetPathValue("history_id", historyID)
	rr := httptest.NewRecorder()
	h.GetFailureSummary(rr, req)
	return rr
}

// decodedFailureSummary mirrors the frozen Seam 5 response for decoding.
type decodedFailureSummary struct {
	Data struct {
		Enabled     bool   `json:"enabled"`
		Cached      bool   `json:"cached"`
		BuildID     int64  `json:"build_id"`
		HistoryID   string `json:"history_id"`
		Model       string `json:"model"`
		GeneratedAt string `json:"generated_at"`
		Disclaimer  string `json:"disclaimer"`
		Error       string `json:"error"`
		Summary     *struct {
			Hypothesis string   `json:"hypothesis"`
			Category   string   `json:"category"`
			Confidence string   `json:"confidence"`
			Evidence   []string `json:"evidence"`
		} `json:"summary"`
		LastGood *struct {
			BuildNumber int    `json:"build_number"`
			CommitSHA   string `json:"commit_sha"`
			BuildsSince int    `json:"builds_since"`
		} `json:"last_good"`
	} `json:"data"`
	Metadata struct {
		Message string `json:"message"`
	} `json:"metadata"`
}

func TestFailureSummary_Disabled(t *testing.T) {
	h := newFailureSummaryHandler(t, config.LLMConfig{Enabled: false}, stubSummarizer{})
	rr := doFailureSummary(t, h, "1", "100", "h1")

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	var env decodedFailureSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Data.Enabled {
		t.Errorf("disabled response must have enabled=false, got %s", rr.Body.String())
	}
	if env.Data.Summary != nil {
		t.Errorf("disabled response must not include a summary: %s", rr.Body.String())
	}
	if env.Metadata.Message != "LLM summaries are disabled" {
		t.Errorf("message: got %q", env.Metadata.Message)
	}
}

func TestFailureSummary_EnabledSuccess(t *testing.T) {
	client := stubSummarizer{result: llm.Summary{
		Hypothesis: "The product returned 500.", Category: "product_bug",
		Confidence: "medium", Evidence: []string{"status 500 from /users"},
	}}
	h := newFailureSummaryHandler(t, config.LLMConfig{Enabled: true, Provider: "openai", Model: "llama3.1", BaseURL: "http://x/v1"}, client)
	rr := doFailureSummary(t, h, "1", "100", "h1")

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200", rr.Code)
	}
	var env decodedFailureSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	d := env.Data
	if !d.Enabled {
		t.Error("enabled must be true")
	}
	if d.BuildID != 100 || d.HistoryID != "h1" {
		t.Errorf("build_id/history_id: got %d/%q", d.BuildID, d.HistoryID)
	}
	if d.Model != "llama3.1" {
		t.Errorf("model: got %q", d.Model)
	}
	if d.Disclaimer == "" {
		t.Error("disclaimer must be present")
	}
	if d.Summary == nil || d.Summary.Category != "product_bug" || d.Summary.Hypothesis == "" {
		t.Errorf("summary: got %+v", d.Summary)
	}
	if len(d.Summary.Evidence) != 1 {
		t.Errorf("evidence: got %v", d.Summary.Evidence)
	}
	// The test never previously passed → last_good omitted.
	if d.LastGood != nil {
		t.Errorf("last_good must be omitted when no prior pass, got %+v", d.LastGood)
	}
}

func TestFailureSummary_LLMErrorSoft(t *testing.T) {
	client := stubSummarizer{err: errors.New("upstream down")}
	h := newFailureSummaryHandler(t, config.LLMConfig{Enabled: true, Provider: "openai", Model: "m", BaseURL: "http://x/v1"}, client)
	rr := doFailureSummary(t, h, "1", "100", "h1")

	if rr.Code != http.StatusOK {
		t.Fatalf("LLM failure must not be a 5xx: got %d", rr.Code)
	}
	var env decodedFailureSummary
	if err := json.Unmarshal(rr.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.Data.Enabled {
		t.Error("enabled must stay true on soft error")
	}
	if env.Data.Summary != nil {
		t.Errorf("summary must be null on generation error, got %+v", env.Data.Summary)
	}
	if env.Data.Error == "" {
		t.Error("data.error must be set on generation failure")
	}
}

func TestFailureSummary_BadBuildID(t *testing.T) {
	h := newFailureSummaryHandler(t, config.LLMConfig{Enabled: true, Provider: "openai", Model: "m", BaseURL: "http://x/v1"}, stubSummarizer{})
	rr := doFailureSummary(t, h, "1", "not-a-number", "h1")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("bad build_id must be 400, got %d", rr.Code)
	}
}

func TestFailureSummary_MissingHistoryID(t *testing.T) {
	h := newFailureSummaryHandler(t, config.LLMConfig{Enabled: true, Provider: "openai", Model: "m", BaseURL: "http://x/v1"}, stubSummarizer{})
	rr := doFailureSummary(t, h, "1", "100", "")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("missing history_id must be 400, got %d", rr.Code)
	}
}
