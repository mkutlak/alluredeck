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

// openaiMessage is one chat message in an OpenAI /chat/completions request.
type openaiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// openaiReq is the OpenAI /chat/completions request body. max_tokens is used
// (rather than max_completion_tokens) for the widest OpenAI-compatible-server
// support, including self-hosted Ollama.
type openaiReq struct {
	Model     string          `json:"model"`
	MaxTokens int             `json:"max_tokens"`
	Messages  []openaiMessage `json:"messages"`
}

// openaiResp captures the single field we read from an OpenAI response.
type openaiResp struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// callOpenAI POSTs to {BaseURL}/chat/completions and returns the assistant
// message content. The bool reports whether the failure is retryable.
func (c *Client) callOpenAI(ctx context.Context, p Prompt) (string, bool, error) {
	body := openaiReq{
		Model:     c.cfg.Model,
		MaxTokens: c.cfg.MaxTokens,
		Messages: []openaiMessage{
			{Role: "system", Content: p.System},
			{Role: "user", Content: p.User},
		},
	}
	bb, err := json.Marshal(body)
	if err != nil {
		return "", false, fmt.Errorf("openai marshal: %w", err)
	}

	endpoint := strings.TrimSuffix(c.cfg.BaseURL, "/") + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(bb))
	if err != nil {
		return "", false, err
	}
	req.Header.Set("content-type", "application/json")
	if c.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("openai: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	switch resp.StatusCode {
	case http.StatusOK:
		var or openaiResp
		if err := json.Unmarshal(respBody, &or); err != nil {
			return "", false, fmt.Errorf("openai decode: %w", err)
		}
		if len(or.Choices) == 0 {
			return "", false, errors.New("openai: empty choices")
		}
		return or.Choices[0].Message.Content, false, nil
	case http.StatusUnauthorized:
		return "", false, ErrAuth
	case http.StatusTooManyRequests:
		return "", true, errors.New("openai: 429 rate limited")
	default:
		return "", false, fmt.Errorf("openai status %d: %s", resp.StatusCode, string(respBody))
	}
}
