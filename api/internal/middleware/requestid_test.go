package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRequestID reuses a client-sent X-Request-ID and otherwise generates a
// fresh UUID-shaped one; either way the ID is in the request context and
// echoed on the response.
func TestRequestID(t *testing.T) {
	t.Parallel()
	var ctxID string
	h := RequestID(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctxID = RequestIDFromContext(r.Context())
	}))
	serve := func(clientID string) string {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		if clientID != "" {
			req.Header.Set("X-Request-ID", clientID)
		}
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)
		if got := rr.Header().Get("X-Request-ID"); got != ctxID {
			t.Errorf("response ID %q differs from context ID %q", got, ctxID)
		}
		return ctxID
	}

	if got := serve("test-correlation-123"); got != "test-correlation-123" {
		t.Errorf("client ID not propagated: got %q", got)
	}
	id1, id2 := serve(""), serve("")
	if len(id1) != 36 || id1 == id2 {
		t.Errorf("generated IDs %q and %q: want distinct 36-char UUIDs", id1, id2)
	}
}
