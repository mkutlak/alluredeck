package middleware

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// UserActiveCache caches the users.is_active flag for a short TTL so the auth
// middleware can enforce "deactivated users lose access" without paying a DB
// round-trip on every request. Cache misses are deduped via an inline
// singleflight to avoid stampedes when many requests hit at once for the same
// user.
//
// Lookups by ID (numeric JWT sub) and by email (API-key path) are both
// supported — they resolve to the same in-memory entry keyed by the
// stringified user_id, so a deactivation invalidates both paths together.
//
// Concurrency model: RWMutex for the map; an inline singleflight per key for
// the load (we roll our own ~25 LOC because the project rule forbids new
// third-party dependencies, and golang.org/x/sync is not vendored).
//
// Bounded growth: maxEntries cap. On insertion past the cap, the oldest entry
// (smallest expiresAt) is evicted. This is O(N) per insertion — fine for the
// expected ≤10k working-set size. A proper LRU is the next iteration if this
// becomes hot.
type UserActiveCache struct {
	store      store.UserStorer
	ttl        time.Duration
	maxEntries int
	nowFn      func() time.Time

	mu      sync.RWMutex
	entries map[string]userActiveEntry // keyed by stringified user.ID

	sfMu    sync.Mutex
	flights map[string]*sfCall
}

type userActiveEntry struct {
	isActive  bool
	expiresAt time.Time
}

// sfCall is an in-flight single-flight load. Mirrors
// golang.org/x/sync/singleflight.call: one goroutine performs the work and
// every concurrent caller for the same key waits on the same WaitGroup, then
// reads the shared result.
type sfCall struct {
	wg  sync.WaitGroup
	val bool
	err error
}

// NewUserActiveCache returns an initialised UserActiveCache. ttl<=0 falls back
// to 30s; maxEntries<=0 falls back to 10000.
func NewUserActiveCache(s store.UserStorer, ttl time.Duration, maxEntries int) *UserActiveCache {
	if ttl <= 0 {
		ttl = 30 * time.Second
	}
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	return &UserActiveCache{
		store:      s,
		ttl:        ttl,
		maxEntries: maxEntries,
		nowFn:      time.Now,
		entries:    make(map[string]userActiveEntry),
		flights:    make(map[string]*sfCall),
	}
}

// SetNowFn overrides the clock used for TTL expiry. Test-only helper.
func (c *UserActiveCache) SetNowFn(fn func() time.Time) {
	c.nowFn = fn
}

// IsActive returns the cached or freshly-loaded is_active value for the user
// identified by sub (the JWT sub claim — a stringified numeric ID for
// DB-backed users). For non-numeric sub (env users) returns (true, nil) — env
// users have no users row and are always considered active.
func (c *UserActiveCache) IsActive(ctx context.Context, sub string) (bool, error) {
	id, err := strconv.ParseInt(sub, 10, 64)
	if err != nil {
		// Env user (e.g. AdminUser/ViewerUser) — no users row exists; always active.
		return true, nil
	}
	return c.isActiveByID(ctx, id, sub)
}

// IsActiveByEmail mirrors IsActive but resolves the lookup via GetByEmail.
// Used by the API-key auth path where the JWT-sub indirection is absent.
// Cached by stringified user ID so subsequent IsActive(sub=user.ID) hits the
// same entry.
func (c *UserActiveCache) IsActiveByEmail(ctx context.Context, email string) (bool, error) {
	u, err := c.store.GetByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			// Unknown email → treat as inactive. Don't cache (no stable key).
			return false, nil
		}
		return false, fmt.Errorf("user_active_cache: get by email: %w", err)
	}
	key := strconv.FormatInt(u.ID, 10)
	c.storeEntry(key, u.IsActive)
	return u.IsActive, nil
}

// Invalidate purges the entry for the given key (stringified user ID). Safe
// to call with a key that is not cached. Optional — callers that issue an
// UpdateActive/Deactivate may invoke this for immediate effect; otherwise the
// TTL handles propagation within ttl seconds.
func (c *UserActiveCache) Invalidate(sub string) {
	c.mu.Lock()
	delete(c.entries, sub)
	c.mu.Unlock()
}

// isActiveByID is the core load path keyed by numeric ID.
func (c *UserActiveCache) isActiveByID(ctx context.Context, id int64, key string) (bool, error) {
	now := c.nowFn()

	// Fast path: read-locked cache hit.
	c.mu.RLock()
	if e, ok := c.entries[key]; ok && now.Before(e.expiresAt) {
		c.mu.RUnlock()
		return e.isActive, nil
	}
	c.mu.RUnlock()

	// Slow path: dedupe concurrent loads via inline singleflight.
	c.sfMu.Lock()
	if call, ok := c.flights[key]; ok {
		c.sfMu.Unlock()
		call.wg.Wait()
		return call.val, call.err
	}
	call := &sfCall{}
	call.wg.Add(1)
	c.flights[key] = call
	c.sfMu.Unlock()

	// Perform the load exactly once.
	call.val, call.err = c.load(ctx, id, key)
	call.wg.Done()

	c.sfMu.Lock()
	delete(c.flights, key)
	c.sfMu.Unlock()

	return call.val, call.err
}

// load fetches the user from the store and populates the cache.
// Behaviour:
//   - ErrUserNotFound → cache (false, nil) so we don't spam the DB for
//     deleted/never-existed IDs.
//   - any other error → propagate, do NOT cache (caller decides; auth
//     middleware fails open on transient DB error).
func (c *UserActiveCache) load(ctx context.Context, id int64, key string) (bool, error) {
	u, err := c.store.GetByID(ctx, id)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			c.storeEntry(key, false)
			return false, nil
		}
		return false, fmt.Errorf("user_active_cache: get by id %d: %w", id, err)
	}
	c.storeEntry(key, u.IsActive)
	return u.IsActive, nil
}

// storeEntry inserts or replaces a cache entry and evicts the oldest entry
// when the size cap is exceeded.
func (c *UserActiveCache) storeEntry(key string, isActive bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key] = userActiveEntry{
		isActive:  isActive,
		expiresAt: c.nowFn().Add(c.ttl),
	}

	// Bounded growth: linear scan to evict the oldest entry. The cap is a
	// safety net for pathological churn, not a hot-path optimisation.
	if len(c.entries) > c.maxEntries {
		var (
			oldestKey string
			oldestAt  time.Time
			first     = true
		)
		for k, e := range c.entries {
			if first || e.expiresAt.Before(oldestAt) {
				oldestKey = k
				oldestAt = e.expiresAt
				first = false
			}
		}
		delete(c.entries, oldestKey)
	}
}
