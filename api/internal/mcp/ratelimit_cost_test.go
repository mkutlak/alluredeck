package mcp_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	internalmcp "github.com/mkutlak/alluredeck/api/internal/mcp"
)

// TestRateLimit_ToolCost drives the middleware with one Mcp-Name until it
// answers 429. No TokenInfo is injected, so every request shares the
// "unknown" identity's bucket; cost accounting is what is under test. The
// rate is 0/min so the bucket never refills and only the burst matters.
func TestRateLimit_ToolCost(t *testing.T) {
	tests := []struct {
		name  string
		tool  string
		burst int
		want  int
	}{
		{"cheap tool costs 1", "list_projects", 10, 10},
		// Per-tool pricing: a caller sweeping diagnose_failure (cost 5) must
		// not get as many calls as one doing cheap lookups.
		{"expensive tool drains the bucket faster", "diagnose_failure", 10, 2},
		{"non-tool request costs 1", "", 4, 4},
		{"unpriced tool costs 1", "some_unpriced_tool", 4, 4},
		// rate.Limiter.AllowN fails unconditionally when n exceeds the burst,
		// so an unclamped cost would make the tool unreachable.
		{"cost is clamped to the burst", "diagnose_failure", 2, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			handler := internalmcp.NewRateLimiter(0, tc.burst).Middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))
			allowed := 0
			for range tc.burst + 5 {
				req := httptest.NewRequest(http.MethodPost, "/mcp", nil)
				if tc.tool != "" {
					req.Header.Set("Mcp-Name", tc.tool)
				}
				rec := httptest.NewRecorder()
				handler.ServeHTTP(rec, req)
				if rec.Code == http.StatusTooManyRequests {
					break
				}
				allowed++
			}
			if allowed != tc.want {
				t.Errorf("allowed %d calls, want %d", allowed, tc.want)
			}
		})
	}
}

// TestParseToolCosts: an MCP_TOOL_COSTS override retunes and adds tools on top
// of the built-in defaults, and a typo in a tuning knob is skipped rather than
// taking the server down.
func TestParseToolCosts(t *testing.T) {
	t.Parallel()
	costs := internalmcp.ParseToolCosts("diagnose_failure=9, list_projects=4,no_equals_sign,bad=notanumber,zero=0,negative=-3")

	for tool, want := range map[string]int{"diagnose_failure": 9, "list_projects": 4, "compare_builds": 3} {
		if got := costs[tool]; got != want {
			t.Errorf("cost[%s] = %d, want %d", tool, got, want)
		}
	}
	for _, bad := range []string{"no_equals_sign", "bad", "zero", "negative"} {
		if _, ok := costs[bad]; ok {
			t.Errorf("malformed entry %q was accepted", bad)
		}
	}
}
