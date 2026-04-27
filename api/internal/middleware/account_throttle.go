package middleware

import (
	"strings"
	"sync"
	"time"
)

// AccountThrottler tracks failed login attempts per lowercased username and
// applies escalating delays plus a hard lockout threshold to defend against
// distributed credential stuffing. It is decoupled from IPRateLimiter — that
// keys on client IP; this is the second tier when an attacker rotates IPs.
//
// Policy (production defaults; configurable via constructor):
//   - window           = 15 min sliding-counter
//   - softThreshold    = 1   (any failure starts the timer)
//   - lockoutThreshold = 20  (failures within window → return Locked=true)
//   - lockoutDuration  = 15 min from the threshold-tripping failure
//   - backoffMax       = 60s, doubling 1s, 2s, 4s, 8s, 16s, 32s, capped
//
// On RecordSuccess(username) the counter resets so a legitimate user is not
// punished by their own typos. Stale entries are pruned by Cleanup.
//
// Concurrency: a single sync.Mutex guards the entries map and counters.
// Operations are short and lock contention is low — per-username locks are
// not worth the bookkeeping.
type AccountThrottler struct {
	window           time.Duration
	softThreshold    int
	lockoutThreshold int
	lockoutDuration  time.Duration
	backoffMax       time.Duration

	mu      sync.Mutex
	entries map[string]*accountEntry
	nowFn   func() time.Time // injectable for tests
}

// accountEntry holds the per-username state. firstFailure anchors the sliding
// window; lockedUntil is non-zero only while the account is hard-locked.
type accountEntry struct {
	failureCount int
	firstFailure time.Time
	lastFailure  time.Time
	lockedUntil  time.Time
}

// AccountThrottleResult is the decision returned by Check / RecordFailure.
type AccountThrottleResult struct {
	// Allowed is false when the account is locked (caller must reject 429).
	Allowed bool
	// LockedUntil is the time at which the account becomes available again.
	// Zero value means the account is not locked.
	LockedUntil time.Time
	// Delay is the recommended artificial delay before responding (caller may
	// honour or ignore). Capped at backoffMax. Zero when no failures recorded.
	Delay time.Duration
	// FailureCount is the current failure count for the username (post-op for
	// RecordFailure, current for Check). Useful for audit metadata.
	FailureCount int
}

// NewAccountThrottler constructs a throttler with the given policy. Pass
// production defaults from cmd/api/main.go; tests use small thresholds.
func NewAccountThrottler(window time.Duration, softThreshold, lockoutThreshold int, lockoutDuration, backoffMax time.Duration) *AccountThrottler {
	return &AccountThrottler{
		window:           window,
		softThreshold:    softThreshold,
		lockoutThreshold: lockoutThreshold,
		lockoutDuration:  lockoutDuration,
		backoffMax:       backoffMax,
		entries:          make(map[string]*accountEntry),
		nowFn:            time.Now,
	}
}

// SetNowFn overrides the clock for deterministic tests. Production callers
// must not invoke this.
func (t *AccountThrottler) SetNowFn(fn func() time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.nowFn = fn
}

// normaliseUsername lowercases and trims the supplied username so that
// "Alice@x" and "alice@x" share a counter. Empty inputs yield empty output;
// callers treat that as a no-op.
func normaliseUsername(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// Check is the pre-login probe. It does NOT increment any counter — the
// caller must record the outcome via RecordFailure or RecordSuccess after the
// credential check resolves. Returns Allowed=true when the username is empty
// (defensive; treat empties as no-op rather than blocking the request).
func (t *AccountThrottler) Check(username string) AccountThrottleResult {
	key := normaliseUsername(username)
	if key == "" {
		return AccountThrottleResult{Allowed: true}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.nowFn()
	entry := t.entries[key]
	if entry == nil {
		return AccountThrottleResult{Allowed: true}
	}

	// Lockout takes precedence: while lockedUntil is in the future, the
	// account is hard-locked regardless of count.
	if !entry.lockedUntil.IsZero() && entry.lockedUntil.After(now) {
		return AccountThrottleResult{
			Allowed:      false,
			LockedUntil:  entry.lockedUntil,
			FailureCount: entry.failureCount,
		}
	}

	// If the sliding window has elapsed since the first failure, treat the
	// counter as stale: report a clean state.
	if now.Sub(entry.firstFailure) > t.window {
		return AccountThrottleResult{Allowed: true}
	}

	return AccountThrottleResult{
		Allowed:      true,
		Delay:        backoffFor(entry.failureCount, t.backoffMax),
		FailureCount: entry.failureCount,
	}
}

// RecordFailure increments the failure counter for username and returns the
// post-increment Result. Crossing the lockout threshold sets LockedUntil on
// the returned Result and on the entry. An empty/whitespace username is a
// no-op (returns Allowed=true with no state change).
func (t *AccountThrottler) RecordFailure(username string) AccountThrottleResult {
	key := normaliseUsername(username)
	if key == "" {
		return AccountThrottleResult{Allowed: true}
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.nowFn()
	entry := t.entries[key]
	if entry == nil || now.Sub(entry.firstFailure) > t.window {
		// Either no prior failures or the window has elapsed — fresh start.
		entry = &accountEntry{
			failureCount: 1,
			firstFailure: now,
			lastFailure:  now,
		}
		t.entries[key] = entry
	} else {
		entry.failureCount++
		entry.lastFailure = now
	}

	// Cross the lockout threshold? Set lockedUntil and return Allowed=false.
	if entry.failureCount >= t.lockoutThreshold {
		entry.lockedUntil = now.Add(t.lockoutDuration)
		return AccountThrottleResult{
			Allowed:      false,
			LockedUntil:  entry.lockedUntil,
			FailureCount: entry.failureCount,
		}
	}

	return AccountThrottleResult{
		Allowed:      true,
		Delay:        backoffFor(entry.failureCount, t.backoffMax),
		FailureCount: entry.failureCount,
	}
}

// RecordSuccess clears any failure state for username so a legitimate user
// who fat-fingered their password is not punished. No-op for empty inputs.
func (t *AccountThrottler) RecordSuccess(username string) {
	key := normaliseUsername(username)
	if key == "" {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.entries, key)
}

// Cleanup removes entries whose window has fully elapsed and whose lockout
// has expired. Called periodically by StartCleanup.
func (t *AccountThrottler) Cleanup() {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.nowFn()
	for key, entry := range t.entries {
		// Keep the entry while the lockout is still active.
		if !entry.lockedUntil.IsZero() && entry.lockedUntil.After(now) {
			continue
		}
		// Drop entries whose last activity is older than the window.
		if now.Sub(entry.lastFailure) > t.window {
			delete(t.entries, key)
		}
	}
}

// StartCleanup runs Cleanup on a periodic interval until done is closed.
// Mirrors IPRateLimiter.StartCleanup.
func (t *AccountThrottler) StartCleanup(interval time.Duration, done <-chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				t.Cleanup()
			}
		}
	}()
}

// backoffFor returns the exponential backoff duration for the given failure
// count, capped at backoffMax. count=0 → 0s, count=1 → 1s, count=2 → 2s,
// count=3 → 4s, … doubling.
func backoffFor(count int, backoffMax time.Duration) time.Duration {
	if count <= 0 {
		return 0
	}
	// Compute 1s * 2^(count-1) without unbounded shifting (cap exponent
	// at 30 — beyond that we are well past backoffMax anyway).
	exp := min(count-1, 30)
	d := time.Second * time.Duration(int64(1)<<exp)
	if d > backoffMax || d < 0 { // d<0 guards against overflow
		return backoffMax
	}
	return d
}
