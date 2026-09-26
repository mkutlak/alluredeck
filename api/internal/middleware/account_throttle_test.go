package middleware

import (
	"sync"
	"testing"
	"time"
)

// TestAccountThrottle walks usernames through the lockout policy on a fake
// clock: window 1m, backoff doubling from 1s capped at 4s, lockout at the 5th
// failure for 200ms. Every step compares the whole result.
func TestAccountThrottle(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	allowed := func(delay time.Duration, n int) AccountThrottleResult {
		return AccountThrottleResult{Allowed: true, Delay: delay, FailureCount: n}
	}
	locked := AccountThrottleResult{LockedUntil: t0.Add(200 * time.Millisecond), FailureCount: 5}

	type step struct {
		op   string // "fail", "ok", "check", or "wait" (arg is a duration)
		arg  string
		want AccountThrottleResult
	}
	tests := []struct {
		name  string
		steps []step
	}{
		{"backoff escalates to a lockout that expires", []step{
			{"fail", "bob", allowed(time.Second, 1)},
			{"check", "bob", allowed(time.Second, 1)},
			{"fail", "bob", allowed(2*time.Second, 2)},
			{"fail", "bob", allowed(4*time.Second, 3)},
			{"fail", "bob", allowed(4*time.Second, 4)}, // capped at backoffMax
			{"fail", "bob", locked},
			{"check", "bob", locked},
			{"wait", "500ms", AccountThrottleResult{}},
			{"check", "bob", allowed(4*time.Second, 5)},
		}},
		{"success resets the counter", []step{
			{"fail", "carol", allowed(time.Second, 1)},
			{"fail", "carol", allowed(2*time.Second, 2)},
			{"ok", "carol", AccountThrottleResult{}},
			{"check", "carol", allowed(0, 0)},
		}},
		{"an elapsed window restarts the count", []step{
			{"fail", "dave", allowed(time.Second, 1)},
			{"fail", "dave", allowed(2*time.Second, 2)},
			{"wait", "2m", AccountThrottleResult{}},
			{"fail", "dave", allowed(time.Second, 1)},
		}},
		{"usernames are case- and space-insensitive", []step{
			{"fail", "Alice@x", allowed(time.Second, 1)},
			{"fail", "  alice@x  ", allowed(2*time.Second, 2)},
			{"fail", "ALICE@X", allowed(4*time.Second, 3)},
			{"check", "alice@x", allowed(4*time.Second, 3)},
		}},
		{"empty usernames are a no-op", []step{
			{"check", "", allowed(0, 0)},
			{"fail", "   ", allowed(0, 0)},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			now := t0
			tr := NewAccountThrottler(time.Minute, 1, 5, 200*time.Millisecond, 4*time.Second)
			tr.SetNowFn(func() time.Time { return now })
			for i, s := range tc.steps {
				var got AccountThrottleResult
				switch s.op {
				case "fail":
					got = tr.RecordFailure(s.arg)
				case "check":
					got = tr.Check(s.arg)
				case "ok":
					tr.RecordSuccess(s.arg)
				case "wait":
					d, err := time.ParseDuration(s.arg)
					if err != nil {
						t.Fatal(err)
					}
					now = now.Add(d)
				}
				if got != s.want {
					t.Errorf("step %d %s(%q) = %+v, want %+v", i, s.op, s.arg, got, s.want)
				}
			}
		})
	}
}

// TestAccountThrottle_Concurrent — 100 goroutines RecordFailure on the same
// username; final count must be exactly 100 with no panics.
func TestAccountThrottle_Concurrent(t *testing.T) {
	tr := NewAccountThrottler(time.Minute, 1, 1000, time.Minute, 60*time.Second)

	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			tr.RecordFailure("frank")
		})
	}
	wg.Wait()

	if check := tr.Check("frank"); check.FailureCount != 100 {
		t.Errorf("FailureCount = %d, want 100", check.FailureCount)
	}
}

// TestAccountThrottle_Cleanup — entries past the window are removed by
// Cleanup; recent entries are retained.
func TestAccountThrottle_Cleanup(t *testing.T) {
	tr := NewAccountThrottler(time.Minute, 1, 5, 200*time.Millisecond, 4*time.Second)
	now := time.Now()
	tr.SetNowFn(func() time.Time { return now })

	tr.RecordFailure("stale")
	now = now.Add(2 * time.Minute)
	tr.RecordFailure("fresh")
	tr.Cleanup()

	tr.mu.Lock()
	_, stale := tr.entries["stale"]
	_, fresh := tr.entries["fresh"]
	tr.mu.Unlock()
	if stale || !fresh {
		t.Errorf("after Cleanup: stale present=%v, fresh present=%v; want false, true", stale, fresh)
	}
}
