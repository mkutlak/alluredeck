package runner

import (
	"context"
	"encoding/json"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// InjectTraceContextIntoMetadata serializes the active trace context from ctx
// into a JSON map suitable for river.InsertOpts.Metadata. Returns nil when ctx
// has no valid span context, no propagator is configured, or marshaling fails,
// so callers can pass the result directly without forcing an empty payload.
func InjectTraceContextIntoMetadata(ctx context.Context) []byte {
	if !trace.SpanContextFromContext(ctx).IsValid() {
		return nil
	}
	carrier := propagation.MapCarrier{}
	otel.GetTextMapPropagator().Inject(ctx, carrier)
	if len(carrier) == 0 {
		return nil
	}
	b, err := json.Marshal(carrier)
	if err != nil {
		return nil
	}
	return b
}

// extractTraceContext reads a trace-context map from the given metadata bytes
// and returns a context with the parent span context restored. On any error
// (nil metadata, malformed JSON), the original ctx is returned unchanged so
// the caller can proceed without parent linkage rather than fail.
func extractTraceContext(ctx context.Context, metadata []byte) context.Context {
	if len(metadata) == 0 {
		return ctx
	}
	var m map[string]string
	if err := json.Unmarshal(metadata, &m); err != nil {
		return ctx
	}
	if len(m) == 0 {
		return ctx
	}
	return otel.GetTextMapPropagator().Extract(ctx, propagation.MapCarrier(m))
}
