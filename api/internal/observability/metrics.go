package observability

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/runtime"
	"go.opentelemetry.io/otel"
	otelprometheus "go.opentelemetry.io/otel/exporters/prometheus"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// newMeterProvider creates a MeterProvider backed by the Prometheus exporter,
// registers it globally, starts Go runtime metric collection, and starts the
// auxiliary HTTP server that serves the Prometheus scrape endpoint.
// Returns the metrics HTTP server so the composite shutdown can close it.
func newMeterProvider(ctx context.Context, cfg config.ObservabilityConfig, res *resource.Resource) (*http.Server, error) {
	promExporter, err := otelprometheus.New()
	if err != nil {
		return nil, fmt.Errorf("prometheus exporter: %w", err)
	}

	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(promExporter),
	)
	otel.SetMeterProvider(mp)

	// Collect Go runtime metrics (goroutines, memory, GC, etc.).
	if err := runtime.Start(runtime.WithMeterProvider(mp), runtime.WithMinimumReadMemStatsInterval(15*time.Second)); err != nil {
		return nil, fmt.Errorf("runtime metrics: %w", err)
	}

	if !cfg.Metrics.Enabled {
		return nil, nil //nolint:nilnil // intentional: no server when metrics disabled
	}

	mux := http.NewServeMux()
	mux.Handle(cfg.Metrics.Path, promhttp.Handler())

	srv := &http.Server{
		Addr:              cfg.Metrics.Addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", cfg.Metrics.Addr)
	if err != nil {
		return nil, fmt.Errorf("metrics server listen %s: %w", cfg.Metrics.Addr, err)
	}

	go func() {
		if serveErr := srv.Serve(ln); serveErr != nil && serveErr != http.ErrServerClosed {
			// Non-fatal: log at error level but don't crash the process.
			_ = serveErr
		}
	}()

	return srv, nil
}
