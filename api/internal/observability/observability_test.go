package observability_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/observability"
)

// TestInitEnabled initialises twice in one process, as a second test run with
// -count does, and expects each to serve metrics. Not parallel: it binds a real
// TCP port.
func TestInitEnabled(t *testing.T) {
	for i := range 2 {
		t.Run(fmt.Sprintf("init %d", i+1), initEnabled)
	}
}

// initEnabled serves Prometheus metrics, Go and process series included, on the
// configured address until shutdown, which closes the server.
func initEnabled(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pick a free port: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close()

	ctx := context.Background()
	shutdown, err := observability.Init(ctx, config.ObservabilityConfig{
		Enabled:     true,
		ServiceName: "test-svc",
		Environment: "test",
		Traces:      config.TracesConfig{Protocol: "http/protobuf", SampleRatio: 1.0},
		Metrics:     config.MetricsConfig{Enabled: true, Addr: addr, Path: "/metrics"},
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("Init(enabled): %v", err)
	}
	t.Cleanup(func() { _ = shutdown(ctx) })

	url := fmt.Sprintf("http://%s/metrics", addr)
	var resp *http.Response
	for deadline := time.Now().Add(2 * time.Second); resp == nil; time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("metrics server at %s did not become ready within 2s", url)
		}
		resp, _ = http.Get(url) //nolint:noctx // test helper
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK || len(body) == 0 {
		t.Errorf("GET /metrics: status %d with %d body bytes, want 200 with metrics", resp.StatusCode, len(body))
	}
	for _, series := range []string{"go_goroutines", "process_cpu_seconds_total"} {
		if !strings.Contains(string(body), series) {
			t.Errorf("GET /metrics has no %s series", series)
		}
	}

	if err := shutdown(ctx); err != nil {
		t.Errorf("shutdown: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	if resp, err := http.Get(url); err == nil { //nolint:noctx // test helper
		_ = resp.Body.Close()
		t.Error("server still responds after shutdown")
	}
}

func TestSampleRatioClamping(t *testing.T) {
	t.Parallel()
	for input, want := range map[float64]float64{-0.5: 0.0, 0.5: 0.5, 1.5: 1.0} {
		if got := observability.ClampSampleRatio(input); got != want {
			t.Errorf("ClampSampleRatio(%v) = %v, want %v", input, got, want)
		}
	}
}
