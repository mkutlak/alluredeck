package mcp_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	internalmcp "github.com/mkutlak/alluredeck/api/internal/mcp"
)

// callsUntilLimited drives the rate-limit middleware with the given Mcp-Name
// until it returns 429, and reports how many calls got through.
//
// No TokenInfo is injected, so every request resolves to the same "unknown"
// identity and therefore the same token bucket — which is what these tests
// need. Identity keying is not under test here; cost accounting is.
func callsUntilLimited(t *testing.T, rl *internalmcp.RateLimiter, toolName string, cap int) int {
	t.Helper()
	handler := rl.Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := range cap {
		req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
		if toolName != "" {
			req.Header.Set("Mcp-Name", toolName)
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code == http.StatusTooManyRequests {
			return i
		}
	}
	return cap
}

// TestRateLimit_ExpensiveToolDrainsBucketFaster is the point of per-tool
// pricing: one caller sweeping diagnose_failure must not get the same number
// of calls as a caller doing cheap lookups.
func TestRateLimit_ExpensiveToolDrainsBucketFaster(t *testing.T) {
	const burst = 10

	// Rate 0/min so the bucket never refills mid-test and only burst matters.
	cheap := callsUntilLimited(t, internalmcp.NewRateLimiter(0, burst), "list_projects", burst+5)
	costly := callsUntilLimited(t, internalmcp.NewRateLimiter(0, burst), "diagnose_failure", burst+5)

	if cheap != burst {
		t.Errorf("cheap tool allowed %d calls, want %d (cost 1 each)", cheap, burst)
	}
	// diagnose_failure costs 5, so a burst of 10 permits exactly two calls.
	if costly != 2 {
		t.Errorf("diagnose_failure allowed %d calls, want 2 (cost 5 against burst %d)", costly, burst)
	}
	if costly >= cheap {
		t.Errorf("expensive tool (%d calls) was not limited sooner than cheap tool (%d calls)", costly, cheap)
	}
}

// TestRateLimit_UnpricedRequestsCostOne covers requests with no Mcp-Name (a
// non-tool call such as tools/list) and tools absent from the cost map.
func TestRateLimit_UnpricedRequestsCostOne(t *testing.T) {
	const burst = 4
	for _, name := range []string{"", "some_unpriced_tool"} {
		t.Run("name="+name, func(t *testing.T) {
			got := callsUntilLimited(t, internalmcp.NewRateLimiter(0, burst), name, burst+3)
			if got != burst {
				t.Errorf("allowed %d calls, want %d (unpriced requests cost 1)", got, burst)
			}
		})
	}
}

// TestRateLimit_CostClampedToBurst guards a trap in rate.Limiter: AllowN fails
// unconditionally when n exceeds the burst, so an unclamped cost above burst
// would make the tool permanently unreachable rather than merely expensive.
func TestRateLimit_CostClampedToBurst(t *testing.T) {
	// diagnose_failure costs 5 by default; a burst of 2 is smaller than that.
	got := callsUntilLimited(t, internalmcp.NewRateLimiter(0, 2), "diagnose_failure", 5)
	if got == 0 {
		t.Fatal("diagnose_failure was rejected outright under a burst smaller than its cost; the cost must be clamped")
	}
}

func TestParseToolCosts(t *testing.T) {
	t.Parallel()

	t.Run("overrides a default and adds a new tool", func(t *testing.T) {
		t.Parallel()
		costs := internalmcp.ParseToolCosts("diagnose_failure=9, list_projects=4")
		if got := costs["diagnose_failure"]; got != 9 {
			t.Errorf("diagnose_failure cost = %d, want 9 (override)", got)
		}
		if got := costs["list_projects"]; got != 4 {
			t.Errorf("list_projects cost = %d, want 4 (added)", got)
		}
		// Defaults not mentioned in the override must survive.
		if got := costs["compare_builds"]; got != 3 {
			t.Errorf("compare_builds cost = %d, want the built-in default 3", got)
		}
	})

	t.Run("empty input keeps the defaults", func(t *testing.T) {
		t.Parallel()
		costs := internalmcp.ParseToolCosts("")
		if got := costs["diagnose_failure"]; got != 5 {
			t.Errorf("diagnose_failure cost = %d, want the built-in default 5", got)
		}
	})

	t.Run("malformed entries are skipped, not fatal", func(t *testing.T) {
		t.Parallel()
		// A typo in a tuning knob must not take the server down.
		costs := internalmcp.ParseToolCosts("no_equals_sign,bad=notanumber,zero=0,negative=-3,good=7")
		if got := costs["good"]; got != 7 {
			t.Errorf("good cost = %d, want 7; valid entries must survive alongside bad ones", got)
		}
		for _, bad := range []string{"no_equals_sign", "bad", "zero", "negative"} {
			if _, ok := costs[bad]; ok {
				t.Errorf("malformed entry %q was accepted", bad)
			}
		}
	})
}
