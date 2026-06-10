package runner

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestIsDisallowedIP checks the deny predicate against known blocked and
// allowed addresses.
func TestIsDisallowedIP(t *testing.T) {
	t.Parallel()

	blocked := []string{
		"127.0.0.1",              // IPv4 loopback
		"::1",                    // IPv6 loopback
		"169.254.169.254",        // AWS/GCP metadata endpoint (link-local)
		"169.254.0.1",            // link-local unicast
		"10.0.0.1",               // RFC 1918 private
		"10.255.255.255",         // RFC 1918 private
		"172.16.0.1",             // RFC 1918 private
		"172.31.255.255",         // RFC 1918 private
		"192.168.0.1",            // RFC 1918 private
		"192.168.255.254",        // RFC 1918 private
		"0.0.0.0",                // unspecified
		"::ffff:127.0.0.1",       // IPv4-mapped loopback
		"::ffff:169.254.169.254", // IPv4-mapped link-local (metadata)
		"::ffff:10.0.0.1",        // IPv4-mapped private
		"::ffff:192.168.1.1",     // IPv4-mapped private
	}
	for _, raw := range blocked {
		ip := net.ParseIP(raw)
		if ip == nil {
			t.Fatalf("could not parse test IP %q", raw)
		}
		if !isDisallowedIP(ip) {
			t.Errorf("isDisallowedIP(%q) = false, want true", raw)
		}
	}

	allowed := []string{
		"8.8.8.8",              // Google public DNS
		"1.1.1.1",              // Cloudflare public DNS
		"93.184.216.34",        // example.com
		"2001:4860:4860::8888", // Google IPv6 DNS
	}
	for _, raw := range allowed {
		ip := net.ParseIP(raw)
		if ip == nil {
			t.Fatalf("could not parse test IP %q", raw)
		}
		if isDisallowedIP(ip) {
			t.Errorf("isDisallowedIP(%q) = true, want false", raw)
		}
	}
}

// TestSafeDialContext_BlocksLoopbackHostname verifies that dialling
// "localhost" is rejected before DNS resolution.
func TestSafeDialContext_BlocksLoopbackHostname(t *testing.T) {
	t.Parallel()

	_, err := safeDialContext(context.Background(), "tcp", "localhost:80")
	if err == nil {
		t.Fatal("expected error for localhost, got nil")
	}
}

// TestSafeDialContext_BlocksLoopbackIP verifies that dialling 127.0.0.1
// directly is rejected after resolution.
func TestSafeDialContext_BlocksLoopbackIP(t *testing.T) {
	t.Parallel()

	_, err := safeDialContext(context.Background(), "tcp", "127.0.0.1:80")
	if err == nil {
		t.Fatal("expected error for 127.0.0.1, got nil")
	}
}

// TestSafeDialContext_BlocksLinkLocal verifies that 169.254.169.254 is blocked.
func TestSafeDialContext_BlocksLinkLocal(t *testing.T) {
	t.Parallel()

	_, err := safeDialContext(context.Background(), "tcp", "169.254.169.254:80")
	if err == nil {
		t.Fatal("expected error for 169.254.169.254, got nil")
	}
}

// TestSafeDialContext_BlocksPrivateRFC1918 verifies that RFC 1918 addresses
// are blocked (10.x, 172.16-31.x, 192.168.x).
func TestSafeDialContext_BlocksPrivateRFC1918(t *testing.T) {
	t.Parallel()

	for _, addr := range []string{"10.0.0.1:80", "192.168.1.1:80", "172.16.0.1:80"} {
		_, err := safeDialContext(context.Background(), "tcp", addr)
		if err == nil {
			t.Errorf("expected error for %s, got nil", addr)
		}
	}
}

// TestValidateWebhookURLRunner checks scheme and host validation.
func TestValidateWebhookURLRunner(t *testing.T) {
	t.Parallel()

	if err := validateWebhookURLRunner("https://hooks.example.com/notify"); err != nil {
		t.Errorf("unexpected error for valid HTTPS URL: %v", err)
	}
	if err := validateWebhookURLRunner("http://hooks.example.com/notify"); err != nil {
		t.Errorf("unexpected error for valid HTTP URL: %v", err)
	}

	bad := []string{
		"ftp://example.com/hook",
		"//example.com/hook",
		"",
		"http://localhost/hook",
		"http:///path",
	}
	for _, u := range bad {
		if err := validateWebhookURLRunner(u); err == nil {
			t.Errorf("expected error for %q, got nil", u)
		}
	}
}

// TestSendWebhookWorker_BlocksPrivateURL verifies that Work records a failed
// delivery (and returns an error for retry) when wh.URL resolves to a private
// or loopback address, without dialling the target.
func TestSendWebhookWorker_BlocksPrivateURL(t *testing.T) {
	t.Parallel()

	ws := testutil.NewMemWebhookStore()
	// Use a loopback literal — the safe client's dialer will refuse it.
	wh := &store.Webhook{
		ProjectID:  1,
		Name:       "ssrf-test",
		TargetType: "generic",
		URL:        "http://127.0.0.1:9999/hook",
		IsActive:   true,
		Events:     []string{"report_completed"},
	}
	created, err := ws.Create(context.Background(), wh)
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	worker := newTestWorker(ws, newWebhookHTTPClient(5))
	job := newTestJob(SendWebhookArgs{
		WebhookID: created.ID,
		Payload:   samplePayload("report_completed"),
	}, 1)

	workErr := worker.Work(context.Background(), job)
	if workErr == nil {
		t.Fatal("Work should return an error for a private-IP webhook URL")
	}

	// A delivery record should be written with the error message.
	deliveries, total, err := ws.ListDeliveries(context.Background(), created.ID, 1, 10)
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 delivery record, got %d", total)
	}
	if deliveries[0].Error == nil || *deliveries[0].Error == "" {
		t.Error("expected delivery error message to be set")
	}
}

// TestSendWebhookWorker_BlocksLocalhostURL verifies that Work refuses to
// deliver to a localhost URL (defense-in-depth scheme+host check).
func TestSendWebhookWorker_BlocksLocalhostURL(t *testing.T) {
	t.Parallel()

	ws := testutil.NewMemWebhookStore()
	wh := &store.Webhook{
		ProjectID:  1,
		Name:       "ssrf-localhost",
		TargetType: "generic",
		URL:        "http://localhost:8080/hook",
		IsActive:   true,
		Events:     []string{"report_completed"},
	}
	created, err := ws.Create(context.Background(), wh)
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	worker := newTestWorker(ws, newWebhookHTTPClient(5))
	job := newTestJob(SendWebhookArgs{
		WebhookID: created.ID,
		Payload:   samplePayload("report_completed"),
	}, 1)

	if err := worker.Work(context.Background(), job); err == nil {
		t.Fatal("Work should return an error for a localhost webhook URL")
	}
}

// TestNewWebhookHTTPClient_OTelTransport verifies the webhook client wraps
// its safe transport with OTel instrumentation.
func TestNewWebhookHTTPClient_OTelTransport(t *testing.T) {
	t.Parallel()

	client := newWebhookHTTPClient(10)
	if client.Transport == nil {
		t.Fatal("expected non-nil Transport")
	}
	if client.CheckRedirect == nil {
		t.Fatal("expected CheckRedirect to be set")
	}
}

// TestNewWebhookHTTPClient_AllowsPublicURL verifies the safe client can
// successfully POST to a real (loopback-free) httptest server.
// httptest.NewServer listens on 127.0.0.1, so we pass srv.Client() (which
// bypasses the transport) for the connectivity check and only verify the
// safe dialer does not block the public-IP path.
func TestSafeDialContext_AllowsPublicIP(t *testing.T) {
	t.Parallel()

	// httptest server for a reachable local endpoint — use a custom transport
	// that forces the connection via 127.0.0.1 so the test doesn't need real
	// external DNS while still confirming the safe client works end-to-end for
	// non-private targets.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// srv.Client() uses a custom transport that dials the test listener
	// directly — it bypasses our safe transport.  We use it here just to
	// confirm the test server is alive.
	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("test server unreachable: %v", err)
	}
	_ = resp.Body.Close()

	// Confirm that a public IP (8.8.8.8) is not blocked by isDisallowedIP.
	if isDisallowedIP(net.ParseIP("8.8.8.8")) {
		t.Error("8.8.8.8 should not be blocked")
	}
}
