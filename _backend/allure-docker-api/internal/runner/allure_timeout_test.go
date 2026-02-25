package runner

import (
	"context"
	"testing"
	"time"

	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/config"
	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/storage"
	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/store"
)

func newTestAllure(t *testing.T) *Allure {
	t.Helper()
	cfg := &config.Config{}
	st := storage.NewLocalStore(cfg)
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatalf("store.Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	bs := store.NewBuildStore(s)
	lm := store.NewLockManager()
	return NewAllure(cfg, st, bs, lm)
}

// TestRunAllureCmdHonoursCancelledContext verifies that runAllureCmd returns a
// non-nil error immediately when the provided context is already cancelled,
// regardless of whether the allure binary is present on the system.
func TestRunAllureCmdHonoursCancelledContext(t *testing.T) {
	a := newTestAllure(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before calling

	start := time.Now()
	err := a.runAllureCmd(ctx, "version")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected non-nil error from cancelled context, got nil")
	}
	if elapsed > 2*time.Second {
		t.Fatalf("runAllureCmd did not return quickly with cancelled context: took %v", elapsed)
	}
}

// TestRunAllureCmdHonoursDeadline verifies that runAllureCmd respects a
// context deadline and returns an error within a reasonable time even if the
// allure binary would otherwise run for longer.
func TestRunAllureCmdHonoursDeadline(t *testing.T) {
	a := newTestAllure(t)

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Millisecond)
	defer cancel()

	// Sleep briefly to let the deadline expire before cmd.Run() is reached.
	time.Sleep(5 * time.Millisecond)

	start := time.Now()
	err := a.runAllureCmd(ctx, "version")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected non-nil error from expired deadline, got nil")
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("runAllureCmd did not return quickly with expired deadline: took %v", elapsed)
	}
}
