package observability_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/observability"
)

// TestInitEnabled serves Prometheus metrics on the configured address until
// shutdown, which closes the server. Not parallel: it binds a real TCP port.
func TestInitEnabled(t *testing.T) {
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

// TestTraceCore adds trace_id and span_id to entries whose "ctx" field
// carries a valid span, and leaves entries without one unchanged.
func TestTraceCore(t *testing.T) {
	t.Parallel()
	core, logs := observer.New(zapcore.DebugLevel)
	logger := zap.New(observability.NewTraceCore(core))

	const traceID, spanID = "4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7"
	tid, terr := trace.TraceIDFromHex(traceID)
	sid, serr := trace.SpanIDFromHex(spanID)
	if terr != nil || serr != nil {
		t.Fatalf("span ids: %v, %v", terr, serr)
	}
	ctx := trace.ContextWithSpanContext(context.Background(), trace.NewSpanContext(trace.SpanContextConfig{TraceID: tid, SpanID: sid}))
	logger.Info("no span", zap.Any("ctx", context.Background()))
	logger.Info("in span", zap.Any("ctx", ctx))

	entries := logs.All()
	if len(entries) != 2 {
		t.Fatalf("expected 2 log entries, got %d", len(entries))
	}
	for i, want := range []map[string]string{{}, {"trace_id": traceID, "span_id": spanID}} {
		got := map[string]string{}
		for _, f := range entries[i].Context {
			if f.Key == "trace_id" || f.Key == "span_id" {
				got[f.Key] = f.String
			}
		}
		if fmt.Sprint(got) != fmt.Sprint(want) {
			t.Errorf("entry %q trace fields = %v, want %v", entries[i].Message, got, want)
		}
	}
}
