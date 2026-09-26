package logging_test

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/mkutlak/alluredeck/api/internal/logging"
)

// TestSetup builds the dev and prod loggers at the parsed level and installs
// each as the global logger. Not parallel: it replaces zap's globals.
func TestSetup(t *testing.T) {
	for _, devMode := range []bool{true, false} {
		logger := logging.Setup(devMode, "warn")
		if zap.L() != logger {
			t.Errorf("devMode=%t: Setup did not replace the global logger", devMode)
		}
		if logger.Core().Enabled(zapcore.InfoLevel) || !logger.Core().Enabled(zapcore.WarnLevel) {
			t.Errorf("devMode=%t: logger level is not warn", devMode)
		}
	}
}

func TestParseLevel(t *testing.T) {
	t.Parallel()
	tests := []struct {
		input string
		want  zapcore.Level
	}{
		{"debug", zapcore.DebugLevel},
		{"warn", zapcore.WarnLevel},
		{"warning", zapcore.WarnLevel},
		{"ERROR", zapcore.ErrorLevel},
		{"", zapcore.InfoLevel}, // unset or unknown → info
	}
	for _, tc := range tests {
		if got := logging.ParseLevel(tc.input); got != tc.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// TestFromContext returns the logger stored by WithContext, and the global
// logger when none (or nil) was stored. Not parallel: it reads zap's globals.
func TestFromContext(t *testing.T) {
	stored := zap.NewNop()
	for _, tc := range []struct {
		name string
		ctx  context.Context
		want *zap.Logger
	}{
		{"stored logger", logging.WithContext(context.Background(), stored), stored},
		{"no logger falls back to global", context.Background(), zap.L()},
		{"nil logger falls back to global", logging.WithContext(context.Background(), nil), zap.L()},
	} {
		if got := logging.FromContext(tc.ctx); got != tc.want {
			t.Errorf("%s: FromContext returned a different logger", tc.name)
		}
	}
}
