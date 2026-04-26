package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river/rivertype"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func makeTestJobRow(kind, queue string, attempt int) *rivertype.JobRow {
	return &rivertype.JobRow{
		Kind:      kind,
		Queue:     queue,
		Attempt:   attempt,
		CreatedAt: time.Now(),
	}
}

func TestOTelWorkerMiddleware_RecordsSpanOnSuccess(t *testing.T) {
	t.Parallel()

	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))

	mw := &OTelWorkerMiddleware{
		tracer: tp.Tracer(otelTracerName),
	}

	job := makeTestJobRow("generate_report", "default", 1)
	err := mw.Work(context.Background(), job, func(_ context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]
	if span.Name() != "river.job.work/generate_report" {
		t.Errorf("unexpected span name: %q", span.Name())
	}

	// Verify job kind attribute.
	foundKind := false
	for _, a := range span.Attributes() {
		if string(a.Key) == "river.job.kind" && a.Value.AsString() == "generate_report" {
			foundKind = true
		}
	}
	if !foundKind {
		t.Errorf("expected river.job.kind attribute, got: %v", span.Attributes())
	}
}

func TestOTelWorkerMiddleware_RecordsErrorOnFailure(t *testing.T) {
	t.Parallel()

	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))

	mw := &OTelWorkerMiddleware{
		tracer: tp.Tracer(otelTracerName),
	}

	jobErr := errors.New("report generation failed")
	job := makeTestJobRow("generate_report", "default", 2)
	err := mw.Work(context.Background(), job, func(_ context.Context) error {
		return jobErr
	})
	if !errors.Is(err, jobErr) {
		t.Fatalf("expected jobErr to be returned, got %v", err)
	}

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	// Span should have an error event.
	span := spans[0]
	foundErr := false
	for _, e := range span.Events() {
		if e.Name == "exception" {
			foundErr = true
		}
	}
	if !foundErr {
		t.Errorf("expected exception event on span, got events: %v", span.Events())
	}
}
