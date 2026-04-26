package observability_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/observability"
)

// ── helpers ────────────────────────────────────────────────────────────────

func disabledCfg() config.ObservabilityConfig {
	return config.ObservabilityConfig{Enabled: false}
}

func enabledCfg(addr string) config.ObservabilityConfig {
	return config.ObservabilityConfig{
		Enabled:     true,
		ServiceName: "test-svc",
		Environment: "test",
		Traces: config.TracesConfig{
			Protocol:    "http/protobuf",
			SampleRatio: 1.0,
		},
		Metrics: config.MetricsConfig{
			Enabled: true,
			Addr:    addr,
			Path:    "/metrics",
		},
	}
}

// ── Init (disabled) ────────────────────────────────────────────────────────

func TestInitDisabledReturnsNoopShutdown(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := zap.NewNop()

	shutdown, err := observability.Init(ctx, disabledCfg(), logger)
	if err != nil {
		t.Fatalf("Init(disabled): unexpected error: %v", err)
	}
	if shutdown == nil {
		t.Fatal("Init(disabled): shutdown func must not be nil")
	}
	// Calling shutdown must not error.
	if err := shutdown(ctx); err != nil {
		t.Errorf("shutdown(disabled): unexpected error: %v", err)
	}
}

func TestInitDisabledShutdownIsIdempotent(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	logger := zap.NewNop()

	shutdown, err := observability.Init(ctx, disabledCfg(), logger)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	// Call twice — must not error either time.
	for i := range 2 {
		if err := shutdown(ctx); err != nil {
			t.Errorf("shutdown call %d: unexpected error: %v", i+1, err)
		}
	}
}

// ── Init (enabled) ─────────────────────────────────────────────────────────

func TestInitEnabledStartsMetricsServer(t *testing.T) {
	// Not parallel — binds a real TCP port.
	addr := freeAddr(t)
	ctx := context.Background()
	logger := zap.NewNop()

	shutdown, err := observability.Init(ctx, enabledCfg(addr), logger)
	if err != nil {
		t.Fatalf("Init(enabled): %v", err)
	}
	t.Cleanup(func() { _ = shutdown(ctx) })

	// Poll until the metrics server is ready (up to 2 s).
	url := fmt.Sprintf("http://%s/metrics", addr)
	var resp *http.Response
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		var reqErr error
		resp, reqErr = http.Get(url) //nolint:noctx // test helper
		if reqErr == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if resp == nil {
		t.Fatalf("metrics server at %s did not become ready within 2s", url)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("GET /metrics: want 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	// Go runtime metrics are registered by the runtime instrumentation.
	// At minimum the response must be non-empty Prometheus text format.
	if len(body) == 0 {
		t.Error("GET /metrics: response body is empty")
	}
}

func TestInitEnabledShutdownStopsServer(t *testing.T) {
	addr := freeAddr(t)
	ctx := context.Background()
	logger := zap.NewNop()

	shutdown, err := observability.Init(ctx, enabledCfg(addr), logger)
	if err != nil {
		t.Fatalf("Init(enabled): %v", err)
	}

	// Wait for server to be up.
	url := fmt.Sprintf("http://%s/metrics", addr)
	waitForHTTP(t, url, 2*time.Second)

	// Shutdown.
	if err := shutdown(ctx); err != nil {
		t.Errorf("shutdown: %v", err)
	}

	// Server must no longer accept connections.
	time.Sleep(50 * time.Millisecond)
	resp, getErr := http.Get(url) //nolint:noctx // test helper
	if getErr == nil {
		_ = resp.Body.Close()
		t.Error("server still responds after shutdown")
	}
}

// ── SampleRatio clamping ───────────────────────────────────────────────────

func TestSampleRatioClamping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input float64
		want  float64
	}{
		{-0.5, 0.0},
		{0.0, 0.0},
		{0.5, 0.5},
		{1.0, 1.0},
		{1.5, 1.0},
	}
	for _, tc := range cases {
		got := observability.ClampSampleRatio(tc.input)
		if got != tc.want {
			t.Errorf("ClampSampleRatio(%v) = %v, want %v", tc.input, got, tc.want)
		}
	}
}

// ── Zap trace-injecting core ───────────────────────────────────────────────

func TestTraceCoreCopiesFieldsWhenNoSpan(t *testing.T) {
	t.Parallel()
	// When no span is active in the context, the core must write the entry
	// without adding trace_id / span_id fields.
	core, logs := observer.New(zapcore.DebugLevel)
	wrapped := observability.NewTraceCore(core)

	logger := zap.New(wrapped)
	logger.Info("hello")

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	for _, f := range entries[0].Context {
		if f.Key == "trace_id" || f.Key == "span_id" {
			t.Errorf("unexpected field %q when no span active", f.Key)
		}
	}
}

func TestTraceCoreInjectsTraceIDWhenSpanActive(t *testing.T) {
	t.Parallel()
	// Start a real SDK span so the context carries a valid trace/span ID.
	ctx := context.Background()

	// Initialize with a no-op (disabled) observability so the global providers
	// are set to noop but we can still test the zapcore wrapper independently
	// by manually starting a span via the SDK test tracer.
	core, logs := observer.New(zapcore.DebugLevel)
	wrapped := observability.NewTraceCore(core)

	// Use the test tracer helper from the observability package.
	ctx, spanID, traceID := observability.StartTestSpan(ctx)
	defer observability.EndTestSpan(ctx)

	logger := zap.New(wrapped)
	// Log with context — the core must pick up the active span.
	observability.LogWithContext(logger, ctx, "hello from span")

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}

	fieldMap := make(map[string]string)
	for _, f := range entries[0].Context {
		if f.Type == zapcore.StringType {
			fieldMap[f.Key] = f.String
		}
	}

	if got := fieldMap["trace_id"]; got != traceID {
		t.Errorf("trace_id: want %q, got %q", traceID, got)
	}
	if got := fieldMap["span_id"]; got != spanID {
		t.Errorf("span_id: want %q, got %q", spanID, got)
	}
}

// ── helpers ────────────────────────────────────────────────────────────────

// freeAddr returns a random available TCP address "127.0.0.1:port".
func freeAddr(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freeAddr: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	return addr
}

// waitForHTTP polls url until it returns 200 or the deadline passes.
func waitForHTTP(t *testing.T, url string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url) //nolint:noctx // test helper
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("waitForHTTP: %s not reachable within %s", url, timeout)
}
