package runner

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river/rivertype"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestOTelWorkerMiddleware drives a River job through the tracing middleware:
// the trace context injected at enqueue time must parent the worker span, a
// failing job must record the error and still return it, and unusable
// metadata must not break the job.
func TestOTelWorkerMiddleware(t *testing.T) {
	prev := otel.GetTextMapPropagator()
	otel.SetTextMapPropagator(propagation.TraceContext{})
	t.Cleanup(func() { otel.SetTextMapPropagator(prev) })

	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
	tracer := tp.Tracer(otelTracerName)

	if meta := InjectTraceContextIntoMetadata(context.Background()); meta != nil {
		t.Errorf("no active span must inject nil metadata, got %s", meta)
	}
	parentCtx, parent := tracer.Start(context.Background(), "http.request")
	parent.End()
	parentMeta := InjectTraceContextIntoMetadata(parentCtx)

	jobErr := errors.New("report generation failed")
	tests := []struct {
		name       string
		kind       string
		meta       []byte
		workErr    error
		wantParent bool
	}{
		{name: "enqueue-time trace context parents the worker span", kind: "generate_report", meta: parentMeta, wantParent: true},
		{name: "job error is recorded and returned", kind: "failing_job", workErr: jobErr},
		{name: "invalid metadata is ignored", kind: "bad_meta_job", meta: []byte("not-json")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			job := &rivertype.JobRow{Kind: tc.kind, Queue: "default", Attempt: 1, CreatedAt: time.Now(), Metadata: tc.meta}
			err := (&OTelWorkerMiddleware{tracer: tracer}).Work(context.Background(), job,
				func(context.Context) error { return tc.workErr })
			if !errors.Is(err, tc.workErr) {
				t.Fatalf("Work error = %v, want %v", err, tc.workErr)
			}

			var span sdktrace.ReadOnlySpan
			for _, s := range rec.Ended() {
				if s.Name() == "river.job.work/"+tc.kind {
					span = s
				}
			}
			if span == nil {
				t.Fatalf("no span named river.job.work/%s", tc.kind)
			}
			if !hasAttr(span.Attributes(), "river.job.kind", tc.kind) {
				t.Errorf("missing river.job.kind=%s attribute: %v", tc.kind, span.Attributes())
			}
			isChild := span.SpanContext().TraceID() == parent.SpanContext().TraceID() &&
				span.Parent().SpanID() == parent.SpanContext().SpanID()
			if isChild != tc.wantParent {
				t.Errorf("worker span parented by enqueue span = %v, want %v", isChild, tc.wantParent)
			}
			recordedErr := false
			for _, e := range span.Events() {
				recordedErr = recordedErr || e.Name == "exception"
			}
			if recordedErr != (tc.workErr != nil) {
				t.Errorf("exception event recorded = %v, want %v", recordedErr, tc.workErr != nil)
			}
		})
	}
}

func hasAttr(attrs []attribute.KeyValue, key, value string) bool {
	for _, a := range attrs {
		if string(a.Key) == key && a.Value.AsString() == value {
			return true
		}
	}
	return false
}

// TestNewOTelHTTPClient checks the instrumented client helper wraps its
// transport with otelhttp and keeps the timeout. The helper currently has no
// production caller (webhooks use newWebhookHTTPClient).
func TestNewOTelHTTPClient(t *testing.T) {
	client := newOTelHTTPClient(10 * time.Second)
	if _, ok := client.Transport.(*otelhttp.Transport); !ok || client.Timeout != 10*time.Second {
		t.Errorf("client = {Transport: %T, Timeout: %v}, want {*otelhttp.Transport, 10s}", client.Transport, client.Timeout)
	}
}
