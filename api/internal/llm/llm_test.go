package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// replyJSON wraps model text in a 200 response body of the given provider.
func replyJSON(provider, text string) string {
	var resp any = map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": text}}}}
	if provider == "anthropic" {
		resp = map[string]any{"content": []any{map[string]any{"type": "text", "text": text}}}
	}
	b, _ := json.Marshal(resp)
	return string(b)
}

const summaryJSON = `{"hypothesis":"The login API returned 500 due to a null pointer in the product code.","category":"product_bug","confidence":"medium","evidence":["status 500 from /users","last passed 3 builds ago"]}`

// errAny marks rows that expect some error without pinning which.
var errAny = errors.New("any error")

// TestSummarize sends one prompt per row to a fake provider and checks the
// request it saw, the number of attempts and the parsed summary.
func TestSummarize(t *testing.T) {
	parsed := Summary{
		Hypothesis: "The login API returned 500 due to a null pointer in the product code.",
		Category:   "product_bug",
		Confidence: "medium",
		Evidence:   []string{"status 500 from /users", "last passed 3 builds ago"},
	}
	const plain = "The test failed because the connection timed out reaching the database."
	// A bounded read must never reach JSON placed after maxResponseBytes of padding.
	padding := strings.Repeat(" ", maxResponseBytes+4096)

	tests := []struct {
		name      string
		provider  string
		apiKey    string
		status    int // 0 = 200
		body      string
		wantReq   map[string]string // "path", or a request header name
		wantInReq []string          // request body substrings
		want      Summary
		wantErr   error
		wantCalls int
	}{
		{name: "openai", provider: "openai", apiKey: "sk-test", body: replyJSON("openai", summaryJSON),
			wantReq:   map[string]string{"path": "/chat/completions", "Authorization": "Bearer sk-test"},
			wantInReq: []string{`"model":"m"`, `"role":"system"`}, want: parsed, wantCalls: 1},
		{name: "openai omits auth without a key", provider: "openai", body: replyJSON("openai", summaryJSON),
			wantReq: map[string]string{"Authorization": ""}, want: parsed, wantCalls: 1},
		{name: "anthropic", provider: "anthropic", apiKey: "sk-ant", body: replyJSON("anthropic", summaryJSON),
			wantReq:   map[string]string{"path": "/v1/messages", "x-api-key": "sk-ant", "anthropic-version": "2023-06-01"},
			wantInReq: []string{`"system":"sys"`}, want: parsed, wantCalls: 1},
		{name: "401 is ErrAuth without retry", provider: "openai", apiKey: "bad", status: http.StatusUnauthorized,
			body: `{"error":{"message":"bad key"}}`, wantErr: ErrAuth, wantCalls: 1},
		{name: "429 retries once then fails", provider: "openai", status: http.StatusTooManyRequests,
			body: `{"error":{"message":"rate limited"}}`, wantErr: errAny, wantCalls: 2},
		{name: "malformed text falls back to the whole text", provider: "openai", body: replyJSON("openai", plain),
			want: Summary{Hypothesis: plain, Category: "infrastructure"}, wantCalls: 1},
		{name: "fenced JSON is parsed", provider: "openai", body: replyJSON("openai", "```json\n"+summaryJSON+"\n```"),
			want: parsed, wantCalls: 1},
		{name: "openai response body is bounded", provider: "openai", body: padding + replyJSON("openai", summaryJSON),
			wantErr: errAny, wantCalls: 1},
		{name: "anthropic response body is bounded", provider: "anthropic", body: padding + replyJSON("anthropic", summaryJSON),
			wantErr: errAny, wantCalls: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			got := map[string]string{}
			var reqBody string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				got["path"] = r.URL.Path
				for h := range tc.wantReq {
					if h != "path" {
						got[h] = r.Header.Get(h)
					}
				}
				b, _ := io.ReadAll(r.Body)
				reqBody = string(b)
				if tc.status != 0 {
					w.WriteHeader(tc.status)
				}
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			c := New(config.LLMConfig{Provider: tc.provider, BaseURL: srv.URL, Model: "m", MaxTokens: 10, APIKey: tc.apiKey,
				Timeout: config.DurationSeconds(5 * time.Second)})
			c.retryDelay = time.Millisecond
			sum, err := c.Summarize(context.Background(), Prompt{System: "sys", User: "usr"})

			switch {
			case tc.wantErr == errAny && err == nil, tc.wantErr != errAny && !errors.Is(err, tc.wantErr):
				t.Fatalf("Summarize error = %v, want %v", err, tc.wantErr)
			case !reflect.DeepEqual(sum, tc.want):
				t.Errorf("summary = %+v, want %+v", sum, tc.want)
			}
			if calls != tc.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tc.wantCalls)
			}
			for k, want := range tc.wantReq {
				if got[k] != want {
					t.Errorf("request %s = %q, want %q", k, got[k], want)
				}
			}
			for _, s := range tc.wantInReq {
				if !strings.Contains(reqBody, s) {
					t.Errorf("request body lacks %s: %s", s, reqBody)
				}
			}
		})
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
