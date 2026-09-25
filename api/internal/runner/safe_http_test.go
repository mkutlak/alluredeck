package runner

import (
	"context"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestIsDisallowedIP checks the SSRF deny predicate against known blocked and
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
	allowed := []string{
		"8.8.8.8",              // Google public DNS
		"1.1.1.1",              // Cloudflare public DNS
		"93.184.216.34",        // example.com
		"2001:4860:4860::8888", // Google IPv6 DNS
	}
	for want, ips := range map[bool][]string{true: blocked, false: allowed} {
		for _, raw := range ips {
			ip := net.ParseIP(raw)
			if ip == nil {
				t.Fatalf("could not parse test IP %q", raw)
			}
			if got := isDisallowedIP(ip); got != want {
				t.Errorf("isDisallowedIP(%q) = %v, want %v", raw, got, want)
			}
		}
	}
}

// TestSafeDialContext_BlocksDisallowedTargets verifies the dialer refuses
// loopback hostnames before DNS and loopback / link-local / RFC 1918 targets
// after resolution — with its own SSRF error, not a network failure.
func TestSafeDialContext_BlocksDisallowedTargets(t *testing.T) {
	t.Parallel()
	for _, addr := range []string{"localhost:80", "127.0.0.1:80", "169.254.169.254:80", "10.0.0.1:80", "192.168.1.1:80", "172.16.0.1:80"} {
		if _, err := safeDialContext(context.Background(), "tcp", addr); err == nil || !strings.Contains(err.Error(), "blocked") {
			t.Errorf("safeDialContext(%s) error = %v, want an SSRF block", addr, err)
		}
	}
}

// TestValidateWebhookURLRunner checks scheme and host validation.
func TestValidateWebhookURLRunner(t *testing.T) {
	t.Parallel()

	for _, u := range []string{"https://hooks.example.com/notify", "http://hooks.example.com/notify"} {
		if err := validateWebhookURLRunner(u); err != nil {
			t.Errorf("unexpected error for %q: %v", u, err)
		}
	}
	for _, u := range []string{"ftp://example.com/hook", "//example.com/hook", "", "http://localhost/hook", "http:///path"} {
		if err := validateWebhookURLRunner(u); err == nil {
			t.Errorf("expected error for %q, got nil", u)
		}
	}
}

// TestNewWebhookHTTPClient_RedirectGuard verifies the webhook client re-checks
// every redirect hop, so a public URL cannot bounce delivery to an internal one.
func TestNewWebhookHTTPClient_RedirectGuard(t *testing.T) {
	t.Parallel()

	client := newWebhookHTTPClient(10 * time.Second)
	if client.CheckRedirect == nil {
		t.Fatal("expected CheckRedirect to be set")
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "http://169.254.169.254/latest/meta-data", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.CheckRedirect(req, nil); err == nil {
		t.Error("redirect to the metadata endpoint must be refused")
	}
}

// TestSendWebhookWorker_BlocksInternalURL verifies Work records a failed
// delivery — and returns an error for retry — when the webhook URL is a
// loopback literal (refused by the safe dialer) or localhost (refused by the
// defense-in-depth URL check), without delivering to the target.
func TestSendWebhookWorker_BlocksInternalURL(t *testing.T) {
	t.Parallel()

	for _, url := range []string{"http://127.0.0.1:9999/hook", "http://localhost:8080/hook"} {
		ws := testutil.NewMemWebhookStore()
		created, err := ws.Create(context.Background(), &store.Webhook{
			ProjectID: 1, Name: "ssrf-test", TargetType: "generic", URL: url, IsActive: true, Events: []string{"report_completed"},
		})
		if err != nil {
			t.Fatalf("create webhook: %v", err)
		}

		worker := newTestWorker(ws, newWebhookHTTPClient(5*time.Second))
		job := newTestJob(SendWebhookArgs{WebhookID: created.ID, Payload: samplePayload("report_completed")}, 1)
		if err := worker.Work(context.Background(), job); err == nil {
			t.Errorf("%s: Work should return an error", url)
		}

		deliveries, total, err := ws.ListDeliveries(context.Background(), created.ID, 1, 10)
		if err != nil {
			t.Fatalf("list deliveries: %v", err)
		}
		if total != 1 || deliveries[0].Error == nil || !strings.Contains(*deliveries[0].Error, "blocked") {
			t.Errorf("%s: want one delivery recording the SSRF block, got %d: %+v", url, total, deliveries)
		}
	}
}
