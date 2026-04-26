package runner

import (
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// newOTelHTTPClient returns an *http.Client whose Transport is wrapped with
// otelhttp so every outbound request produces a child span. The given timeout
// is preserved on the client.
func newOTelHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   timeout,
	}
}
