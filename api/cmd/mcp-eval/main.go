// mcp-eval is the Phase 2.5 gate for the alluredeck MCP server.
// It runs a curated set of test failures against the MCP server + Anthropic API
// and asserts the LLM-assigned category matches ground truth at or above a
// configurable threshold.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	anthropicURL     = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"
	exitOK           = 0
	exitBelowThresh  = 1
	exitMCPDown      = 2
	exitMCPAuth      = 3
	exitLLMAuth      = 4
	exitToolMissing  = 5
	exitBadFixture   = 6
	exitStructural   = 7
	exitUsage        = 64
)

type GroundTruth struct {
	HistoryID        string `json:"history_id"`
	ProjectID        int    `json:"project_id"`
	BuildID          int    `json:"build_id"`
	FullName         string `json:"full_name"`
	ExpectedCategory string `json:"expected_category"`
	Notes            string `json:"notes,omitempty"`
	// MaxDiagnoseBytes caps the marshaled size of the diagnose_failure
	// response for this fixture. Zero means the default (100000) applies.
	MaxDiagnoseBytes int `json:"max_diagnose_bytes,omitempty"`
}

type Result struct {
	HistoryID string `json:"history_id"`
	Expected  string `json:"expected"`
	Predicted string `json:"predicted"`
	Score     int    `json:"score"`
	MCPCallMs int64  `json:"mcp_call_ms"`
	LLMCallMs int64  `json:"llm_call_ms"`
	// DiagnoseCallMs is the wall time of the diagnose_failure tool call.
	DiagnoseCallMs int64 `json:"diagnose_call_ms"`
	// DiagnoseBytes is the size of the diagnose_failure structured content,
	// re-marshaled to JSON. It is the regression guard against payloads that
	// silently balloon (e.g. duplicate rows doubling every entry).
	DiagnoseBytes int `json:"diagnose_bytes"`
	// StructuralErrors lists every structural assertion that failed against
	// the diagnose_failure response. Empty means the response passed all
	// structural checks. These are pure shape/size checks, never LLM-scored.
	StructuralErrors []string `json:"structural_errors,omitempty"`
	// LLMInputTokens / LLMOutputTokens are the usage block from the
	// Anthropic classify call. Zero when -skip-llm is set.
	LLMInputTokens  int    `json:"llm_input_tokens,omitempty"`
	LLMOutputTokens int    `json:"llm_output_tokens,omitempty"`
	Error           string `json:"error,omitempty"`
}

type Report struct {
	Model     string  `json:"model"`
	Threshold float64 `json:"threshold"`
	SkipLLM   bool    `json:"skip_llm,omitempty"`
	Total     int     `json:"total"`
	Correct   int     `json:"correct"`
	Accuracy  float64 `json:"accuracy"`
	// StructuralFailures is the count of fixtures whose diagnose_failure
	// response failed at least one structural assertion.
	StructuralFailures int `json:"structural_failures"`
	// TotalDiagnoseBytes is the sum of DiagnoseBytes across all fixtures.
	TotalDiagnoseBytes int `json:"total_diagnose_bytes"`
	// TotalLLMInputTokens / TotalLLMOutputTokens sum the Anthropic usage
	// block across all fixtures. Zero when -skip-llm is set.
	TotalLLMInputTokens  int      `json:"total_llm_input_tokens"`
	TotalLLMOutputTokens int      `json:"total_llm_output_tokens"`
	Results              []Result `json:"results"`
}

type bearerTransport struct {
	token string
	base  http.RoundTripper
}

func (t *bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	r.Header.Set("Authorization", "Bearer "+t.token)
	return t.base.RoundTrip(r)
}

type anthropicReq struct {
	Model     string `json:"model"`
	MaxTokens int    `json:"max_tokens"`
	// Temperature is pinned to 0 for every classify call so runs are
	// reproducible; the Messages API defaults to 1.0 when omitted, so this
	// is sent explicitly rather than relying on omitempty.
	Temperature float64            `json:"temperature"`
	Messages    []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicUsage is the Messages API usage block: token counts for the
// request/response pair, surfaced per fixture and totaled in the report.
type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicResp struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Usage anthropicUsage `json:"usage"`
}

func main() {
	os.Exit(run())
}

func run() int {
	serverURL := flag.String("server-url", "http://localhost:8081/mcp", "MCP server endpoint")
	token := flag.String("token", "", "Bearer token for MCP authentication (required)")
	fixtures := flag.String("fixtures", "e2e/fixtures/mcp-eval/ground-truth.json", "ground-truth JSON path")
	output := flag.String("output", "eval-report.json", "report output path")
	model := flag.String("model", "claude-sonnet-4-6", "Anthropic model id")
	threshold := flag.Float64("threshold", 0.7, "accuracy threshold for exit 0")
	skipLLM := flag.Bool("skip-llm", false, "run only structural assertions against diagnose_failure; no LLM calls, no ANTHROPIC_API_KEY required")
	flag.Parse()

	var apiKey string
	if !*skipLLM {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
		if apiKey == "" {
			fmt.Fprintln(os.Stderr, "ANTHROPIC_API_KEY env var is required (use -skip-llm to run structural checks only)")
			return exitUsage
		}
	}
	if *token == "" {
		fmt.Fprintln(os.Stderr, "--token is required")
		return exitUsage
	}

	entries, err := loadFixture(*fixtures)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load fixture: %v\n", err)
		return exitBadFixture
	}
	// An empty fixture array is a broken fixture, not a clean run. Without
	// this, -skip-llm over `[]` asserts nothing and still exits 0, so a CI gate
	// wired to a mistyped or emptied fixture path reports green forever.
	if len(entries) == 0 {
		fmt.Fprintf(os.Stderr, "fixture %s contains no entries\n", *fixtures)
		return exitBadFixture
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	cs, err := connectMCP(ctx, *serverURL, *token)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect mcp: %v\n", err)
		if strings.Contains(err.Error(), "401") {
			return exitMCPAuth
		}
		return exitMCPDown
	}
	defer func() { _ = cs.Close() }()

	llmClient := &http.Client{Timeout: 60 * time.Second}

	report := Report{Model: *model, Threshold: *threshold, SkipLLM: *skipLLM, Total: len(entries)}

	for i, e := range entries {
		fmt.Printf("[%d/%d] history_id=%s expected=%s ... ", i+1, len(entries), e.HistoryID, e.ExpectedCategory)
		res := evalOne(ctx, cs, llmClient, apiKey, *model, e, *skipLLM)
		report.Results = append(report.Results, res)
		if res.Score == 1 {
			report.Correct++
		}
		if len(res.StructuralErrors) > 0 {
			report.StructuralFailures++
		}
		report.TotalDiagnoseBytes += res.DiagnoseBytes
		report.TotalLLMInputTokens += res.LLMInputTokens
		report.TotalLLMOutputTokens += res.LLMOutputTokens
		switch {
		case res.Error != "":
			fmt.Printf("error: %s\n", res.Error)
			if strings.Contains(res.Error, "tool not found") {
				_ = writeReport(*output, &report)
				return exitToolMissing
			}
			if strings.Contains(res.Error, "llm auth") {
				_ = writeReport(*output, &report)
				return exitLLMAuth
			}
		case len(res.StructuralErrors) > 0:
			fmt.Printf("STRUCTURAL FAILURE: %s\n", strings.Join(res.StructuralErrors, "; "))
		case *skipLLM:
			fmt.Printf("structural OK (diagnose=%dms, %d bytes)\n", res.DiagnoseCallMs, res.DiagnoseBytes)
		default:
			fmt.Printf("predicted=%s score=%d (mcp=%dms diagnose=%dms llm=%dms)\n", res.Predicted, res.Score, res.MCPCallMs, res.DiagnoseCallMs, res.LLMCallMs)
		}
	}

	if report.Total > 0 {
		report.Accuracy = float64(report.Correct) / float64(report.Total)
	}
	if err := writeReport(*output, &report); err != nil {
		fmt.Fprintf(os.Stderr, "write report: %v\n", err)
	}

	if *skipLLM {
		fmt.Printf("\nStructural-only mode: %d/%d fixtures passed, %d total diagnose bytes\n",
			report.Total-report.StructuralFailures, report.Total, report.TotalDiagnoseBytes)
	} else {
		fmt.Printf("\nAccuracy: %d/%d = %.2f%% (threshold %.0f%%)\n", report.Correct, report.Total, report.Accuracy*100, *threshold*100)
	}
	if report.StructuralFailures > 0 {
		fmt.Printf("Structural failures: %d/%d fixtures\n", report.StructuralFailures, report.Total)
		return exitStructural
	}
	if *skipLLM {
		return exitOK
	}
	if report.Accuracy < *threshold {
		return exitBelowThresh
	}
	return exitOK
}

func loadFixture(path string) ([]GroundTruth, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []GroundTruth
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, err
	}
	for i, e := range entries {
		if e.HistoryID == "" || e.ExpectedCategory == "" {
			return nil, fmt.Errorf("entry %d: history_id and expected_category required", i)
		}
	}
	return entries, nil
}

func connectMCP(ctx context.Context, serverURL, token string) (*mcpsdk.ClientSession, error) {
	httpClient := &http.Client{
		Transport: &bearerTransport{token: token, base: http.DefaultTransport},
		Timeout:   30 * time.Second,
	}
	client := mcpsdk.NewClient(&mcpsdk.Implementation{Name: "mcp-eval", Version: "v0"}, nil)
	transport := &mcpsdk.StreamableClientTransport{Endpoint: serverURL, HTTPClient: httpClient}
	return client.Connect(ctx, transport, nil)
}

func evalOne(ctx context.Context, cs *mcpsdk.ClientSession, llm *http.Client, apiKey, model string, e GroundTruth, skipLLM bool) Result {
	res := Result{HistoryID: e.HistoryID, Expected: e.ExpectedCategory}

	mcpStart := time.Now()
	failure, ferr := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "get_test_failure",
		Arguments: map[string]any{
			"project_id": e.ProjectID,
			"build_id":   e.BuildID,
			"history_id": e.HistoryID,
		},
	})
	if ferr != nil {
		res.Error = "get_test_failure: " + ferr.Error()
		if isToolNotFound(ferr) {
			res.Error = "tool not found: get_test_failure"
		}
		return res
	}
	if failure.IsError {
		res.Error = "get_test_failure returned error: " + contentText(failure.Content)
		return res
	}

	history, herr := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "get_test_history",
		Arguments: map[string]any{
			"project_id": e.ProjectID,
			"history_id": e.HistoryID,
			"limit":      10,
		},
	})
	if herr != nil {
		res.Error = "get_test_history: " + herr.Error()
		if isToolNotFound(herr) {
			res.Error = "tool not found: get_test_history"
		}
		return res
	}
	res.MCPCallMs = time.Since(mcpStart).Milliseconds()

	// diagnose_failure is the flagship tool — exercise it on every fixture so
	// dedup/cluster/build_verdict work under active development is covered
	// by the gate, not just get_test_failure/get_test_history.
	diagnoseStart := time.Now()
	diagnose, derr := cs.CallTool(ctx, &mcpsdk.CallToolParams{
		Name: "diagnose_failure",
		Arguments: map[string]any{
			"project_id": e.ProjectID,
			"build_id":   e.BuildID,
		},
	})
	res.DiagnoseCallMs = time.Since(diagnoseStart).Milliseconds()
	if derr != nil {
		res.Error = "diagnose_failure: " + derr.Error()
		if isToolNotFound(derr) {
			res.Error = "tool not found: diagnose_failure"
		}
		return res
	}
	if diagnose.IsError {
		res.Error = "diagnose_failure returned error: " + contentText(diagnose.Content)
		return res
	}

	diagnoseBytes, merr := json.Marshal(diagnose.StructuredContent)
	if merr != nil {
		res.Error = "diagnose_failure: marshal structured content: " + merr.Error()
		return res
	}
	res.DiagnoseBytes = len(diagnoseBytes)

	maxDiagnoseBytes := e.MaxDiagnoseBytes
	if maxDiagnoseBytes <= 0 {
		maxDiagnoseBytes = defaultMaxDiagnoseBytes
	}
	res.StructuralErrors = validateDiagnoseStructure(diagnoseBytes, maxDiagnoseBytes)

	if skipLLM {
		return res
	}

	prompt := buildPrompt(failure.StructuredContent, history.StructuredContent, diagnose.StructuredContent)

	llmStart := time.Now()
	predicted, usage, lerr := classify(ctx, llm, apiKey, model, prompt)
	res.LLMCallMs = time.Since(llmStart).Milliseconds()
	if lerr != nil {
		res.Error = lerr.Error()
		return res
	}

	res.Predicted = predicted
	res.LLMInputTokens = usage.InputTokens
	res.LLMOutputTokens = usage.OutputTokens
	if predicted == e.ExpectedCategory {
		res.Score = 1
	}
	return res
}

func buildPrompt(failure, history, diagnose any) string {
	fb, _ := json.MarshalIndent(failure, "", "  ")
	hb, _ := json.MarshalIndent(history, "", "  ")
	db, _ := json.MarshalIndent(diagnose, "", "  ")
	return fmt.Sprintf(`Classify this CI test failure into one of: test_bug, product_bug, infrastructure, flake.
Respond with ONLY the category name (no punctuation, no explanation).

Failure data:
%s

Recent history:
%s

Full build diagnosis:
%s
`, string(fb), string(hb), string(db))
}

func classify(ctx context.Context, c *http.Client, apiKey, model, prompt string) (string, anthropicUsage, error) {
	body := anthropicReq{
		Model:       model,
		MaxTokens:   50,
		Temperature: 0,
		Messages:    []anthropicMessage{{Role: "user", Content: prompt}},
	}
	for range 2 {
		predicted, usage, retry, err := callAnthropic(ctx, c, apiKey, body)
		if err == nil {
			return predicted, usage, nil
		}
		if !retry {
			return "", anthropicUsage{}, err
		}
		select {
		case <-ctx.Done():
			return "", anthropicUsage{}, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	return "", anthropicUsage{}, errors.New("anthropic: rate limited after retry")
}

func callAnthropic(ctx context.Context, c *http.Client, apiKey string, body anthropicReq) (string, anthropicUsage, bool, error) {
	bb, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, anthropicURL, bytes.NewReader(bb))
	if err != nil {
		return "", anthropicUsage{}, false, err
	}
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	resp, err := c.Do(req)
	if err != nil {
		return "", anthropicUsage{}, false, fmt.Errorf("anthropic: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(resp.Body)
	switch resp.StatusCode {
	case http.StatusOK:
		var ar anthropicResp
		if err := json.Unmarshal(respBody, &ar); err != nil {
			return "", anthropicUsage{}, false, fmt.Errorf("anthropic decode: %w", err)
		}
		if len(ar.Content) == 0 {
			return "", anthropicUsage{}, false, errors.New("anthropic: empty content")
		}
		return normalizeCategory(ar.Content[0].Text), ar.Usage, false, nil
	case http.StatusUnauthorized:
		return "", anthropicUsage{}, false, errors.New("llm auth failed (check ANTHROPIC_API_KEY)")
	case http.StatusTooManyRequests:
		return "", anthropicUsage{}, true, errors.New("anthropic 429 rate limited")
	default:
		return "", anthropicUsage{}, false, fmt.Errorf("anthropic status %d: %s", resp.StatusCode, string(respBody))
	}
}

func normalizeCategory(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	s = strings.Trim(s, ".\"' \t\n")
	switch s {
	case "test_bug", "product_bug", "infrastructure", "flake":
		return s
	}
	return s
}

func isToolNotFound(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "method not found") || strings.Contains(msg, "tool not found") || strings.Contains(msg, "unknown tool")
}

func contentText(content []mcpsdk.Content) string {
	parts := make([]string, 0, len(content))
	for _, c := range content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	return strings.Join(parts, " ")
}

func writeReport(path string, r *Report) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}
