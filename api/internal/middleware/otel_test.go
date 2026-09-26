package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// TestOTel records one span per request on the given provider, named by the
// matched ServeMux pattern, or by method and raw path when nothing matched.
func TestOTel(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/projects/{id}", okHandler)
	tests := []struct {
		name     string
		next     http.Handler
		path     string
		wantSpan string
	}{
		{"matched pattern", mux, "/api/v1/projects/42", "GET /api/v1/projects/{id}"},
		{"no pattern", http.HandlerFunc(okHandler), "/health", "GET /health"},
	}
	for _, tc := range tests {
		rec := tracetest.NewSpanRecorder()
		tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(rec))
		OTel(tp)(tc.next).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, tc.path, nil))

		spans := rec.Ended()
		if len(spans) != 1 || spans[0].Name() != tc.wantSpan {
			names := make([]string, len(spans))
			for i, s := range spans {
				names[i] = s.Name()
			}
			t.Errorf("%s: spans %v, want [%s]", tc.name, names, tc.wantSpan)
		}
	}
}
