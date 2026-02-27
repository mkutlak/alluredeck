package middleware

import (
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ipLimiter tracks the token bucket state for a single IP address.
type ipLimiter struct {
	tokens   float64
	lastSeen time.Time
}

// IPRateLimiter implements a per-IP token bucket rate limiter using only stdlib.
type IPRateLimiter struct {
	mu      sync.RWMutex
	clients map[string]*ipLimiter
	rate    float64       // tokens added per second
	burst   int           // max tokens (bucket capacity)
	ttl     time.Duration // stale entry lifetime
}

// NewIPRateLimiter creates a rate limiter with the given rate (tokens/sec),
// burst capacity, and TTL for stale entry cleanup.
func NewIPRateLimiter(rate float64, burst int, ttl time.Duration) *IPRateLimiter {
	return &IPRateLimiter{
		clients: make(map[string]*ipLimiter),
		rate:    rate,
		burst:   burst,
		ttl:     ttl,
	}
}

// Allow checks whether a request from the given IP is permitted.
// Returns true if allowed, false if rate limited.
func (rl *IPRateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	limiter, exists := rl.clients[ip]
	if !exists {
		rl.clients[ip] = &ipLimiter{
			tokens:   float64(rl.burst) - 1,
			lastSeen: now,
		}
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(limiter.lastSeen).Seconds()
	limiter.tokens += elapsed * rl.rate
	if limiter.tokens > float64(rl.burst) {
		limiter.tokens = float64(rl.burst)
	}
	limiter.lastSeen = now

	if limiter.tokens < 1 {
		return false
	}

	limiter.tokens--
	return true
}

// Cleanup removes entries that haven't been seen within the TTL.
func (rl *IPRateLimiter) Cleanup() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cutoff := time.Now().Add(-rl.ttl)
	for ip, limiter := range rl.clients {
		if limiter.lastSeen.Before(cutoff) {
			delete(rl.clients, ip)
		}
	}
}

// StartCleanup runs Cleanup on a periodic interval until done is closed.
func (rl *IPRateLimiter) StartCleanup(interval time.Duration, done <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				rl.Cleanup()
			}
		}
	}()
}

// clientIP extracts the client IP from X-Forwarded-For (first entry) or RemoteAddr.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For: client, proxy1, proxy2 — take the leftmost (client) IP
		ip := strings.TrimSpace(strings.SplitN(xff, ",", 2)[0])
		if ip != "" {
			return ip
		}
	}

	// RemoteAddr is "IP:port" — strip the port
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// RateLimitMiddleware wraps a handler and rejects requests that exceed the rate limit.
// Returns 429 Too Many Requests with a Retry-After header and JSON body.
func RateLimitMiddleware(rl *IPRateLimiter) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ip := clientIP(r)
			if !rl.Allow(ip) {
				w.Header().Set("Content-Type", "application/json")
				w.Header().Set("Retry-After", "1")
				w.WriteHeader(http.StatusTooManyRequests)
				_ = json.NewEncoder(w).Encode(map[string]any{
					"metadata": map[string]string{"message": "Too many requests, please try again later"},
				})
				return
			}
			next(w, r)
		}
	}
}
