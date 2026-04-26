package observability

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// ClampSampleRatio clamps ratio to the valid [0.0, 1.0] range.
// Values below 0 become 0; values above 1 become 1.
func ClampSampleRatio(ratio float64) float64 {
	if ratio < 0 {
		return 0
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}

// newTraceProvider constructs an SDK TracerProvider with an OTLP exporter
// selected by cfg.Protocol ("grpc" or "http/protobuf"), registers it
// globally, and returns it so the caller can call Shutdown on exit.
func newTraceProvider(ctx context.Context, cfg config.ObservabilityConfig, res *resource.Resource) (*sdktrace.TracerProvider, error) {
	exporter, err := newTraceExporter(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("trace exporter: %w", err)
	}

	ratio := ClampSampleRatio(cfg.Traces.SampleRatio)
	sampler := sdktrace.ParentBased(sdktrace.TraceIDRatioBased(ratio))

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sampler),
	)
	otel.SetTracerProvider(tp)
	return tp, nil
}

// newTraceExporter returns an OTLP span exporter.
// Protocol "grpc" uses gRPC; everything else (including the default
// "http/protobuf") uses HTTP/protobuf.
func newTraceExporter(ctx context.Context, cfg config.ObservabilityConfig) (sdktrace.SpanExporter, error) {
	endpoint := cfg.Traces.Endpoint

	if cfg.Traces.Protocol == "grpc" {
		opts := []otlptracegrpc.Option{}
		if endpoint != "" {
			opts = append(opts, otlptracegrpc.WithEndpoint(endpoint))
		}
		if cfg.Traces.Insecure {
			opts = append(opts, otlptracegrpc.WithInsecure())
		}
		return otlptracegrpc.New(ctx, opts...)
	}

	// Default: HTTP/protobuf
	opts := []otlptracehttp.Option{}
	if endpoint != "" {
		opts = append(opts, otlptracehttp.WithEndpoint(endpoint))
	}
	if cfg.Traces.Insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}
	return otlptracehttp.New(ctx, opts...)
}

// newResource builds the OTel resource with service.name, service.version,
// and deployment.environment attributes.
func newResource(serviceName, serviceVersion, environment string) (*resource.Resource, error) {
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
			semconv.DeploymentEnvironmentNameKey.String(environment),
		),
	)
}
