package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestIPRateLimiter_Allow: each IP gets its own bucket of burst tokens.
func TestIPRateLimiter_Allow(t *testing.T) {
	rl := NewIPRateLimiter(2, 2, 5*time.Minute, false) // 2 req/s, burst 2
	for i, want := range []bool{true, true, false} {
		if got := rl.Allow("192.168.1.1"); got != want {
			t.Errorf("request %d: Allow = %v, want %v", i+1, got, want)
		}
	}
	if !rl.Allow("192.168.1.2") {
		t.Error("an exhausted IP must not throttle another IP")
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

// TestStartCleanup_RemovesStaleEntries: the background loop runs Cleanup,
// which drops entries idle longer than the TTL.
func TestStartCleanup_RemovesStaleEntries(t *testing.T) {
	rl := NewIPRateLimiter(10, 10, 10*time.Millisecond, false)
	rl.Allow("10.0.0.1")
	rl.Allow("10.0.0.2")

	done := make(chan struct{})
	rl.StartCleanup(50*time.Millisecond, done)
	time.Sleep(80 * time.Millisecond) // at least one cleanup cycle
	close(done)

	rl.mu.RLock()
	count := len(rl.clients)
	rl.mu.RUnlock()
	if count != 0 {
		t.Errorf("expected 0 entries after cleanup cycle, got %d", count)
	}
}

// TestRateLimitMiddleware sends each row's requests from one RemoteAddr with
// the given X-Forwarded-For. XFF picks the bucket only when trusted, and then
// by its leftmost (client) address. A 429 carries Retry-After and the JSON
// error envelope.
func TestRateLimitMiddleware(t *testing.T) {
	type req struct {
		xff  string
		want int
	}
	tests := []struct {
		name  string
		trust bool
		burst int
		reqs  []req
	}{
		{"burst then 429", false, 2, []req{{"", http.StatusOK}, {"", http.StatusOK}, {"", http.StatusTooManyRequests}}},
		{"untrusted XFF is ignored", false, 1, []req{{"203.0.113.50", http.StatusOK}, {"198.51.100.1", http.StatusTooManyRequests}}},
		{"trusted XFF keys by the client address", true, 1, []req{
			{"203.0.113.50, 70.41.3.18", http.StatusOK},
			{"198.51.100.1", http.StatusOK},
			{"203.0.113.50, 70.41.3.18", http.StatusTooManyRequests},
		}},
	}
	for _, tc := range tests {
		handler := RateLimitMiddleware(NewIPRateLimiter(1, tc.burst, 5*time.Minute, tc.trust))(okHandler)
		for i, r := range tc.reqs {
			hr := httptest.NewRequest(http.MethodPost, "/login", nil)
			hr.RemoteAddr = "10.0.0.1:12345"
			if r.xff != "" {
				hr.Header.Set("X-Forwarded-For", r.xff)
			}
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, hr)
			if rr.Code != r.want {
				t.Errorf("%s: request %d got %d, want %d", tc.name, i+1, rr.Code, r.want)
			}
			if rr.Code != http.StatusTooManyRequests {
				continue
			}
			var resp struct {
				Metadata struct{ Message string } `json:"metadata"`
			}
			if rr.Header().Get("Retry-After") == "" || json.Unmarshal(rr.Body.Bytes(), &resp) != nil || resp.Metadata.Message == "" {
				t.Errorf("%s: 429 lacks Retry-After or the error envelope: %v %s", tc.name, rr.Header(), rr.Body)
			}
		}
	}
}
