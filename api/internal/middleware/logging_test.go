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

// newObservedLogger creates a zap logger that records log entries for assertions.
func newObservedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zap.DebugLevel)
	return zap.New(core), logs
}

func TestLoggingMiddlewareLogsRequestCompletion(t *testing.T) {
	logger, logs := newObservedLogger()

	handler := middleware.LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if logs.Len() == 0 {
		t.Fatal("expected at least one log entry, got none")
	}

	entry := logs.All()[0]
	if entry.Message != "request completed" {
		t.Errorf("expected message %q, got %q", "request completed", entry.Message)
	}

	fields := entry.ContextMap()
	if fields["method"] != "GET" {
		t.Errorf("expected method=GET, got %v", fields["method"])
	}
	if fields["path"] != "/api/v1/health" {
		t.Errorf("expected path=/api/v1/health, got %v", fields["path"])
	}
	if _, ok := fields["status"]; !ok {
		t.Error("expected status field in log entry")
	}
	if _, ok := fields["duration"]; !ok {
		t.Error("expected duration field in log entry")
	}
}

func TestLoggingMiddlewareIncludesRequestID(t *testing.T) {
	logger, logs := newObservedLogger()

	// Wrap with RequestID middleware so the ID is in context before logging
	handler := middleware.RequestID(
		middleware.LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})),
	)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Request-ID", "test-req-123")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if logs.Len() == 0 {
		t.Fatal("expected log entry")
	}
	fields := logs.All()[0].ContextMap()
	if fields["request_id"] != "test-req-123" {
		t.Errorf("expected request_id=test-req-123, got %v", fields["request_id"])
	}
}

func TestLoggingMiddlewareStoresChildLoggerInContext(t *testing.T) {
	logger, _ := newObservedLogger()

	var loggerInCtx *zap.Logger
	handler := middleware.LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		loggerInCtx = logging.FromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if loggerInCtx == nil {
		t.Fatal("expected logger in context, got nil")
	}
}

func TestLoggingMiddlewareStatusCode(t *testing.T) {
	logger, logs := newObservedLogger()

	handler := middleware.LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	fields := logs.All()[0].ContextMap()
	if got := fields["status"]; got != int64(http.StatusNotFound) {
		t.Errorf("expected status=%d, got %v", http.StatusNotFound, got)
	}
}

func TestLoggingMiddlewareDefaultStatus200(t *testing.T) {
	logger, logs := newObservedLogger()

	// Handler writes body but never calls WriteHeader explicitly
	handler := middleware.LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	fields := logs.All()[0].ContextMap()
	if got := fields["status"]; got != int64(http.StatusOK) {
		t.Errorf("expected default status=200, got %v", got)
	}
}

func TestLoggingMiddlewareOnlyFirstWriteHeaderCounts(t *testing.T) {
	logger, logs := newObservedLogger()

	handler := middleware.LoggingMiddleware(logger)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.WriteHeader(http.StatusOK) // second call must be ignored
	}))

	req := httptest.NewRequest(http.MethodPost, "/resource", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	fields := logs.All()[0].ContextMap()
	if got := fields["status"]; got != int64(http.StatusCreated) {
		t.Errorf("expected status=201 (first call), got %v", got)
	}
}
