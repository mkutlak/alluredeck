package mcp

import (
	"container/list"
	"fmt"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/auth"
	"golang.org/x/time/rate"
)

const maxRateLimitEntries = 10_000

// defaultToolCosts prices the tools that cost materially more to serve than a
// single indexed lookup, so one caller sweeping them cannot crowd out everyone
// else on a flat request budget. Tools absent from this map cost 1.
//
// Costs are relative, not absolute: they express how many ordinary requests a
// call is worth, and are deliberately coarse.
var defaultToolCosts = map[string]int{
	// Resolves a build, lists every failing test, and runs per-test triage
	// with a last-good lookup each — by far the heaviest tool.
	"diagnose_failure": 5,
	// Walks a test across many builds.
	"get_test_history": 3,
	// Full diff of two builds' result sets.
	"compare_builds": 3,
	// Scans recent failure messages for a substring match.
	"find_test_by_name": 2,
}

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
	costs map[string]int
}

type lruEntry struct {
	key     string
	limiter *rate.Limiter
	elem    *list.Element
}

// NewRateLimiter creates a RateLimiter with a sustained rate of perMin
// requests per minute and the given burst size, pricing tools with
// defaultToolCosts.
func NewRateLimiter(perMin, burst int) *RateLimiter {
	return NewRateLimiterWithCosts(perMin, burst, defaultToolCosts)
}

// NewRateLimiterWithCosts is the full constructor. costs maps a tool name to
// the number of requests a call to it is worth; nil falls back to
// defaultToolCosts, so an embedder cannot lose the built-in pricing by
// forgetting to set it.
func NewRateLimiterWithCosts(perMin, burst int, costs map[string]int) *RateLimiter {
	if costs == nil {
		costs = defaultToolCosts
	}
	r := rate.Limit(float64(perMin) / 60.0)
	return &RateLimiter{
		limiters: make(map[string]*lruEntry, 128),
		order:    list.New(),
		r:        r,
		burst:    burst,
		costs:    costs,
	}
}

// costOf returns the token cost of a request, read from the Mcp-Name header
// that MCP 2026-07-28 requires alongside the JSON-RPC body. Using the header
// keeps pricing out of the request-body parsing path entirely.
//
// The cost is clamped to the burst size: rate.Limiter rejects any AllowN
// larger than its burst outright, so an unclamped cost above burst would make
// the tool permanently unreachable rather than merely expensive.
func (rl *RateLimiter) costOf(r *http.Request) int {
	name := r.Header.Get("Mcp-Name")
	if name == "" {
		return 1
	}
	cost, ok := rl.costs[name]
	if !ok || cost < 1 {
		return 1
	}
	if rl.burst > 0 && cost > rl.burst {
		return rl.burst
	}
	return cost
}

// ParseToolCosts parses an MCP_TOOL_COSTS override of the form
// "diagnose_failure=5,compare_builds=3" into a cost map, layered over the
// built-in defaults so an operator can retune one tool without restating all
// of them. Malformed or non-positive entries are skipped rather than failing
// startup: a typo in a tuning knob should not take the server down.
func ParseToolCosts(raw string) map[string]int {
	costs := make(map[string]int, len(defaultToolCosts))
	maps.Copy(costs, defaultToolCosts)
	for pair := range strings.SplitSeq(raw, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		name, rawCost, ok := strings.Cut(pair, "=")
		if !ok {
			continue
		}
		cost, err := strconv.Atoi(strings.TrimSpace(rawCost))
		if err != nil || cost < 1 {
			continue
		}
		costs[strings.TrimSpace(name)] = cost
	}
	return costs
}

// Middleware returns an HTTP middleware that enforces the rate limit.
// It must be called AFTER auth.RequireBearerToken so TokenInfo is present.
// On rate-limit exceeded it returns 429 with a Retry-After: 1 header.
//
// Requests are charged per tool via the Mcp-Name header — see costOf.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := identityKey(r)
		limiter := rl.getLimiter(key)

		if !limiter.AllowN(time.Now(), rl.costOf(r)) {
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
