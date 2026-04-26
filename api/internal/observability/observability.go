// Package observability bootstraps the OpenTelemetry SDK for the AllureDeck API.
//
// Call Init once, right after config and logger are ready, before any DB or
// HTTP server is started. The returned ShutdownFunc must be called after the
// HTTP server drains and the DB pool closes, so in-flight spans can flush.
//
// When cfg.Enabled is false, Init installs noop providers and returns a
// no-op shutdown — callers are unchanged regardless of whether the feature
// flag is on or off.
package observability

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric/noop"
	nooptrace "go.opentelemetry.io/otel/trace/noop"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/version"
)

// Init bootstraps the OTel SDK and returns a composite ShutdownFunc.
//
//   - When cfg.Enabled == false: installs noop providers, returns a no-op shutdown.
//   - When cfg.Enabled == true: wires OTLP trace exporter, Prometheus metrics server,
//     runtime metric collection, and W3C propagation. The returned ShutdownFunc
//     flushes the trace provider and stops the metrics server.
//
// The logger is used only for startup/shutdown diagnostic messages.
func Init(ctx context.Context, cfg config.ObservabilityConfig, logger *zap.Logger) (ShutdownFunc, error) {
	if !cfg.Enabled {
		// Install noop providers so otel.Tracer/otel.Meter calls are safe
		// everywhere in the codebase even when observability is off.
		otel.SetTracerProvider(nooptrace.NewTracerProvider())
		otel.SetMeterProvider(noop.NewMeterProvider())
		c := &composite{}
		return c.shutdown, nil
	}

	// Resolve service version from build metadata if not overridden in config.
	svcVersion := cfg.ServiceVersion
	if svcVersion == "" {
		svcVersion = version.Version
	}

	res, err := newResource(cfg.ServiceName, svcVersion, cfg.Environment)
	if err != nil {
		return nil, err
	}

	tp, err := newTraceProvider(ctx, cfg, res)
	if err != nil {
		return nil, err
	}
	logger.Info("observability: trace provider initialised",
		zap.String("protocol", cfg.Traces.Protocol),
		zap.Float64("sample_ratio", cfg.Traces.SampleRatio),
	)

	metricsSrv, err := newMeterProvider(ctx, cfg, res)
	if err != nil {
		// Shut down the already-started trace provider before returning.
		_ = tp.Shutdown(ctx)
		return nil, err
	}
	if metricsSrv != nil {
		logger.Info("observability: metrics server started",
			zap.String("addr", cfg.Metrics.Addr),
			zap.String("path", cfg.Metrics.Path),
		)
	}

	setupPropagation()

	c := &composite{
		traceProvider: tp,
		metricsServer: metricsSrv,
	}
	return c.shutdown, nil
}
