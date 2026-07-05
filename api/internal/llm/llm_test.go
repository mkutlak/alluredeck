package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// canned OpenAI 200 response whose assistant content is a strict-JSON summary.
func openaiJSON(content string) string {
	resp := map[string]any{
		"choices": []map[string]any{
			{"message": map[string]any{"role": "assistant", "content": content}},
		},
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

// canned Anthropic 200 response whose text block is a strict-JSON summary.
func anthropicJSON(text string) string {
	resp := map[string]any{
		"content": []map[string]any{
			{"type": "text", "text": text},
		},
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

const summaryJSON = `{"hypothesis":"The login API returned 500 due to a null pointer in the product code.","category":"product_bug","confidence":"medium","evidence":["status 500 from /users","last passed 3 builds ago"]}`

func TestSummarize_OpenAI_Parse200(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, openaiJSON(summaryJSON))
	}))
	defer srv.Close()

	c := New(config.LLMConfig{
		Provider: "openai", BaseURL: srv.URL, Model: "llama3.1", MaxTokens: 512, APIKey: "sk-test",
		Timeout: config.DurationSeconds(5 * time.Second),
	})
	got, err := c.Summarize(context.Background(), Prompt{System: "sys", User: "usr"})
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if gotPath != "/chat/completions" {
		t.Errorf("path: got %q, want /chat/completions", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("auth header: got %q, want Bearer sk-test", gotAuth)
	}
	if !strings.Contains(gotBody, `"model":"llama3.1"`) || !strings.Contains(gotBody, `"role":"system"`) {
		t.Errorf("request body missing model/system message: %s", gotBody)
	}
	if got.Hypothesis == "" || got.Category != "product_bug" || got.Confidence != "medium" {
		t.Errorf("parsed summary: %+v", got)
	}
	if len(got.Evidence) != 2 {
		t.Errorf("evidence: got %v, want 2 items", got.Evidence)
	}
}

func TestSummarize_OpenAI_OmitsAuthWhenNoKey(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		_, _ = io.WriteString(w, openaiJSON(summaryJSON))
	}))
	defer srv.Close()

	c := New(config.LLMConfig{Provider: "openai", BaseURL: srv.URL, Model: "m", MaxTokens: 10})
	if _, err := c.Summarize(context.Background(), Prompt{User: "u"}); err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if gotAuth != "" {
		t.Errorf("Authorization header must be omitted when APIKey is empty, got %q", gotAuth)
	}
}

func TestSummarize_Anthropic_Parse200(t *testing.T) {
	var gotPath, gotKey, gotVer, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("x-api-key")
		gotVer = r.Header.Get("anthropic-version")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		_, _ = io.WriteString(w, anthropicJSON(summaryJSON))
	}))
	defer srv.Close()

	c := New(config.LLMConfig{Provider: "anthropic", BaseURL: srv.URL, Model: "claude-x", MaxTokens: 256, APIKey: "sk-ant"})
	got, err := c.Summarize(context.Background(), Prompt{System: "sys", User: "usr"})
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if gotPath != "/v1/messages" {
		t.Errorf("path: got %q, want /v1/messages", gotPath)
	}
	if gotKey != "sk-ant" {
		t.Errorf("x-api-key: got %q", gotKey)
	}
	if gotVer != "2023-06-01" {
		t.Errorf("anthropic-version: got %q", gotVer)
	}
	if !strings.Contains(gotBody, `"system":"sys"`) {
		t.Errorf("request body must carry system field: %s", gotBody)
	}
	if got.Category != "product_bug" {
		t.Errorf("parsed category: got %q", got.Category)
	}
}

func TestSummarize_401_Auth(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"bad key"}}`)
	}))
	defer srv.Close()

	c := New(config.LLMConfig{Provider: "openai", BaseURL: srv.URL, Model: "m", MaxTokens: 10, APIKey: "bad"})
	_, err := c.Summarize(context.Background(), Prompt{User: "u"})
	if !errors.Is(err, ErrAuth) {
		t.Errorf("want ErrAuth, got %v", err)
	}
	if calls != 1 {
		t.Errorf("401 must not retry: got %d calls, want 1", calls)
	}
}

func TestSummarize_429_RetriesThenFails(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"rate limited"}}`)
	}))
	defer srv.Close()

	c := New(config.LLMConfig{Provider: "openai", BaseURL: srv.URL, Model: "m", MaxTokens: 10})
	c.retryDelay = time.Millisecond // keep the test fast
	_, err := c.Summarize(context.Background(), Prompt{User: "u"})
	if err == nil {
		t.Fatal("want error after 429 retries exhausted")
	}
	if calls != 2 {
		t.Errorf("429 should be attempted twice: got %d calls, want 2", calls)
	}
}

func TestSummarize_Malformed_FallsBackToWholeText(t *testing.T) {
	const plain = "The test failed because the connection timed out reaching the database."
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, openaiJSON(plain))
	}))
	defer srv.Close()

	c := New(config.LLMConfig{Provider: "openai", BaseURL: srv.URL, Model: "m", MaxTokens: 10})
	got, err := c.Summarize(context.Background(), Prompt{User: "u"})
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if got.Hypothesis != plain {
		t.Errorf("fallback hypothesis: got %q, want the whole text", got.Hypothesis)
	}
}

func TestSummarize_StripsJSONFences(t *testing.T) {
	fenced := "```json\n" + summaryJSON + "\n```"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, openaiJSON(fenced))
	}))
	defer srv.Close()

	c := New(config.LLMConfig{Provider: "openai", BaseURL: srv.URL, Model: "m", MaxTokens: 10})
	got, err := c.Summarize(context.Background(), Prompt{User: "u"})
	if err != nil {
		t.Fatalf("Summarize: %v", err)
	}
	if got.Category != "product_bug" {
		t.Errorf("fenced JSON must be parsed: got category %q, hypothesis %q", got.Category, got.Hypothesis)
	}
}

// TestSummarize_OpenAI_ResponseBodyIsBounded is a regression test: a hostile
// or misconfigured endpoint should not be able to force a huge allocation via
// an oversized response. The valid JSON payload is placed AFTER more than
// maxResponseBytes of padding, so a bounded read must never reach it (and
// therefore fails to parse); an unbounded read would have consumed the
// padding and succeeded.
func TestSummarize_OpenAI_ResponseBodyIsBounded(t *testing.T) {
	padding := strings.Repeat(" ", maxResponseBytes+4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, padding)
		_, _ = io.WriteString(w, openaiJSON(summaryJSON))
	}))
	defer srv.Close()

	c := New(config.LLMConfig{Provider: "openai", BaseURL: srv.URL, Model: "m", MaxTokens: 10})
	if _, err := c.Summarize(context.Background(), Prompt{User: "u"}); err == nil {
		t.Fatal("want a decode error: the bounded read must not reach JSON appended after maxResponseBytes of padding")
	}
}

// TestSummarize_Anthropic_ResponseBodyIsBounded mirrors the OpenAI bounded-read
// regression test for the Anthropic path.
func TestSummarize_Anthropic_ResponseBodyIsBounded(t *testing.T) {
	padding := strings.Repeat(" ", maxResponseBytes+4096)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, padding)
		_, _ = io.WriteString(w, anthropicJSON(summaryJSON))
	}))
	defer srv.Close()

	c := New(config.LLMConfig{Provider: "anthropic", BaseURL: srv.URL, Model: "m", MaxTokens: 10})
	if _, err := c.Summarize(context.Background(), Prompt{User: "u"}); err == nil {
		t.Fatal("want a decode error: the bounded read must not reach JSON appended after maxResponseBytes of padding")
	}
}

func TestNormalizeCategory(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"test_bug", "test_bug"},
		{" Product_Bug\n", "product_bug"},
		{"infrastructure.", "infrastructure"},
		{`"flake"`, "flake"},
		{"something-random", "something-random"},
	}
	for _, c := range cases {
		if got := normalizeCategory(c.in); got != c.want {
			t.Errorf("normalizeCategory(%q): got %q, want %q", c.in, got, c.want)
		}
	}
}
