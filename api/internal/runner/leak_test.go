//go:build leakprofile

package runner

import (
	"context"
	"runtime"
	"runtime/pprof"
	"strings"
	"testing"
	"time"
)

// TestNoGoroutineLeaks_MemJobManager exercises MemJobManager through
// submit -> work -> shutdown and asserts the goroutine leak profile is empty.
// Requires GOEXPERIMENT=goroutineleakprofile at build time.
func TestNoGoroutineLeaks_MemJobManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mgr := NewMemJobManager(&fakeReportGenerator{}, 2, nil)
	mgr.Start(ctx)

	// Submit a no-op job and wait for it to reach a terminal state.
	job := mgr.Submit(ctx, 1, "test-slug", JobParams{
		StorageKey: "test/key",
	})

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		j := mgr.Get(ctx, job.ID)
		if j != nil && (j.Status == JobStatusCompleted || j.Status == JobStatusFailed) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Signal workers to stop and wait for them to drain.
	cancel()
	mgr.Shutdown()

	// Force GC so any goroutines blocked on unreachable primitives are visible.
	runtime.GC()
	runtime.GC()
	time.Sleep(100 * time.Millisecond)

	p := pprof.Lookup("goroutineleak")
	if p == nil {
		t.Skip("goroutineleak profile unavailable (build without GOEXPERIMENT=goroutineleakprofile?)")
	}
	if got := p.Count(); got != 0 {
		var buf strings.Builder
		_ = p.WriteTo(&buf, 1)
		t.Fatalf("goroutine leak detected: count=%d\n%s", got, buf.String())
	}
}
