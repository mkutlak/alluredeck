package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"

	"github.com/mkutlak/alluredeck/api/internal/logging"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
)

// TestLoggingMiddleware: behind RequestID, each request logs one "request
// completed" entry with request_id, method, path, the first status written
// (200 when the handler only writes a body) and a duration. The handler's
// context carries the request-scoped child logger.
func TestLoggingMiddleware(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		write      func(http.ResponseWriter)
		wantStatus int64
	}{
		{"explicit status", func(w http.ResponseWriter) { w.WriteHeader(http.StatusNotFound) }, http.StatusNotFound},
		{"body only defaults to 200", func(w http.ResponseWriter) { _, _ = w.Write([]byte("ok")) }, http.StatusOK},
		{"only the first WriteHeader counts", func(w http.ResponseWriter) {
			w.WriteHeader(http.StatusCreated)
			w.WriteHeader(http.StatusOK)
		}, http.StatusCreated},
	}
	for _, tc := range tests {
		core, logs := observer.New(zap.DebugLevel)
		handler := middleware.RequestID(middleware.LoggingMiddleware(zap.New(core))(
			http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				logging.FromContext(r.Context()).Info("inside handler")
				tc.write(w)
			})))
		req := httptest.NewRequest(http.MethodPost, "/api/v1/resource", nil)
		req.Header.Set("X-Request-ID", "test-req-123")
		handler.ServeHTTP(httptest.NewRecorder(), req)

		entries := logs.All()
		if len(entries) != 2 {
			t.Fatalf("%s: expected the handler's entry and the completion entry, got %d", tc.name, len(entries))
		}
		if got := entries[0].ContextMap()["request_id"]; got != "test-req-123" {
			t.Errorf("%s: context logger request_id = %v, want test-req-123", tc.name, got)
		}
		done, fields := entries[1], entries[1].ContextMap()
		if done.Message != "request completed" || fields["request_id"] != "test-req-123" ||
			fields["method"] != http.MethodPost || fields["path"] != "/api/v1/resource" || fields["status"] != tc.wantStatus {
			t.Errorf("%s: completion entry %q %v", tc.name, done.Message, fields)
		}
		if _, ok := fields["duration"]; !ok {
			t.Errorf("%s: completion entry has no duration", tc.name)
		}
	}
}
