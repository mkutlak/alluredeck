package middleware

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// stubUserStore counts store calls and lets tests inject responses. It only
// implements the methods the cache uses; other UserStorer methods panic.
type stubUserStore struct {
	getByIDFn  func(ctx context.Context, id int64) (*store.User, error)
	getByEmail func(ctx context.Context, email string) (*store.User, error)
	getIDCalls int32
	getEmCalls atomic.Int32
	getIDDelay time.Duration // optional delay to widen the singleflight race window
}

func (s *stubUserStore) GetByID(ctx context.Context, id int64) (*store.User, error) {
	atomic.AddInt32(&s.getIDCalls, 1)
	if s.getIDDelay > 0 {
		time.Sleep(s.getIDDelay)
	}
	if s.getByIDFn == nil {
		return nil, store.ErrUserNotFound
	}
	return s.getByIDFn(ctx, id)
}

func (s *stubUserStore) GetByEmail(ctx context.Context, email string) (*store.User, error) {
	s.getEmCalls.Add(1)
	if s.getByEmail == nil {
		return nil, store.ErrUserNotFound
	}
	return s.getByEmail(ctx, email)
}

// Unused UserStorer methods — present to satisfy the interface.
func (s *stubUserStore) UpsertByOIDC(_ context.Context, _, _, _, _, _ string) (*store.User, error) {
	panic("unused")
}
func (s *stubUserStore) CreateLocal(_ context.Context, _, _, _, _ string) (*store.User, error) {
	panic("unused")
}
func (s *stubUserStore) List(_ context.Context) ([]store.User, error) { panic("unused") }
func (s *stubUserStore) ListPaginated(_ context.Context, _ store.ListUsersParams) ([]store.User, int, error) {
	panic("unused")
}
func (s *stubUserStore) UpdateRole(_ context.Context, _ int64, _ string) error    { panic("unused") }
func (s *stubUserStore) UpdateActive(_ context.Context, _ int64, _ bool) error    { panic("unused") }
func (s *stubUserStore) UpdateProfile(_ context.Context, _ int64, _ string) error { panic("unused") }
func (s *stubUserStore) UpdatePasswordHash(_ context.Context, _ int64, _ string) error {
	panic("unused")
}
func (s *stubUserStore) UpdateLastLogin(_ context.Context, _ int64) error { panic("unused") }
func (s *stubUserStore) ClearLastLogin(_ context.Context, _ int64) error  { panic("unused") }
func (s *stubUserStore) Deactivate(_ context.Context, _ int64) error      { panic("unused") }
func (s *stubUserStore) RelinkOIDC(_ context.Context, _ int64, _, _ string) error {
	panic("unused")
}

var _ store.UserStorer = (*stubUserStore)(nil)

func activeUser(id int64, email string, active bool) *store.User {
	return &store.User{ID: id, Email: email, IsActive: active}
}

func TestUserActiveCache_HitMissExpiry(t *testing.T) {
	t.Parallel()
	stub := &stubUserStore{
		getByIDFn: func(_ context.Context, id int64) (*store.User, error) {
			return activeUser(id, "u@x", true), nil
		},
	}
	c := NewUserActiveCache(stub, 25*time.Millisecond, 100)

	// Miss → load.
	if ok, err := c.IsActive(context.Background(), "1"); err != nil || !ok {
		t.Fatalf("first call: ok=%v err=%v", ok, err)
	}
	if got := atomic.LoadInt32(&stub.getIDCalls); got != 1 {
		t.Fatalf("expected 1 store call after miss, got %d", got)
	}

	// Hit (within TTL) → no extra call.
	if ok, _ := c.IsActive(context.Background(), "1"); !ok {
		t.Fatal("expected hit to return true")
	}
	if got := atomic.LoadInt32(&stub.getIDCalls); got != 1 {
		t.Fatalf("expected store still called once, got %d", got)
	}

	// Wait past TTL → reload.
	time.Sleep(50 * time.Millisecond)
	if ok, _ := c.IsActive(context.Background(), "1"); !ok {
		t.Fatal("expected post-expiry hit to return true")
	}
	if got := atomic.LoadInt32(&stub.getIDCalls); got != 2 {
		t.Fatalf("expected store called twice after expiry, got %d", got)
	}
}

func TestUserActiveCache_EnvUserBypass(t *testing.T) {
	t.Parallel()
	stub := &stubUserStore{
		getByIDFn: func(_ context.Context, _ int64) (*store.User, error) {
			t.Fatal("store must not be consulted for env users")
			return nil, nil
		},
	}
	c := NewUserActiveCache(stub, time.Second, 10)
	ok, err := c.IsActive(context.Background(), "admin")
	if err != nil || !ok {
		t.Fatalf("env user: ok=%v err=%v", ok, err)
	}
	if atomic.LoadInt32(&stub.getIDCalls) != 0 {
		t.Fatalf("expected zero store calls for env user, got %d", stub.getIDCalls)
	}
}

func TestUserActiveCache_DeactivatedUser(t *testing.T) {
	t.Parallel()
	stub := &stubUserStore{
		getByIDFn: func(_ context.Context, id int64) (*store.User, error) {
			return activeUser(id, "u@x", false), nil
		},
	}
	c := NewUserActiveCache(stub, time.Second, 10)
	ok, err := c.IsActive(context.Background(), "42")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if ok {
		t.Fatal("expected inactive user to return false")
	}
	// Second call should hit the cache.
	_, _ = c.IsActive(context.Background(), "42")
	if got := atomic.LoadInt32(&stub.getIDCalls); got != 1 {
		t.Fatalf("expected exactly 1 store call (second is cached), got %d", got)
	}
}

func TestUserActiveCache_ConcurrentMissDedupes(t *testing.T) {
	t.Parallel()
	stub := &stubUserStore{
		getIDDelay: 10 * time.Millisecond, // widen the window so the race is real
		getByIDFn: func(_ context.Context, id int64) (*store.User, error) {
			return activeUser(id, "u@x", true), nil
		},
	}
	c := NewUserActiveCache(stub, time.Second, 10)

	const N = 100
	var wg sync.WaitGroup
	wg.Add(N)
	for range N {
		go func() {
			defer wg.Done()
			if ok, err := c.IsActive(context.Background(), "7"); err != nil || !ok {
				t.Errorf("concurrent call: ok=%v err=%v", ok, err)
			}
		}()
	}
	wg.Wait()
	if got := atomic.LoadInt32(&stub.getIDCalls); got != 1 {
		t.Fatalf("expected exactly 1 store call across %d concurrent callers, got %d", N, got)
	}
}

func TestUserActiveCache_NotFoundCachesNegative(t *testing.T) {
	t.Parallel()
	stub := &stubUserStore{
		getByIDFn: func(_ context.Context, _ int64) (*store.User, error) {
			return nil, store.ErrUserNotFound
		},
	}
	c := NewUserActiveCache(stub, time.Second, 10)

	ok, err := c.IsActive(context.Background(), "999")
	if err != nil {
		t.Fatalf("not-found should not propagate: %v", err)
	}
	if ok {
		t.Fatal("not-found should resolve to false")
	}
	_, _ = c.IsActive(context.Background(), "999")
	if got := atomic.LoadInt32(&stub.getIDCalls); got != 1 {
		t.Fatalf("expected not-found to cache (1 call), got %d", got)
	}
}

func TestUserActiveCache_StoreErrorDoesNotCache(t *testing.T) {
	t.Parallel()
	boom := errors.New("db boom")
	stub := &stubUserStore{
		getByIDFn: func(_ context.Context, _ int64) (*store.User, error) {
			return nil, boom
		},
	}
	c := NewUserActiveCache(stub, time.Second, 10)

	if _, err := c.IsActive(context.Background(), "5"); err == nil {
		t.Fatal("expected error to propagate")
	}
	if _, err := c.IsActive(context.Background(), "5"); err == nil {
		t.Fatal("expected error to propagate on second call too")
	}
	if got := atomic.LoadInt32(&stub.getIDCalls); got != 2 {
		t.Fatalf("expected store called twice (errors not cached), got %d", got)
	}
}

func TestUserActiveCache_SizeBoundEvicts(t *testing.T) {
	t.Parallel()
	stub := &stubUserStore{
		getByIDFn: func(_ context.Context, id int64) (*store.User, error) {
			return activeUser(id, "u@x", true), nil
		},
	}
	c := NewUserActiveCache(stub, time.Hour, 2)
	// Use a controlled clock so eviction picks the truly oldest entry.
	t0 := time.Unix(1_700_000_000, 0)
	cur := t0
	c.SetNowFn(func() time.Time { return cur })

	_, _ = c.IsActive(context.Background(), "1")
	cur = cur.Add(time.Second)
	_, _ = c.IsActive(context.Background(), "2")
	cur = cur.Add(time.Second)
	_, _ = c.IsActive(context.Background(), "3")

	c.mu.RLock()
	defer c.mu.RUnlock()
	if len(c.entries) != 2 {
		t.Fatalf("expected size cap to keep 2 entries, got %d (%v)", len(c.entries), c.entries)
	}
	if _, ok := c.entries["1"]; ok {
		t.Fatal("expected oldest entry (1) to be evicted")
	}
}

func TestUserActiveCache_Invalidate(t *testing.T) {
	t.Parallel()
	stub := &stubUserStore{
		getByIDFn: func(_ context.Context, id int64) (*store.User, error) {
			return activeUser(id, "u@x", true), nil
		},
	}
	c := NewUserActiveCache(stub, time.Hour, 10)
	_, _ = c.IsActive(context.Background(), "11")
	c.Invalidate("11")
	_, _ = c.IsActive(context.Background(), "11")
	if got := atomic.LoadInt32(&stub.getIDCalls); got != 2 {
		t.Fatalf("expected post-invalidate reload, got %d store calls", got)
	}
}

func TestUserActiveCache_ByEmailMergesWithByID(t *testing.T) {
	t.Parallel()
	stub := &stubUserStore{
		getByIDFn: func(_ context.Context, id int64) (*store.User, error) {
			return activeUser(id, "u@x", true), nil
		},
		getByEmail: func(_ context.Context, email string) (*store.User, error) {
			return activeUser(33, email, true), nil
		},
	}
	c := NewUserActiveCache(stub, time.Hour, 10)

	if ok, err := c.IsActiveByEmail(context.Background(), "u@x"); err != nil || !ok {
		t.Fatalf("by-email: ok=%v err=%v", ok, err)
	}
	// Subsequent IsActive(sub="33") must hit the same cache entry.
	if ok, err := c.IsActive(context.Background(), "33"); err != nil || !ok {
		t.Fatalf("by-id followup: ok=%v err=%v", ok, err)
	}
	if got := atomic.LoadInt32(&stub.getIDCalls); got != 0 {
		t.Fatalf("expected zero GetByID calls (cache hit via email warmup), got %d", got)
	}
	if got := stub.getEmCalls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 GetByEmail call, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// IsActiveByEmail — ErrUserNotFound propagation (Phase 1 fix)
// ---------------------------------------------------------------------------

func TestUserActiveCache_IsActiveByEmail_NotFoundPropagatesError(t *testing.T) {
	t.Parallel()
	stub := &stubUserStore{
		// getByEmail returns ErrUserNotFound — must surface as error, not (false, nil).
		getByEmail: nil, // nil fn → ErrUserNotFound (see stubUserStore.GetByEmail)
	}
	c := NewUserActiveCache(stub, time.Second, 10)

	ok, err := c.IsActiveByEmail(context.Background(), "nope@nowhere")
	if err == nil {
		t.Fatal("expected ErrUserNotFound to propagate as an error, got nil")
	}
	if !errors.Is(err, store.ErrUserNotFound) {
		t.Fatalf("expected errors.Is(err, store.ErrUserNotFound), got: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when user not found")
	}
}

// ---------------------------------------------------------------------------
// IsActiveByAPIKeyUsername — shape dispatch (Phase 1 new method)
// ---------------------------------------------------------------------------

func TestUserActiveCache_IsActiveByAPIKeyUsername_EnvUserAdmin(t *testing.T) {
	t.Parallel()
	stub := &stubUserStore{
		getByIDFn: func(_ context.Context, _ int64) (*store.User, error) {
			t.Fatal("store must not be consulted for env user 'admin'")
			return nil, nil
		},
		getByEmail: func(_ context.Context, _ string) (*store.User, error) {
			t.Fatal("store must not be consulted for env user 'admin'")
			return nil, nil
		},
	}
	c := NewUserActiveCache(stub, time.Second, 10)
	ok, err := c.IsActiveByAPIKeyUsername(context.Background(), "admin")
	if err != nil || !ok {
		t.Fatalf("env user 'admin': want (true, nil), got (%v, %v)", ok, err)
	}
	if atomic.LoadInt32(&stub.getIDCalls) != 0 || stub.getEmCalls.Load() != 0 {
		t.Fatal("expected zero store calls for env user 'admin'")
	}
}

func TestUserActiveCache_IsActiveByAPIKeyUsername_EnvUserViewer(t *testing.T) {
	t.Parallel()
	stub := &stubUserStore{}
	c := NewUserActiveCache(stub, time.Second, 10)
	ok, err := c.IsActiveByAPIKeyUsername(context.Background(), "viewer")
	if err != nil || !ok {
		t.Fatalf("env user 'viewer': want (true, nil), got (%v, %v)", ok, err)
	}
}

func TestUserActiveCache_IsActiveByAPIKeyUsername_EnvUserEditor(t *testing.T) {
	t.Parallel()
	stub := &stubUserStore{}
	c := NewUserActiveCache(stub, time.Second, 10)
	ok, err := c.IsActiveByAPIKeyUsername(context.Background(), "editor")
	if err != nil || !ok {
		t.Fatalf("env user 'editor': want (true, nil), got (%v, %v)", ok, err)
	}
}

func TestUserActiveCache_IsActiveByAPIKeyUsername_NumericUsesIsActive(t *testing.T) {
	t.Parallel()
	stub := &stubUserStore{
		getByIDFn: func(_ context.Context, id int64) (*store.User, error) {
			return activeUser(id, "u@x.test", true), nil
		},
	}
	c := NewUserActiveCache(stub, time.Second, 10)
	ok, err := c.IsActiveByAPIKeyUsername(context.Background(), "1")
	if err != nil || !ok {
		t.Fatalf("numeric username '1': want (true, nil), got (%v, %v)", ok, err)
	}
	if got := atomic.LoadInt32(&stub.getIDCalls); got != 1 {
		t.Fatalf("expected 1 GetByID call for numeric username, got %d", got)
	}
	if stub.getEmCalls.Load() != 0 {
		t.Fatalf("expected 0 GetByEmail calls for numeric username, got %d", stub.getEmCalls.Load())
	}
}

func TestUserActiveCache_IsActiveByAPIKeyUsername_EmailUsesIsActiveByEmail(t *testing.T) {
	t.Parallel()
	stub := &stubUserStore{
		getByEmail: func(_ context.Context, email string) (*store.User, error) {
			return activeUser(42, email, true), nil
		},
	}
	c := NewUserActiveCache(stub, time.Second, 10)
	ok, err := c.IsActiveByAPIKeyUsername(context.Background(), "x@y.z")
	if err != nil || !ok {
		t.Fatalf("email username: want (true, nil), got (%v, %v)", ok, err)
	}
	if stub.getEmCalls.Load() != 1 {
		t.Fatalf("expected 1 GetByEmail call for email username, got %d", stub.getEmCalls.Load())
	}
	if atomic.LoadInt32(&stub.getIDCalls) != 0 {
		t.Fatalf("expected 0 GetByID calls for email username, got %d", stub.getIDCalls)
	}
}

func TestUserActiveCache_IsActiveByAPIKeyUsername_MissingEmailPropagatesError(t *testing.T) {
	t.Parallel()
	stub := &stubUserStore{
		// getByEmail nil → ErrUserNotFound propagated from IsActiveByEmail
	}
	c := NewUserActiveCache(stub, time.Second, 10)
	ok, err := c.IsActiveByAPIKeyUsername(context.Background(), "nope@nowhere")
	if err == nil {
		t.Fatal("expected error for unknown email, got nil")
	}
	if !errors.Is(err, store.ErrUserNotFound) {
		t.Fatalf("expected errors.Is(err, store.ErrUserNotFound), got: %v", err)
	}
	if ok {
		t.Fatal("expected ok=false when user not found by email")
	}
}

// Smoke test: the cache works against the canonical MemUserStore too.
func TestUserActiveCache_WithMemUserStore(t *testing.T) {
	t.Parallel()
	mem := testutil.NewMemUserStore()
	u, err := mem.UpsertByOIDC(context.Background(), "oidc", "sub-1", "u@x", "u", "viewer")
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	c := NewUserActiveCache(mem, time.Second, 10)

	sub := strconv.FormatInt(u.ID, 10)
	if ok, err := c.IsActive(context.Background(), sub); err != nil || !ok {
		t.Fatalf("active user: ok=%v err=%v", ok, err)
	}

	if err := mem.UpdateActive(context.Background(), u.ID, false); err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	c.Invalidate(sub)
	ok, err := c.IsActive(context.Background(), sub)
	if err != nil {
		t.Fatalf("post-deactivate: %v", err)
	}
	if ok {
		t.Fatal("expected post-deactivate IsActive to return false")
	}
}
