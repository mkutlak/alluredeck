package runner

import (
	"context"
	"encoding/json"
	"testing"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestInjectTraceContextIntoMetadata_WithActiveSpan(t *testing.T) {
	t.Parallel()

	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))

	// Set the global propagator so inject works.
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}))
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	ctx, span := tp.Tracer(otelTracerName).Start(context.Background(), "parent")
	defer span.End()

	meta := InjectTraceContextIntoMetadata(ctx)
	if meta == nil {
		t.Fatal("expected non-nil metadata bytes with active span")
	}

	var m map[string]string
	if err := json.Unmarshal(meta, &m); err != nil {
		t.Fatalf("metadata is not valid JSON: %v", err)
	}

	if _, ok := m["traceparent"]; !ok {
		t.Errorf("expected 'traceparent' key in metadata, got: %v", m)
	}
}

func TestInjectTraceContextIntoMetadata_NoSpan(t *testing.T) {
	t.Parallel()

	// With no active span, inject should return nil.
	meta := InjectTraceContextIntoMetadata(context.Background())
	if meta != nil {
		t.Errorf("expected nil metadata with no active span, got: %s", meta)
	}
}

func TestExtractTraceContext_RestoresParent(t *testing.T) {
	t.Parallel()

	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))

	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}))
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	// Create parent span and inject its context into metadata.
	parentCtx, parentSpan := tp.Tracer(otelTracerName).Start(context.Background(), "parent")
	parentSpanCtx := parentSpan.SpanContext()
	parentSpan.End()

	meta := InjectTraceContextIntoMetadata(parentCtx)
	if meta == nil {
		t.Fatal("expected non-nil metadata from parent span")
	}

	// Extract the context from the metadata bytes.
	extracted := extractTraceContext(context.Background(), meta)

	// Start a child span using the extracted context and verify parentage.
	_, childSpan := tp.Tracer(otelTracerName).Start(extracted, "child")
	childSpan.End()

	var child sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		if s.Name() == "child" {
			child = s
			break
		}
	}
	if child == nil {
		t.Fatal("child span not found in recorded spans")
	}

	if child.SpanContext().TraceID() != parentSpanCtx.TraceID() {
		t.Errorf("child trace ID %s != parent trace ID %s",
			child.SpanContext().TraceID(), parentSpanCtx.TraceID())
	}
	if child.Parent().SpanID() != parentSpanCtx.SpanID() {
		t.Errorf("child parent span ID %s != parent span ID %s",
			child.Parent().SpanID(), parentSpanCtx.SpanID())
	}
}

func TestExtractTraceContext_NilMetadata(t *testing.T) {
	t.Parallel()

	// Nil metadata should return the original ctx without panicking.
	ctx := extractTraceContext(context.Background(), nil)
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
}

func TestExtractTraceContext_InvalidJSON(t *testing.T) {
	t.Parallel()

	// Invalid JSON should return the original ctx without panicking.
	ctx := extractTraceContext(context.Background(), []byte("not-json"))
	if ctx == nil {
		t.Fatal("expected non-nil context")
	}
}

func TestOTelWorkerMiddleware_PropagatesParentSpan(t *testing.T) {
	t.Parallel()

	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))

	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}))
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	// Create a parent span and inject into metadata.
	parentCtx, parentSpan := tp.Tracer(otelTracerName).Start(context.Background(), "http.request")
	parentSpanCtx := parentSpan.SpanContext()
	parentSpan.End()

	meta := InjectTraceContextIntoMetadata(parentCtx)
	if meta == nil {
		t.Fatal("expected non-nil metadata")
	}

	// Build a job row with that metadata.
	job := makeTestJobRow("generate_report", "default", 1)
	job.Metadata = meta

	mw := &OTelWorkerMiddleware{
		tracer: tp.Tracer(otelTracerName),
	}

	err := mw.Work(context.Background(), job, func(_ context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var workerSpan sdktrace.ReadOnlySpan
	for _, s := range rec.Ended() {
		if s.Name() == "river.job.work/generate_report" {
			workerSpan = s
			break
		}
	}
	if workerSpan == nil {
		t.Fatal("worker span not found")
	}

	if workerSpan.SpanContext().TraceID() != parentSpanCtx.TraceID() {
		t.Errorf("worker trace ID %s != parent trace ID %s",
			workerSpan.SpanContext().TraceID(), parentSpanCtx.TraceID())
	}
	if workerSpan.Parent().SpanID() != parentSpanCtx.SpanID() {
		t.Errorf("worker parent span ID %s != parent span ID %s",
			workerSpan.Parent().SpanID(), parentSpanCtx.SpanID())
	}
}
