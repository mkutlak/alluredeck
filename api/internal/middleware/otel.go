package middleware

import (
	"net/http"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"
)

// OTel returns a middleware constructor that wraps an http.Handler with
// OpenTelemetry tracing and metrics using the given TracerProvider.
// Slot it between RequestID and Logging so the active span is available
// to downstream middleware (e.g. Zap trace_id injection).
//
// Span naming uses the matched route pattern from Go 1.22+ ServeMux
// (e.g. "GET /api/v1/projects/{id}"). For unmatched routes the raw path
// is used as fallback, keeping cardinality bounded.
func OTel(tp trace.TracerProvider) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return otelhttp.NewHandler(
			next,
			"", // operation name — overridden per-request by the span name formatter below
			otelhttp.WithTracerProvider(tp),
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				if p := r.Pattern; p != "" {
					// Go 1.22+ ServeMux patterns include the method prefix,
					// e.g. "GET /api/v1/projects/{id}" — use as-is.
					return p
				}
				// Fallback for unmatched routes or handlers registered without
				// a method prefix: prefix with method to keep names readable.
				return r.Method + " " + r.URL.Path
			}),
		)
	}
}
