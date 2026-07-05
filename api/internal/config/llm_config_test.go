package config

import (
	"errors"
	"testing"
	"time"
)

// TestLLMConfig_Defaults verifies LoadConfig applies the documented LLM defaults
// (provider openai, max_tokens 512, timeout 30s) when nothing overrides them.
func TestLLMConfig_Defaults(t *testing.T) {
	t.Setenv("CONFIG_FILE", "/nonexistent-config-file-for-defaults.yaml")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.LLM.Enabled {
		t.Errorf("LLM must be disabled by default, got Enabled=true")
	}
	if cfg.LLM.Provider != "openai" {
		t.Errorf("default provider: got %q, want openai", cfg.LLM.Provider)
	}
	if cfg.LLM.MaxTokens != 512 {
		t.Errorf("default max_tokens: got %d, want 512", cfg.LLM.MaxTokens)
	}
	if cfg.LLM.Timeout.Duration() != 30*time.Second {
		t.Errorf("default timeout: got %s, want 30s", cfg.LLM.Timeout.Duration())
	}
}

// TestLLMConfig_Validate_Disabled verifies a disabled LLM never fails validation
// even with an otherwise-incomplete LLM config.
func TestLLMConfig_Validate_Disabled(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		DatabaseURL: "postgres://localhost/test",
		JWTSecret:   "some-safe-secret",
		LLM:         LLMConfig{Enabled: false},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected no error when LLM disabled, got %v", err)
	}
}

// TestLLMConfig_Validate_MissingModel verifies an enabled LLM without a model
// is rejected.
func TestLLMConfig_Validate_MissingModel(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		DatabaseURL: "postgres://localhost/test",
		JWTSecret:   "some-safe-secret",
		LLM: LLMConfig{
			Enabled:  true,
			Provider: "openai",
			BaseURL:  "http://ollama:11434/v1",
		},
	}
	if err := cfg.Validate(); !errors.Is(err, ErrLLMModelRequired) {
		t.Errorf("expected ErrLLMModelRequired, got %v", err)
	}
}

// TestLLMConfig_Validate_OpenAIMissingBaseURL verifies the openai provider
// requires a base URL.
func TestLLMConfig_Validate_OpenAIMissingBaseURL(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		DatabaseURL: "postgres://localhost/test",
		JWTSecret:   "some-safe-secret",
		LLM: LLMConfig{
			Enabled:  true,
			Provider: "openai",
			Model:    "llama3.1",
		},
	}
	if err := cfg.Validate(); !errors.Is(err, ErrLLMBaseURLRequired) {
		t.Errorf("expected ErrLLMBaseURLRequired, got %v", err)
	}
}

// TestLLMConfig_Validate_InvalidProvider verifies an unknown provider is rejected.
func TestLLMConfig_Validate_InvalidProvider(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		DatabaseURL: "postgres://localhost/test",
		JWTSecret:   "some-safe-secret",
		LLM: LLMConfig{
			Enabled:  true,
			Provider: "gemini",
			Model:    "some-model",
			BaseURL:  "http://x/v1",
		},
	}
	if err := cfg.Validate(); !errors.Is(err, ErrLLMProviderInvalid) {
		t.Errorf("expected ErrLLMProviderInvalid, got %v", err)
	}
}

// TestLLMConfig_Validate_OpenAIValid verifies a complete openai config passes.
func TestLLMConfig_Validate_OpenAIValid(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		DatabaseURL: "postgres://localhost/test",
		JWTSecret:   "some-safe-secret",
		LLM: LLMConfig{
			Enabled:  true,
			Provider: "openai",
			Model:    "llama3.1",
			BaseURL:  "http://ollama:11434/v1",
			// APIKey intentionally empty — valid for local Ollama.
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid openai config to pass, got %v", err)
	}
}

// TestLLMConfig_Validate_AnthropicValidWithoutBaseURL verifies the anthropic
// provider does not require a base URL (it defaults to the hosted endpoint).
func TestLLMConfig_Validate_AnthropicValidWithoutBaseURL(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		DatabaseURL: "postgres://localhost/test",
		JWTSecret:   "some-safe-secret",
		LLM: LLMConfig{
			Enabled:  true,
			Provider: "anthropic",
			Model:    "claude-sonnet-4-6",
			APIKey:   "sk-ant-xxx",
		},
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("expected valid anthropic config to pass without base_url, got %v", err)
	}
}
