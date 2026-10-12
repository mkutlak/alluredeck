package middleware

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
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
// OIDC access tokens, whose sub is an email, are cached under a separate
// oidcKey entry that InvalidateOIDC purges.
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
	envUsers   map[string]bool // configured env-user names: active when no users row has the name

	mu      sync.RWMutex
	entries map[string]userActiveEntry // keyed by stringified user.ID or oidcKey(email)

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

// UserActiveTTL is how long the API and MCP servers trust a cached is_active
// flag. Neither process hears of a deactivation made in another (the MCP
// server shares no cache with the API, and each API replica has its own), so
// this is how long a deactivated user keeps access there.
const UserActiveTTL = 2 * time.Second

// userActiveLoadTimeout bounds a user lookup. Every caller that missed the
// cache for the same key waits on one lookup, so it does not stop when the
// caller that started it is cancelled.
const userActiveLoadTimeout = 5 * time.Second

// NewUserActiveCache returns an initialised UserActiveCache. ttl<=0 falls back
// to UserActiveTTL; maxEntries<=0 falls back to 10000. envUsers are the usernames of the
// env-configured users (ADMIN_USER, VIEWER_USER): they have no users row, so an
// API key held by one is always active. Empty names are ignored.
func NewUserActiveCache(s store.UserStorer, ttl time.Duration, maxEntries int, envUsers ...string) *UserActiveCache {
	if ttl <= 0 {
		ttl = UserActiveTTL
	}
	if maxEntries <= 0 {
		maxEntries = 10000
	}
	names := make(map[string]bool, len(envUsers))
	for _, name := range envUsers {
		if name != "" {
			names[name] = true
		}
	}
	return &UserActiveCache{
		store:      s,
		ttl:        ttl,
		maxEntries: maxEntries,
		nowFn:      time.Now,
		envUsers:   names,
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

// IsActiveForToken reports whether the user an access token names is active.
// OIDC tokens name the user by email, so a provider "oidc" sub is looked up by
// email and cached under its own key, which InvalidateOIDC purges; an unknown
// email counts as inactive, as an unknown ID does. Every other sub goes
// through IsActive, which leaves env users always active.
func (c *UserActiveCache) IsActiveForToken(ctx context.Context, sub, provider string) (bool, error) {
	if provider != "oidc" {
		return c.IsActive(ctx, sub)
	}
	return c.lookup(ctx, oidcKey(sub), func(ctx context.Context) (*store.User, error) { return c.store.GetByEmail(ctx, sub) })
}

// oidcKey is the cache key of an OIDC access token's email sub. The prefix
// keeps it apart from the numeric-ID keys.
func oidcKey(email string) string { return "oidc:" + strings.ToLower(email) }

// envUserKey is the cache key recording that an env-user name has no users
// row.
func envUserKey(name string) string { return "env:" + name }

// APIKeyIdentity is the users row behind an API key.
//
// UserID is 0 when the key belongs to an env-config user (ADMIN_USER or
// VIEWER_USER), which has no users row at all. Callers that need to write a
// users(id) foreign key must treat 0 as "no such user" rather than as an ID.
type APIKeyIdentity struct {
	UserID   int64
	IsActive bool
}

// ResolveAPIKeyUsername resolves an api_keys.username value to the owning user,
// regardless of the value's historical shape:
//   - numeric string other than an env-user name (legacy DB-user ID stored at
//     creation time) → looks up by users.id.
//   - anything else (email address, the correct post-Phase-2 shape) → looks up
//     by email.
//   - a configured env-user name → looks up by email too, because a users row
//     with that email is authoritative. Only when there is none is it the env
//     user, {UserID: 0, IsActive: true}, which has no users row and is always
//     active; that answer is cached for the TTL.
//
// A key whose owner no longer exists — an OIDC email change leaves the old
// address behind — resolves to {IsActive: false}, like a deactivated owner,
// and is not an error. Only a failing store is an error, and callers must not
// let the key through on one.
//
// It exists because api_keys.user_id, though present in the schema since
// migration 0020, is never written by APIKeyStore.Create and so is NULL on
// every key. The owning user therefore has to be resolved from username, which
// is the same lookup the is_active recheck already performs — callers get the
// ID for free rather than paying a second query.
func (c *UserActiveCache) ResolveAPIKeyUsername(ctx context.Context, username string) (APIKeyIdentity, error) {
	envUser := c.envUsers[username]
	if id, err := strconv.ParseInt(username, 10, 64); err == nil && !envUser {
		active, err := c.isActiveByID(ctx, id, username)
		return APIKeyIdentity{UserID: id, IsActive: active}, err
	}
	if envUser {
		if _, ok := c.cached(envUserKey(username)); ok {
			return APIKeyIdentity{UserID: 0, IsActive: true}, nil
		}
	}

	u, err := c.store.GetByEmail(ctx, username)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			if envUser {
				c.storeEntry(envUserKey(username), true)
				return APIKeyIdentity{UserID: 0, IsActive: true}, nil
			}
			// Do not cache — no stable key.
			return APIKeyIdentity{}, nil
		}
		return APIKeyIdentity{}, fmt.Errorf("user_active_cache: get by email: %w", err)
	}
	c.storeEntry(strconv.FormatInt(u.ID, 10), u.IsActive)
	return APIKeyIdentity{UserID: u.ID, IsActive: u.IsActive}, nil
}

// IsActiveByAPIKeyUsername reports whether the user owning an API key is
// active. It is a thin wrapper over ResolveAPIKeyUsername for callers that only
// need the flag.
func (c *UserActiveCache) IsActiveByAPIKeyUsername(ctx context.Context, username string) (bool, error) {
	identity, err := c.ResolveAPIKeyUsername(ctx, username)
	return identity.IsActive, err
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

// InvalidateOIDC purges the entry IsActiveForToken caches for an OIDC access
// token whose sub is email. Safe to call for an email that is not cached.
func (c *UserActiveCache) InvalidateOIDC(email string) {
	c.Invalidate(oidcKey(email))
}

// isActiveByID is the core load path keyed by numeric ID.
func (c *UserActiveCache) isActiveByID(ctx context.Context, id int64, key string) (bool, error) {
	return c.lookup(ctx, key, func(ctx context.Context) (*store.User, error) { return c.store.GetByID(ctx, id) })
}

// lookup returns the flag cached under key, loading it through get on a miss.
func (c *UserActiveCache) lookup(ctx context.Context, key string, get func(context.Context) (*store.User, error)) (bool, error) {
	// Fast path: read-locked cache hit.
	if active, ok := c.cached(key); ok {
		return active, nil
	}

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

	// Perform the load exactly once, on behalf of every waiting caller: detach
	// it from this caller's cancellation and bound it instead.
	loadCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), userActiveLoadTimeout)
	defer cancel()
	call.val, call.err = c.load(loadCtx, key, get)
	call.wg.Done()

	c.sfMu.Lock()
	delete(c.flights, key)
	c.sfMu.Unlock()

	return call.val, call.err
}

// cached returns the flag stored under key, if it has not expired.
func (c *UserActiveCache) cached(key string) (isActive, ok bool) {
	now := c.nowFn()
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[key]
	if !ok || !now.Before(e.expiresAt) {
		return false, false
	}
	return e.isActive, true
}

// load fetches the user through get and populates the cache under key.
// Behaviour:
//   - ErrUserNotFound → cache (false, nil) so we don't spam the DB for
//     deleted/never-existed users.
//   - any other error → propagate, do NOT cache (the auth middleware
//     answers 503 on a transient DB error).
func (c *UserActiveCache) load(ctx context.Context, key string, get func(context.Context) (*store.User, error)) (bool, error) {
	u, err := get(ctx)
	if err != nil {
		if errors.Is(err, store.ErrUserNotFound) {
			c.storeEntry(key, false)
			return false, nil
		}
		return false, fmt.Errorf("user_active_cache: load %s: %w", key, err)
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
