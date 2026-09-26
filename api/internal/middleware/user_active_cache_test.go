package middleware

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// userLookups counts the store lookups a UserActiveCache makes.
type userLookups struct{ byID, byEmail atomic.Int32 }

// countingUsers answers every GetByID with that user and every GetByEmail
// with user 42, in the given active state, or fails both with err.
func countingUsers(active bool, err error, delay time.Duration) (*testutil.MockUserStore, *userLookups) {
	n := &userLookups{}
	return &testutil.MockUserStore{
		GetByIDFn: func(_ context.Context, id int64) (*store.User, error) {
			n.byID.Add(1)
			time.Sleep(delay)
			if err != nil {
				return nil, err
			}
			return &store.User{ID: id, IsActive: active}, nil
		},
		GetByEmailFn: func(_ context.Context, email string) (*store.User, error) {
			n.byEmail.Add(1)
			if err != nil {
				return nil, err
			}
			return &store.User{ID: 42, Email: email, IsActive: active}, nil
		},
	}, n
}

// TestUserActiveCache_Lookup calls one entry point twice per row and counts
// the store lookups. Env literals never reach the store; numeric values go by
// ID and are cached, including a not-found negative; store errors are not
// cached. By email, not-found is an error rather than "inactive" — the auth
// middleware fails open on it (fix 927ef9e).
func TestUserActiveCache_Lookup(t *testing.T) {
	t.Parallel()
	byID := (*UserActiveCache).IsActive
	byEmail := (*UserActiveCache).IsActiveByEmail
	byKey := (*UserActiveCache).IsActiveByAPIKeyUsername
	boom := errors.New("db boom")
	tests := []struct {
		name                  string
		lookup                func(*UserActiveCache, context.Context, string) (bool, error)
		arg                   string
		active                bool
		storeErr              error
		want                  bool
		wantErr               error
		wantByID, wantByEmail int32
	}{
		{"env sub", byID, "admin", false, nil, true, nil, 0, 0},
		{"deactivated user is cached", byID, "42", false, nil, false, nil, 1, 0},
		{"unknown ID caches a negative", byID, "999", true, store.ErrUserNotFound, false, nil, 1, 0},
		{"store errors propagate uncached", byID, "5", true, boom, false, boom, 2, 0},
		{"unknown email is an error", byEmail, "nope@nowhere", true, store.ErrUserNotFound, false, store.ErrUserNotFound, 0, 2},
		{"API key of env admin", byKey, "admin", false, nil, true, nil, 0, 0},
		{"API key of env editor", byKey, "editor", false, nil, true, nil, 0, 0},
		{"API key of env viewer", byKey, "viewer", false, nil, true, nil, 0, 0},
		{"API key of a numeric username goes by ID", byKey, "1", true, nil, true, nil, 1, 0},
		{"API key of an email username goes by email", byKey, "x@y.z", true, nil, true, nil, 0, 2},
		{"API key of an unknown email is an error", byKey, "nope@nowhere", true, store.ErrUserNotFound, false, store.ErrUserNotFound, 0, 2},
	}
	for _, tc := range tests {
		users, n := countingUsers(tc.active, tc.storeErr, 0)
		c := NewUserActiveCache(users, time.Minute, 10)
		for range 2 {
			got, err := tc.lookup(c, context.Background(), tc.arg)
			if got != tc.want || !errors.Is(err, tc.wantErr) {
				t.Errorf("%s: got (%v, %v), want (%v, %v)", tc.name, got, err, tc.want, tc.wantErr)
			}
		}
		if n.byID.Load() != tc.wantByID || n.byEmail.Load() != tc.wantByEmail {
			t.Errorf("%s: lookups by ID %d, by email %d; want %d, %d", tc.name, n.byID.Load(), n.byEmail.Load(), tc.wantByID, tc.wantByEmail)
		}
	}
}

// TestUserActiveCache_Reload: a cached flag is not re-read within the TTL, is
// re-read once it expires, and at once after Invalidate, so a deactivation
// can take effect immediately.
func TestUserActiveCache_Reload(t *testing.T) {
	t.Parallel()
	active, loads := true, 0
	users := &testutil.MockUserStore{GetByIDFn: func(_ context.Context, id int64) (*store.User, error) {
		loads++
		return &store.User{ID: id, IsActive: active}, nil
	}}
	c := NewUserActiveCache(users, time.Minute, 10)
	now := time.Unix(1_700_000_000, 0)
	c.SetNowFn(func() time.Time { return now })

	steps := []struct {
		name       string
		advance    time.Duration
		deactivate bool
		invalidate bool
		want       bool
		wantLoads  int
	}{
		{"miss loads", 0, false, false, true, 1},
		{"hit within the TTL", 30 * time.Second, false, false, true, 1},
		{"deactivation unseen within the TTL", 0, true, false, true, 1},
		{"invalidate reloads", 0, false, true, false, 2},
		{"expiry reloads", 2 * time.Minute, false, false, false, 3},
	}
	for _, s := range steps {
		now = now.Add(s.advance)
		if s.deactivate {
			active = false
		}
		if s.invalidate {
			c.Invalidate("11")
		}
		got, err := c.IsActive(context.Background(), "11")
		if err != nil || got != s.want || loads != s.wantLoads {
			t.Errorf("%s: got (%v, %v) after %d loads, want %v after %d", s.name, got, err, loads, s.want, s.wantLoads)
		}
	}
}

func TestUserActiveCache_ConcurrentMissDedupes(t *testing.T) {
	t.Parallel()
	// The delay widens the window so the concurrent misses really overlap.
	users, n := countingUsers(true, nil, 10*time.Millisecond)
	c := NewUserActiveCache(users, time.Second, 10)

	const N = 100
	var wg sync.WaitGroup
	for range N {
		wg.Go(func() {
			if ok, err := c.IsActive(context.Background(), "7"); err != nil || !ok {
				t.Errorf("concurrent call: ok=%v err=%v", ok, err)
			}
		})
	}
	wg.Wait()
	if got := n.byID.Load(); got != 1 {
		t.Fatalf("expected exactly 1 store call across %d concurrent callers, got %d", N, got)
	}
}

func TestUserActiveCache_SizeBoundEvicts(t *testing.T) {
	t.Parallel()
	users, _ := countingUsers(true, nil, 0)
	c := NewUserActiveCache(users, time.Hour, 2)
	// A controlled clock makes "oldest" unambiguous.
	cur := time.Unix(1_700_000_000, 0)
	c.SetNowFn(func() time.Time { return cur })
	for _, sub := range []string{"1", "2", "3"} {
		_, _ = c.IsActive(context.Background(), sub)
		cur = cur.Add(time.Second)
	}

	c.mu.RLock()
	defer c.mu.RUnlock()
	if _, ok := c.entries["1"]; len(c.entries) != 2 || ok {
		t.Fatalf("expected the size cap to evict the oldest entry (1), got %v", c.entries)
	}
}

// TestUserActiveCache_ByEmailMergesWithByID: an email lookup warms the entry
// keyed by user ID, so a following IsActive for that ID is a cache hit.
func TestUserActiveCache_ByEmailMergesWithByID(t *testing.T) {
	t.Parallel()
	users, n := countingUsers(true, nil, 0)
	c := NewUserActiveCache(users, time.Hour, 10)

	if ok, err := c.IsActiveByEmail(context.Background(), "u@x"); err != nil || !ok {
		t.Fatalf("by-email: ok=%v err=%v", ok, err)
	}
	if ok, err := c.IsActive(context.Background(), "42"); err != nil || !ok {
		t.Fatalf("by-id followup: ok=%v err=%v", ok, err)
	}
	if n.byID.Load() != 0 || n.byEmail.Load() != 1 {
		t.Fatalf("lookups by ID %d, by email %d; want 0, 1", n.byID.Load(), n.byEmail.Load())
	}
}
