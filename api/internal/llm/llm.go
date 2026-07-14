// Package llm is a small, stdlib-only client for generating a plain-language
// hypothesis about why a CI test failed. It talks to an OpenAI-compatible
// /chat/completions endpoint (default; e.g. a self-hosted Ollama) or the
// Anthropic /v1/messages endpoint. It is only ever reached when the operator
// has explicitly enabled the feature (config.LLMConfig.Enabled); nothing here
// runs on the default self-hosted, no-egress path.
package llm

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// Summary is the structured result of an LLM failure analysis. Every field is
// a display-only hypothesis, never an authoritative verdict.
type Summary struct {
	// Hypothesis is a plain-language best guess at the cause — a hypothesis,
	// not a verdict.
	Hypothesis string
	// Category is a normalized display label: test_bug | product_bug |
	// infrastructure | flake (or the model's raw label when it does not match).
	Category string
	// Confidence is one of "low" | "medium" | "high" | "".
	Confidence string
	// Evidence holds short bullet strings grounded in the provided input.
	Evidence []string
}

// Prompt carries the system and user halves of a single completion request.
type Prompt struct {
	System string
	User   string
}

// ErrAuth is returned when the provider rejects the API key (HTTP 401). It is
// not retryable.
var ErrAuth = errors.New("llm auth failed (check LLM_API_KEY)")

// Client is a provider-agnostic LLM client. Construct it with New.
type Client struct {
	http       *http.Client
	cfg        config.LLMConfig
	retryDelay time.Duration
}

// defaultRetryDelay is the backoff between the two attempts on a retryable
// (429) failure. It mirrors cmd/mcp-eval's classify() backoff intent while
// staying short enough not to stall a synchronous request for long.
const defaultRetryDelay = 2 * time.Second

// maxResponseBytes bounds how much of an LLM provider's HTTP response body is
// read (see openai.go / anthropic.go). A hostile or misconfigured endpoint
// should not be able to force a large allocation via an oversized response.
const maxResponseBytes = 2 << 20 // 2 MiB

// New returns a Client configured from cfg. The HTTP timeout is taken from
// cfg.Timeout (falling back to 30s when unset).
func New(cfg config.LLMConfig) *Client {
	timeout := cfg.Timeout.Duration()
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &Client{
		http:       &http.Client{Timeout: timeout},
		cfg:        cfg,
		retryDelay: defaultRetryDelay,
	}
}

// Summarize sends p to the configured provider and returns the parsed Summary.
// It makes up to two attempts, retrying once with a backoff on a retryable
// (rate-limited) failure, mirroring cmd/mcp-eval's classify().
func (c *Client) Summarize(ctx context.Context, p Prompt) (Summary, error) {
	var lastErr error
	for attempt := range 2 {
		raw, retry, err := c.call(ctx, p)
		if err == nil {
			return parseSummary(raw), nil
		}
		lastErr = err
		if !retry {
			return Summary{}, err
		}
		if attempt == 1 {
			break
		}
		select {
		case <-ctx.Done():
			return Summary{}, ctx.Err()
		case <-time.After(c.retryDelay):
		}
	}
	if lastErr != nil {
		return Summary{}, lastErr
	}
	return Summary{}, errors.New("llm: rate limited after retry")
}

// call dispatches to the provider-specific request. It returns the raw model
// text, a retryable flag, and an error.
func (c *Client) call(ctx context.Context, p Prompt) (string, bool, error) {
	if c.cfg.Provider == "anthropic" {
		return c.callAnthropic(ctx, p)
	}
	return c.callOpenAI(ctx, p)
}
