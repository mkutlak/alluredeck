package observability

import (
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
)

// setupPropagation installs the W3C TraceContext + Baggage composite propagator
// as the global text map propagator. This enables trace context to be extracted
// from and injected into HTTP headers and other carriers automatically.
func setupPropagation() {
	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)
}
