package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIPRateLimiter_Allow(t *testing.T) {
	rl := NewIPRateLimiter(2, 2, 5*time.Minute, false) // 2 req/s, burst 2

	ip := "192.168.1.1"

	// First two requests should be allowed (burst)
	if !rl.Allow(ip) {
		t.Error("expected first request to be allowed")
	}
	if !rl.Allow(ip) {
		t.Error("expected second request to be allowed (burst)")
	}

	// Third request should be blocked (burst exhausted, no time to refill)
	if rl.Allow(ip) {
		t.Error("expected third request to be blocked")
	}
}

func TestIPRateLimiter_IPIsolation(t *testing.T) {
	rl := NewIPRateLimiter(1, 1, 5*time.Minute, false) // 1 req/s, burst 1

	// Exhaust IP A
	if !rl.Allow("10.0.0.1") {
		t.Error("expected first request from IP A to be allowed")
	}
	if rl.Allow("10.0.0.1") {
		t.Error("expected second request from IP A to be blocked")
	}

	// IP B should still be allowed
	if !rl.Allow("10.0.0.2") {
		t.Error("expected first request from IP B to be allowed")
	}
}

func TestIPRateLimiter_Refill(t *testing.T) {
	rl := NewIPRateLimiter(1000, 1, 5*time.Minute, false) // high rate so refill is fast

	ip := "10.0.0.1"
	if !rl.Allow(ip) {
		t.Error("expected first request allowed")
	}
	if rl.Allow(ip) {
		t.Error("expected second request blocked")
	}

	// Wait for token refill
	time.Sleep(5 * time.Millisecond)

	if !rl.Allow(ip) {
		t.Error("expected request allowed after refill")
	}
}

func TestIPRateLimiter_Cleanup(t *testing.T) {
	rl := NewIPRateLimiter(10, 10, 10*time.Millisecond, false) // very short TTL

	rl.Allow("10.0.0.1")
	rl.Allow("10.0.0.2")

	// Wait for entries to become stale
	time.Sleep(20 * time.Millisecond)
	rl.Cleanup()

	rl.mu.RLock()
	count := len(rl.clients)
	rl.mu.RUnlock()

	if count != 0 {
		t.Errorf("expected 0 entries after cleanup, got %d", count)
	}
}

func TestRateLimitMiddleware_NormalTraffic(t *testing.T) {
	rl := NewIPRateLimiter(10, 10, 5*time.Minute, false)

	handler := RateLimitMiddleware(rl)(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestRateLimitMiddleware_ExcessiveRequests(t *testing.T) {
	rl := NewIPRateLimiter(1, 2, 5*time.Minute, false) // burst 2

	handler := RateLimitMiddleware(rl)(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Exhaust burst
	for i := range 2 {
		req := httptest.NewRequest(http.MethodPost, "/login", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Errorf("request %d: expected 200, got %d", i+1, rr.Code)
		}
	}

	// Next request should be rate limited
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d", rr.Code)
	}

	// Check Retry-After header
	if rr.Header().Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}

	// Check JSON response body matches project conventions
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("expected valid JSON response: %v", err)
	}
	meta, ok := resp["metadata"].(map[string]any)
	if !ok {
		t.Fatal("expected metadata in response")
	}
	if _, ok := meta["message"].(string); !ok {
		t.Error("expected message string in metadata")
	}
}

// TestRateLimitMiddleware_XForwardedFor_Untrusted verifies that when
// trustForwardedFor is false, X-Forwarded-For is ignored and rate limiting
// is based on RemoteAddr only.
func TestRateLimitMiddleware_XForwardedFor_Untrusted(t *testing.T) {
	rl := NewIPRateLimiter(1, 1, 5*time.Minute, false) // untrusted

	handler := RateLimitMiddleware(rl)(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// First request — allowed (burst=1)
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// Second request with different XFF but same RemoteAddr → should be 429
	// (XFF is ignored, so both requests count against the same IP)
	req2 := httptest.NewRequest(http.MethodPost, "/login", nil)
	req2.RemoteAddr = "10.0.0.1:12345"
	req2.Header.Set("X-Forwarded-For", "198.51.100.1")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 when XFF untrusted and same RemoteAddr, got %d", rr2.Code)
	}
}

// TestRateLimitMiddleware_XForwardedFor_Trusted verifies that when
// trustForwardedFor is true, different XFF IPs get separate rate-limit buckets.
func TestRateLimitMiddleware_XForwardedFor_Trusted(t *testing.T) {
	rl := NewIPRateLimiter(1, 1, 5*time.Minute, true) // trusted

	handler := RateLimitMiddleware(rl)(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// First request from forwarded IP A — allowed
	req := httptest.NewRequest(http.MethodPost, "/login", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// Second request from different forwarded IP B — allowed (different bucket)
	req2 := httptest.NewRequest(http.MethodPost, "/login", nil)
	req2.RemoteAddr = "10.0.0.1:12345"
	req2.Header.Set("X-Forwarded-For", "198.51.100.1")
	rr2 := httptest.NewRecorder()
	handler.ServeHTTP(rr2, req2)
	if rr2.Code != http.StatusOK {
		t.Errorf("expected 200 for different forwarded IP, got %d", rr2.Code)
	}

	// Third request from forwarded IP A again — should be 429
	req3 := httptest.NewRequest(http.MethodPost, "/login", nil)
	req3.RemoteAddr = "10.0.0.1:12345"
	req3.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")
	rr3 := httptest.NewRecorder()
	handler.ServeHTTP(rr3, req3)
	if rr3.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429 for same forwarded IP, got %d", rr3.Code)
	}
}

func TestStartCleanup_StopsOnDone(t *testing.T) {
	rl := NewIPRateLimiter(10, 10, 10*time.Millisecond, false)
	rl.Allow("10.0.0.1")

	done := make(chan struct{})
	rl.StartCleanup(50*time.Millisecond, done)

	// Wait for at least one cleanup cycle
	time.Sleep(80 * time.Millisecond)

	close(done)

	rl.mu.RLock()
	count := len(rl.clients)
	rl.mu.RUnlock()

	if count != 0 {
		t.Errorf("expected 0 entries after cleanup cycle, got %d", count)
	}
}
