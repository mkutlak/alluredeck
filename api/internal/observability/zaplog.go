package observability

import (
	"context"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// traceCore is a zapcore.Core wrapper that appends trace_id and span_id
// fields to every log entry when an active span is present in the context.
// When no span is active the entry is written unchanged.
//
// Usage: wrap the zap core returned by logging.Setup with NewTraceCore, then
// build a *zap.Logger from the wrapped core. All downstream With/Named calls
// are transparently delegated to the inner core.
type traceCore struct {
	zapcore.Core
}

// NewTraceCore wraps inner with a core that injects OTel trace/span IDs.
func NewTraceCore(inner zapcore.Core) zapcore.Core {
	return &traceCore{Core: inner}
}

// With delegates to the inner core, preserving the traceCore wrapper type.
func (c *traceCore) With(fields []zapcore.Field) zapcore.Core {
	return &traceCore{Core: c.Core.With(fields)}
}

// Check asks the inner core whether the entry should be logged. If so, it adds
// this traceCore to the CheckedEntry so that traceCore.Write is called (and
// can inject trace fields) rather than the inner core's Write directly.
func (c *traceCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return ce.AddCore(entry, c)
	}
	return ce
}

// Write appends trace_id and span_id fields when the entry carries a valid
// span via its context field, then delegates to the inner core.
// If no span is active, the entry is written without modification.
func (c *traceCore) Write(entry zapcore.Entry, fields []zapcore.Field) error {
	// Look for a context field injected by the logging middleware.
	ctx := extractContext(fields)
	if ctx != nil {
		span := trace.SpanFromContext(ctx)
		if span.SpanContext().IsValid() {
			sc := span.SpanContext()
			extra := []zapcore.Field{
				zap.String("trace_id", sc.TraceID().String()),
				zap.String("span_id", sc.SpanID().String()),
			}
			fields = append(extra, fields...)
		}
	}
	return c.Core.Write(entry, fields)
}

// extractContext scans fields for a zapcore.SkipType field whose key is
// "ctx" carrying a context.Context value. This field is set by the logging
// middleware (PR2) via zap.Any("ctx", ctx). If not found, returns nil.
func extractContext(fields []zapcore.Field) context.Context {
	for _, f := range fields {
		if f.Key == "ctx" {
			if ctx, ok := f.Interface.(context.Context); ok {
				return ctx
			}
		}
	}
	return nil
}
