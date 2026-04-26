package observability

import (
	"context"
	"errors"
	"net/http"
	"sync"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// ShutdownFunc is the type returned by Init. Calling it flushes traces,
// stops the metrics server, and returns the first error encountered.
type ShutdownFunc func(ctx context.Context) error

// composite bundles the resources that need to be torn down on shutdown.
type composite struct {
	once          sync.Once
	traceProvider *sdktrace.TracerProvider // nil when disabled
	metricsServer *http.Server             // nil when metrics disabled
}

// shutdown flushes the trace provider (if any), shuts down the metrics HTTP
// server (if any), and returns the first non-nil error. It is safe to call
// more than once — subsequent calls are no-ops.
func (c *composite) shutdown(ctx context.Context) error {
	var firstErr error
	c.once.Do(func() {
		var errs []error

		if c.traceProvider != nil {
			if err := c.traceProvider.Shutdown(ctx); err != nil {
				errs = append(errs, err)
			}
		}

		if c.metricsServer != nil {
			if err := c.metricsServer.Shutdown(ctx); err != nil {
				errs = append(errs, err)
			}
		}

		firstErr = errors.Join(errs...)
	})
	return firstErr
}
