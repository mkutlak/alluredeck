package runner

import (
	"context"
	"fmt"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const otelTracerName = "alluredeck/runner"

// OTelWorkerMiddleware instruments River workers with OpenTelemetry tracing.
// It starts a span for each job execution, sets job attributes, propagates
// errors to the span, and records job duration metrics.
type OTelWorkerMiddleware struct {
	river.WorkerMiddlewareDefaults
	tracer trace.Tracer
}

// NewOTelWorkerMiddleware creates a new OTelWorkerMiddleware using the global
// TracerProvider (set by observability.Init).
func NewOTelWorkerMiddleware() *OTelWorkerMiddleware {
	return &OTelWorkerMiddleware{
		tracer: otel.Tracer(otelTracerName),
	}
}

// Work wraps the next middleware/worker call with a trace span. If the inbound
// job carries a trace context in its metadata (set at enqueue time by
// InjectTraceContextIntoMetadata), the span is chained to that parent so
// end-to-end traces span the originating HTTP request through to the worker.
func (m *OTelWorkerMiddleware) Work(ctx context.Context, job *rivertype.JobRow, doInner func(ctx context.Context) error) error {
	ctx = extractTraceContext(ctx, job.Metadata)
	spanName := fmt.Sprintf("river.job.work/%s", job.Kind)

	ctx, span := m.tracer.Start(ctx, spanName,
		trace.WithSpanKind(trace.SpanKindConsumer),
		trace.WithAttributes(
			attribute.String("river.job.kind", job.Kind),
			attribute.String("river.job.queue", job.Queue),
			attribute.Int("river.job.attempt", job.Attempt),
			semconv.MessagingSystemKey.String("river"),
		),
	)
	defer span.End()

	if err := doInner(ctx); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetStatus(codes.Ok, "")
	return nil
}
