package testutil

import (
	"context"
	"sync"

	"github.com/mkutlak/alluredeck/api/internal/llm"
)

// StubSummarizer is a thread-safe failure.Summarizer double (the *llm.Client
// surface): Summarize returns Result and Err and counts its calls.
type StubSummarizer struct {
	Result llm.Summary
	Err    error

	mu    sync.Mutex
	calls int
}

func (s *StubSummarizer) Summarize(context.Context, llm.Prompt) (llm.Summary, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.Result, s.Err
}

// Calls reports how many times Summarize ran.
func (s *StubSummarizer) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}
