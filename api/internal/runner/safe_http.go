package runner

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// isDisallowedIP reports whether ip must be blocked for SSRF prevention.
// It rejects loopback, private (RFC 1918 / RFC 4193), link-local unicast,
// link-local multicast, and unspecified addresses, as well as IPv4-in-IPv6
// mapped equivalents (e.g. ::ffff:169.254.169.254).
//
// This is the single authoritative deny predicate used by both the safe
// dialer (enforcement at connect time) and validateWebhookURLRunner
// (defense-in-depth at delivery time).
// Note: internal/handlers.isPrivateIP is a parallel implementation at
// create/update time — keep the deny rules in sync when modifying either.
func isDisallowedIP(ip net.IP) bool {
	// Unwrap IPv4-mapped IPv6 addresses (::ffff:x.x.x.x) so the IPv4
	// methods (IsPrivate etc.) work correctly.
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified()
}

// safeDialContext is a DialContext function that:
//  1. Resolves the target hostname to IPs,
//  2. Rejects the connection if ANY resolved IP is disallowed (SSRF),
//  3. Pins the dial to the first checked IP (defeats DNS-rebinding).
func safeDialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, fmt.Errorf("ssrf-safe dialer: split host/port %q: %w", addr, err)
	}

	// Reject loopback hostnames before DNS resolution.
	lower := strings.ToLower(host)
	if lower == "localhost" {
		return nil, fmt.Errorf("ssrf-safe dialer: blocked loopback hostname %q", host)
	}

	// Resolve hostname to IPs.
	resolver := &net.Resolver{}
	addrs, err := resolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("ssrf-safe dialer: resolve %q: %w", host, err)
	}
	if len(addrs) == 0 {
		return nil, fmt.Errorf("ssrf-safe dialer: no addresses for %q", host)
	}

	// Check every resolved IP; reject if any is disallowed.
	for _, a := range addrs {
		if isDisallowedIP(a.IP) {
			return nil, fmt.Errorf("ssrf-safe dialer: blocked: %q resolves to disallowed IP %s", host, a.IP)
		}
	}

	// Pin the connection to the first validated IP so the checked IP is the
	// connected IP — this defeats DNS-rebinding (the hostname is not
	// re-resolved at dial time).
	pinnedAddr := net.JoinHostPort(addrs[0].IP.String(), port)
	d := &net.Dialer{}
	return d.DialContext(ctx, network, pinnedAddr)
}

// validateWebhookURLRunner performs a fast, scheme+host sanity check on a
// webhook URL before delivery (defense-in-depth; the dialer is the real
// enforcement layer).  It intentionally mirrors the logic in
// internal/handlers.validateWebhookURL without importing that package.
func validateWebhookURLRunner(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("webhook delivery blocked: invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("webhook delivery blocked: URL must use http or https scheme")
	}
	if u.Host == "" {
		return fmt.Errorf("webhook delivery blocked: URL must have a host")
	}
	// Reject localhost up front (same as handler).
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") {
		return fmt.Errorf("webhook delivery blocked: loopback hostname not allowed")
	}
	return nil
}

// safeCheckRedirect re-validates each redirect target using the same deny
// logic as the dialer.  It blocks redirects to private/loopback addresses
// and caps the redirect chain at 5 hops.
func safeCheckRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return fmt.Errorf("ssrf-safe client: stopped after 5 redirects")
	}
	if err := validateWebhookURLRunner(req.URL.String()); err != nil {
		return err
	}
	// Also resolve and check the redirect target IP.
	host := req.URL.Hostname()
	if host == "" {
		return fmt.Errorf("ssrf-safe client: redirect target has no host")
	}
	resolver := &net.Resolver{}
	addrs, err := resolver.LookupIPAddr(req.Context(), host)
	if err != nil {
		return fmt.Errorf("ssrf-safe client: redirect resolve %q: %w", host, err)
	}
	for _, a := range addrs {
		if isDisallowedIP(a.IP) {
			return fmt.Errorf("ssrf-safe client: redirect blocked: %q resolves to disallowed IP %s", host, a.IP)
		}
	}
	return nil
}

// newWebhookHTTPClient returns an *http.Client suitable for webhook delivery.
// It composes:
//   - A safe DialContext that resolves hostnames, rejects private/loopback IPs,
//     and pins the connection to a validated IP (SSRF + DNS-rebinding defence).
//   - A CheckRedirect that re-validates each hop with the same deny rules.
//   - OTel instrumentation (otelhttp.Transport wrapping the safe transport) so
//     every webhook POST produces a child trace span.
//   - A client-level timeout.
func newWebhookHTTPClient(timeout time.Duration) *http.Client {
	safeTransport := &http.Transport{
		DialContext: safeDialContext,
	}

	return &http.Client{
		Transport:     otelhttp.NewTransport(safeTransport),
		CheckRedirect: safeCheckRedirect,
		Timeout:       timeout,
	}
}
