package config

import (
	"errors"
	"testing"
)

// TestLLMConfig_DisabledByDefault pins the no-egress default: nothing contacts
// an external LLM unless an operator sets LLM_ENABLED.
func TestLLMConfig_DisabledByDefault(t *testing.T) {
	t.Setenv("CONFIG_FILE", "/nonexistent-config-file-for-defaults.yaml")
	t.Setenv("DATABASE_URL", "postgres://localhost/test")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.LLM.Enabled {
		t.Error("LLM must be disabled by default, got Enabled=true")
	}
}

func TestLLMConfig_Validate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		llm  LLMConfig
		want error
	}{
		{"disabled skips validation of an incomplete config", LLMConfig{}, nil},
		{"enabled without a model", LLMConfig{Enabled: true, Provider: "openai", BaseURL: "http://ollama:11434/v1"}, ErrLLMModelRequired},
		{"openai without a base URL", LLMConfig{Enabled: true, Provider: "openai", Model: "llama3.1"}, ErrLLMBaseURLRequired},
		{"unknown provider", LLMConfig{Enabled: true, Provider: "gemini", Model: "some-model", BaseURL: "http://x/v1"}, ErrLLMProviderInvalid},
		{"openai without an API key (local Ollama)", LLMConfig{Enabled: true, Provider: "openai", Model: "llama3.1", BaseURL: "http://ollama:11434/v1"}, nil},
		{"anthropic needs no base URL", LLMConfig{Enabled: true, Provider: "anthropic", Model: "claude-sonnet-4-6", APIKey: "sk-ant-xxx"}, nil},
	}
	for _, tc := range tests {
		cfg := &Config{DatabaseURL: "postgres://localhost/test", JWTSecret: "some-safe-secret", LLM: tc.llm}
		if err := cfg.Validate(); !errors.Is(err, tc.want) {
			t.Errorf("%s: Validate() = %v, want %v", tc.name, err, tc.want)
		}
	}
}
