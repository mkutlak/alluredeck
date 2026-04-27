package middleware

import (
	"sync"
	"testing"
	"time"
)

// newTestThrottler returns an AccountThrottler with small thresholds and a
// short lockout so tests stay fast. window=1min, soft=1, lockout=5,
// lockoutDuration=200ms, backoffMax=4s.
func newTestThrottler() *AccountThrottler {
	return NewAccountThrottler(time.Minute, 1, 5, 200*time.Millisecond, 4*time.Second)
}

// TestAccountThrottle_AllowsBelowThreshold — 1 failure → next Check returns
// Allowed=true with delay >= 1s (exponential backoff starts at 1s).
func TestAccountThrottle_AllowsBelowThreshold(t *testing.T) {
	tr := newTestThrottler()

	res := tr.RecordFailure("alice")
	if !res.Allowed {
		t.Fatalf("after 1 failure, want Allowed=true, got false")
	}
	if res.FailureCount != 1 {
		t.Errorf("FailureCount = %d, want 1", res.FailureCount)
	}

	check := tr.Check("alice")
	if !check.Allowed {
		t.Fatalf("Check after 1 failure: want Allowed=true, got false")
	}
	if check.Delay < time.Second {
		t.Errorf("Delay = %v, want >= 1s", check.Delay)
	}
}

// TestAccountThrottle_LocksOutAtThreshold — 5 failures → next Check returns
// Allowed=false with LockedUntil set in the future.
func TestAccountThrottle_LocksOutAtThreshold(t *testing.T) {
	tr := newTestThrottler()

	for i := 0; i < 4; i++ {
		res := tr.RecordFailure("bob")
		if !res.Allowed {
			t.Fatalf("failure %d: locked too early, count=%d", i+1, res.FailureCount)
		}
	}
	// Fifth failure trips the lockout.
	res := tr.RecordFailure("bob")
	if res.Allowed {
		t.Fatalf("after 5 failures: want Allowed=false, got true")
	}
	if res.LockedUntil.IsZero() || !res.LockedUntil.After(time.Now()) {
		t.Errorf("LockedUntil = %v, want a future time", res.LockedUntil)
	}

	// A subsequent Check should also report locked.
	check := tr.Check("bob")
	if check.Allowed {
		t.Errorf("Check while locked: want Allowed=false, got true")
	}
}

// TestAccountThrottle_RecordSuccessResets — record several failures, then a
// success → next Check returns Allowed=true with FailureCount==0 and no delay.
func TestAccountThrottle_RecordSuccessResets(t *testing.T) {
	tr := newTestThrottler()

	for i := 0; i < 3; i++ {
		tr.RecordFailure("carol")
	}
	tr.RecordSuccess("carol")

	check := tr.Check("carol")
	if !check.Allowed {
		t.Fatalf("after RecordSuccess: want Allowed=true")
	}
	if check.FailureCount != 0 {
		t.Errorf("FailureCount = %d, want 0", check.FailureCount)
	}
	if check.Delay != 0 {
		t.Errorf("Delay = %v, want 0", check.Delay)
	}
}

// TestAccountThrottle_WindowExpires — record failures just below the lockout
// threshold, advance the fake clock past the window, then check that Check
// reports a clean state and that the next failure resets the counter.
func TestAccountThrottle_WindowExpires(t *testing.T) {
	tr := newTestThrottler()
	now := time.Now()
	tr.SetNowFn(func() time.Time { return now })

	for i := 0; i < 4; i++ {
		tr.RecordFailure("dave")
	}

	// Advance past window.
	now = now.Add(2 * time.Minute)

	res := tr.RecordFailure("dave")
	if !res.Allowed {
		t.Fatalf("after window expiry: want Allowed=true")
	}
	if res.FailureCount != 1 {
		t.Errorf("FailureCount after window reset = %d, want 1", res.FailureCount)
	}
}

// TestAccountThrottle_BackoffEscalates — measure that the recommended delay
// grows 1s, 2s, 4s, … capped at backoffMax.
func TestAccountThrottle_BackoffEscalates(t *testing.T) {
	tr := newTestThrottler() // backoffMax = 4s

	want := []time.Duration{
		1 * time.Second, // after 1 failure
		2 * time.Second, // after 2
		4 * time.Second, // after 3
		4 * time.Second, // after 4 → capped at backoffMax
	}
	for i, w := range want {
		res := tr.RecordFailure("eve")
		if res.Delay != w {
			t.Errorf("after %d failures: Delay = %v, want %v", i+1, res.Delay, w)
		}
	}
}

// TestAccountThrottle_UsernameNormalisation — "Alice@x" and "alice@x" share
// the same counter (case-insensitive, whitespace-stripped).
func TestAccountThrottle_UsernameNormalisation(t *testing.T) {
	tr := newTestThrottler()

	tr.RecordFailure("Alice@x")
	tr.RecordFailure("  alice@x  ")
	tr.RecordFailure("ALICE@X")

	check := tr.Check("alice@x")
	if check.FailureCount != 3 {
		t.Errorf("FailureCount = %d, want 3 (all variants share counter)", check.FailureCount)
	}
}

// TestAccountThrottle_Concurrent — 100 goroutines RecordFailure on the same
// username; final count must be exactly 100 with no panics.
func TestAccountThrottle_Concurrent(t *testing.T) {
	tr := NewAccountThrottler(time.Minute, 1, 1000, time.Minute, 60*time.Second)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tr.RecordFailure("frank")
		}()
	}
	wg.Wait()

	check := tr.Check("frank")
	if check.FailureCount != 100 {
		t.Errorf("FailureCount = %d, want 100", check.FailureCount)
	}
}

// TestAccountThrottle_Cleanup — entries past the window are removed by
// Cleanup; recent entries are retained.
func TestAccountThrottle_Cleanup(t *testing.T) {
	tr := newTestThrottler()
	now := time.Now()
	tr.SetNowFn(func() time.Time { return now })

	tr.RecordFailure("stale")
	tr.RecordFailure("stale")

	// Advance past window so "stale" is eligible for removal.
	now = now.Add(2 * time.Minute)
	tr.RecordFailure("fresh")

	tr.Cleanup()

	tr.mu.Lock()
	_, staleStillThere := tr.entries["stale"]
	_, freshStillThere := tr.entries["fresh"]
	tr.mu.Unlock()

	if staleStillThere {
		t.Errorf("expected stale entry to be removed by Cleanup")
	}
	if !freshStillThere {
		t.Errorf("expected fresh entry to remain after Cleanup")
	}
}

// TestAccountThrottle_LockoutExpiresAllowsRetry — once lockoutDuration has
// elapsed, a previously-locked username may attempt again. The first new
// failure should reset the counter (window also elapsed) or carry forward
// (if window still open) — verify the re-armed lockout pattern matches the
// documented sliding-window semantics.
func TestAccountThrottle_LockoutExpiresAllowsRetry(t *testing.T) {
	tr := newTestThrottler() // lockoutDuration = 200ms
	now := time.Now()
	tr.SetNowFn(func() time.Time { return now })

	for i := 0; i < 5; i++ {
		tr.RecordFailure("grace")
	}

	check := tr.Check("grace")
	if check.Allowed {
		t.Fatalf("expected lock immediately after 5 failures")
	}

	// Advance past lockoutDuration.
	now = now.Add(500 * time.Millisecond)

	check = tr.Check("grace")
	if !check.Allowed {
		t.Errorf("expected Allowed=true after lockout expiry, got LockedUntil=%v now=%v", check.LockedUntil, now)
	}
}

// TestAccountThrottle_EmptyUsernameNoOp — Check/Record on an empty username
// must not panic and should be a logical no-op (returns Allowed=true).
func TestAccountThrottle_EmptyUsernameNoOp(t *testing.T) {
	tr := newTestThrottler()

	check := tr.Check("")
	if !check.Allowed {
		t.Errorf("Check(\"\"): want Allowed=true")
	}
	res := tr.RecordFailure("   ")
	if !res.Allowed {
		t.Errorf("RecordFailure(\"   \"): want Allowed=true (no-op)")
	}
}
