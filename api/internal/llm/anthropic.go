package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// anthropicVersion is the required anthropic-version header value. It matches
// cmd/mcp-eval/main.go.
const anthropicVersion = "2023-06-01"

// defaultAnthropicBaseURL is used when LLMConfig.BaseURL is empty for the
// anthropic provider.
const defaultAnthropicBaseURL = "https://api.anthropic.com"

// anthropicMessage mirrors the verbatim struct from cmd/mcp-eval/main.go.
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// anthropicReq is the /v1/messages request body. System carries the system
// prompt as a top-level field (Anthropic's convention).
type anthropicReq struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

// anthropicResp mirrors the verbatim struct from cmd/mcp-eval/main.go.
type anthropicResp struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// callAnthropic POSTs to {BaseURL or default}/v1/messages and returns the first
// text content block. The bool reports whether the failure is retryable.
func (c *Client) callAnthropic(ctx context.Context, p Prompt) (string, bool, error) {
	base := c.cfg.BaseURL
	if base == "" {
		base = defaultAnthropicBaseURL
	}
	endpoint := strings.TrimSuffix(base, "/") + "/v1/messages"

	body := anthropicReq{
		Model:     c.cfg.Model,
		MaxTokens: c.cfg.MaxTokens,
		System:    p.System,
		Messages:  []anthropicMessage{{Role: "user", Content: p.User}},
	}
	bb, err := json.Marshal(body)
	if err != nil {
		return "", false, fmt.Errorf("anthropic marshal: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bb))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("x-api-key", c.cfg.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("content-type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("anthropic: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	switch resp.StatusCode {
	case http.StatusOK:
		var ar anthropicResp
		if err := json.Unmarshal(respBody, &ar); err != nil {
			return "", false, fmt.Errorf("anthropic decode: %w", err)
		}
		if len(ar.Content) == 0 {
			return "", false, errors.New("anthropic: empty content")
		}
		return ar.Content[0].Text, false, nil
	case http.StatusUnauthorized:
		return "", false, ErrAuth
	case http.StatusTooManyRequests:
		return "", true, errors.New("anthropic: 429 rate limited")
	default:
		return "", false, fmt.Errorf("anthropic status %d: %s", resp.StatusCode, string(respBody))
	}
}
