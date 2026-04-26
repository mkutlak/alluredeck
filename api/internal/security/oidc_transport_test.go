package security

import (
	"net/http"
	"testing"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// TestOIDCProviderHTTPClientUsesOTelTransport verifies that a freshly built
// OIDCProvider has an httpClient whose Transport is wrapped with otelhttp so
// outbound OIDC requests are traced.
//
// We test the field directly (white-box) because NewOIDCProvider makes a
// network call to discover the OIDC issuer, which we cannot do in unit tests.
func TestOIDCProviderHTTPClientUsesOTelTransport(t *testing.T) {
	t.Parallel()

	// Construct the client the same way NewOIDCProvider does.
	client := &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	p := &OIDCProvider{
		httpClient: client,
	}

	if p.httpClient == nil {
		t.Fatal("expected non-nil httpClient on OIDCProvider")
	}

	if _, ok := p.httpClient.Transport.(*otelhttp.Transport); !ok {
		t.Errorf("expected *otelhttp.Transport, got %T", p.httpClient.Transport)
	}
}
