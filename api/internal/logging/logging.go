// Package logging provides structured logging helpers built on go.uber.org/zap.
// It exposes Setup (root logger factory), ParseLevel (string → zapcore.Level),
// and context helpers (WithContext / FromContext) for request-scoped loggers.
package logging

import (
	"context"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type contextKey struct{}

// Setup creates the root zap logger and installs it as the global logger.
// devMode=true produces a human-readable console encoder (development preset).
// devMode=false produces a JSON encoder suitable for log aggregation (production preset).
// level is parsed by ParseLevel; unrecognised values default to InfoLevel.
func Setup(devMode bool, level string) *zap.Logger {
	lvl := ParseLevel(level)

	var logger *zap.Logger
	if devMode {
		cfg := zap.NewDevelopmentConfig()
		cfg.Level = zap.NewAtomicLevelAt(lvl)
		var err error
		logger, err = cfg.Build()
		if err != nil {
			// Fall back to a no-op logger rather than panicking
			logger = zap.NewNop()
		}
	} else {
		cfg := zap.NewProductionConfig()
		cfg.Level = zap.NewAtomicLevelAt(lvl)
		var err error
		logger, err = cfg.Build()
		if err != nil {
			logger = zap.NewNop()
		}
	}

	zap.ReplaceGlobals(logger)
	return logger
}

// ParseLevel maps a log-level string to its zapcore.Level equivalent.
// Recognised values (case-insensitive): debug, info, warn / warning, error.
// Any unrecognised string (including empty) returns InfoLevel.
func ParseLevel(s string) zapcore.Level {
	switch strings.ToLower(s) {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn", "warning":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// WithContext stores logger in ctx and returns the derived context.
// Passing nil stores nothing; FromContext will return the global fallback.
func WithContext(ctx context.Context, logger *zap.Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, contextKey{}, logger)
}

// FromContext retrieves the logger stored by WithContext.
// If no logger is present in ctx it returns the global zap.L() fallback.
func FromContext(ctx context.Context) *zap.Logger {
	if logger, ok := ctx.Value(contextKey{}).(*zap.Logger); ok && logger != nil {
		return logger
	}
	return zap.L()
}
