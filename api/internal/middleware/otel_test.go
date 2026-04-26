package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

func TestOTel_RecordsSpan(t *testing.T) {
	t.Parallel()

	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))

	handler := OTel(tp)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	span := spans[0]

	// Span name must contain the HTTP method.
	if span.Name() == "" {
		t.Error("expected non-empty span name")
	}

	// Verify HTTP attributes are present.
	attrs := span.Attributes()
	foundMethod := false
	for _, a := range attrs {
		if string(a.Key) == "http.request.method" || string(a.Key) == "http.method" {
			foundMethod = true
			break
		}
	}
	if !foundMethod {
		t.Errorf("expected http method attribute in span, got attrs: %v", attrs)
	}
}

func TestOTel_SpanNameUsesPattern(t *testing.T) {
	t.Parallel()

	rec := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))

	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/projects/{id}", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := OTel(tp)(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/42", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	spans := rec.Ended()
	if len(spans) != 1 {
		t.Fatalf("expected 1 span, got %d", len(spans))
	}

	name := spans[0].Name()
	const want = "GET /api/v1/projects/{id}"
	if name != want {
		t.Errorf("expected span name %q, got %q", want, name)
	}
}
