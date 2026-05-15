package mcp

import (
	"container/list"
	"fmt"
	"net/http"
	"sync"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"golang.org/x/time/rate"
)

const maxRateLimitEntries = 10_000

// RateLimiter is a per-identity token-bucket rate limiter. The identity key is
// derived from the api_key_id (for API-key auth) or user_id (for JWT auth)
// stored in the auth.TokenInfo injected by auth.RequireBearerToken.
//
// The internal map is bounded at maxRateLimitEntries entries using a simple
// LRU eviction policy so memory growth is predictable under adversarial load.
type RateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*lruEntry
	order    *list.List // front = most-recently used

	r     rate.Limit
	burst int
}

type lruEntry struct {
	key     string
	limiter *rate.Limiter
	elem    *list.Element
}

// NewRateLimiter creates a RateLimiter with a sustained rate of perMin
// requests per minute and the given burst size.
func NewRateLimiter(perMin, burst int) *RateLimiter {
	r := rate.Limit(float64(perMin) / 60.0)
	return &RateLimiter{
		limiters: make(map[string]*lruEntry, 128),
		order:    list.New(),
		r:        r,
		burst:    burst,
	}
}

// Middleware returns an HTTP middleware that enforces the rate limit.
// It must be called AFTER auth.RequireBearerToken so TokenInfo is present.
// On rate-limit exceeded it returns 429 with a Retry-After: 1 header.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := identityKey(r)
		limiter := rl.getLimiter(key)

		if !limiter.Allow() {
			w.Header().Set("Retry-After", "1")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (rl *RateLimiter) getLimiter(key string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if entry, ok := rl.limiters[key]; ok {
		rl.order.MoveToFront(entry.elem)
		return entry.limiter
	}

	// Evict LRU entry when at capacity.
	if len(rl.limiters) >= maxRateLimitEntries {
		back := rl.order.Back()
		if back != nil {
			evicted := back.Value.(*lruEntry)
			rl.order.Remove(back)
			delete(rl.limiters, evicted.key)
		}
	}

	lim := rate.NewLimiter(rl.r, rl.burst)
	entry := &lruEntry{key: key, limiter: lim}
	entry.elem = rl.order.PushFront(entry)
	rl.limiters[key] = entry
	return lim
}

// identityKey extracts the rate-limit key from the request context.
// Key is "apikey:<api_key_id>" for API-key auth, "user:<user_id>" for JWT auth.
// Falls back to "unknown" if TokenInfo is not present (should not happen in
// normal operation since auth middleware runs first).
func identityKey(r *http.Request) string {
	info := auth.TokenInfoFromContext(r.Context())
	if info == nil {
		return "unknown"
	}

	if id, ok := info.Extra["api_key_id"]; ok {
		if apiKeyID, ok := id.(int64); ok && apiKeyID != 0 {
			return fmt.Sprintf("apikey:%d", apiKeyID)
		}
	}

	if info.UserID != "" {
		return fmt.Sprintf("user:%s", info.UserID)
	}

	return "unknown"
}

// ReservationFor is exported for testing — returns the limiter for a given key
// so tests can inspect reservation state.
func (rl *RateLimiter) ReservationFor(key string) *rate.Reservation {
	lim := rl.getLimiter(key)
	return lim.Reserve()
}
