package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequestID_GeneratesWhenMissing(t *testing.T) {
	t.Parallel()
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify the ID is available in the request context
		id := RequestIDFromContext(r.Context())
		if id == "" {
			t.Error("expected request ID in context, got empty string")
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// Response must contain X-Request-ID header
	respID := rr.Header().Get("X-Request-ID")
	if respID == "" {
		t.Error("expected X-Request-ID in response header")
	}
	if len(respID) != 36 {
		t.Errorf("expected UUID-length (36 chars), got %d: %q", len(respID), respID)
	}
}

func TestRequestID_PropagatesExisting(t *testing.T) {
	t.Parallel()
	const clientID = "test-correlation-123"

	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := RequestIDFromContext(r.Context())
		if id != clientID {
			t.Errorf("expected context ID %q, got %q", clientID, id)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", clientID)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	respID := rr.Header().Get("X-Request-ID")
	if respID != clientID {
		t.Errorf("expected propagated ID %q, got %q", clientID, respID)
	}
}

func TestRequestID_UniquePerRequest(t *testing.T) {
	t.Parallel()
	handler := RequestID(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req1 := httptest.NewRequest(http.MethodGet, "/", nil)
	rr1 := httptest.NewRecorder()
	handler.ServeHTTP(rr1, req1)

	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)

	id1 := rr1.Header().Get("X-Request-ID")
	id2 := rr2.Header().Get("X-Request-ID")

	if id1 == id2 {
		t.Errorf("expected unique IDs per request, both got %q", id1)
	}
}
