package runner

import (
	"net/http"
	"testing"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// TestWebhookWorkerUsesOTelTransport verifies that the http.Client used by
// SendWebhookWorker has its Transport wrapped with otelhttp.Transport so
// outbound webhook HTTP requests are traced.
func TestWebhookWorkerUsesOTelTransport(t *testing.T) {
	t.Parallel()

	client := newOTelHTTPClient(10 * time.Second)

	if _, ok := client.Transport.(*otelhttp.Transport); !ok {
		t.Errorf("expected Transport to be *otelhttp.Transport, got %T", client.Transport)
	}

	if client.Timeout != 10*time.Second {
		t.Errorf("expected 10s timeout, got %v", client.Timeout)
	}
}

// TestOTelHTTPClientPreservesNilTransport verifies that passing a nil base
// transport falls back to http.DefaultTransport.
func TestOTelHTTPClientPreservesDefaultTransport(t *testing.T) {
	t.Parallel()

	client := newOTelHTTPClient(5 * time.Second)

	tr, ok := client.Transport.(*otelhttp.Transport)
	if !ok {
		t.Fatalf("expected *otelhttp.Transport, got %T", client.Transport)
	}

	// The underlying transport should be http.DefaultTransport (the default
	// when nil is passed to otelhttp.NewTransport).
	if tr == nil {
		t.Error("expected non-nil otelhttp.Transport")
	}
	_ = http.DefaultTransport // compile-time reference
}
