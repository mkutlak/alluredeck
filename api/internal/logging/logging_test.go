package logging_test

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/mkutlak/alluredeck/api/internal/logging"
)

func TestSetupDevMode(t *testing.T) {
	logger := logging.Setup(true, "debug")
	if logger == nil {
		t.Fatal("Setup(devMode=true) returned nil logger")
	}
}

func TestSetupProdMode(t *testing.T) {
	logger := logging.Setup(false, "info")
	if logger == nil {
		t.Fatal("Setup(devMode=false) returned nil logger")
	}
}

func TestSetupReplacesGlobals(t *testing.T) {
	logging.Setup(false, "info")
	global := zap.L()
	if global == nil {
		t.Fatal("Setup() should replace globals; zap.L() returned nil")
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input string
		want  zapcore.Level
	}{
		{"debug", zapcore.DebugLevel},
		{"DEBUG", zapcore.DebugLevel},
		{"info", zapcore.InfoLevel},
		{"INFO", zapcore.InfoLevel},
		{"warn", zapcore.WarnLevel},
		{"warning", zapcore.WarnLevel},
		{"WARN", zapcore.WarnLevel},
		{"error", zapcore.ErrorLevel},
		{"ERROR", zapcore.ErrorLevel},
		{"", zapcore.InfoLevel},        // empty → default info
		{"invalid", zapcore.InfoLevel}, // unknown → default info
		{"verbose", zapcore.InfoLevel}, // unknown → default info
	}
	for _, tc := range tests {
		got := logging.ParseLevel(tc.input)
		if got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

func TestFromContextNoLogger(t *testing.T) {
	// When no logger is stored, FromContext returns the global zap.L() fallback
	logging.Setup(false, "info") // ensure global is set
	ctx := context.Background()
	logger := logging.FromContext(ctx)
	if logger == nil {
		t.Fatal("FromContext(empty ctx) should return fallback logger, got nil")
	}
}

func TestWithAndFromContext(t *testing.T) {
	logging.Setup(false, "info")
	original := zap.NewNop()
	ctx := logging.WithContext(context.Background(), original)
	retrieved := logging.FromContext(ctx)
	if retrieved != original {
		t.Errorf("FromContext should return the exact logger stored by WithContext")
	}
}

func TestFromContextReturnsFallbackWhenNil(t *testing.T) {
	// Explicitly passing nil should fall back to global
	logging.Setup(false, "info")
	ctx := logging.WithContext(context.Background(), nil)
	logger := logging.FromContext(ctx)
	if logger == nil {
		t.Fatal("FromContext with nil stored logger should return fallback, got nil")
	}
}
