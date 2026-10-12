package handlers

import (
	"context"
	"net/http"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// TestPipelineOrigins_Unknown: a failing test with no baseline result to compare
// is counted as unknown, so "2 new" never reads as the whole answer when it is
// not; a known origin on any twin and environment both still win over unknown.
func TestPipelineOrigins_Unknown(t *testing.T) {
	tests := []struct {
		name string
		rows []store.RunFailureRow
		want map[string]any
	}{
		{name: "unknown sits next to new and inherited",
			rows: []store.RunFailureRow{
				originsRow("a", "a", store.FailureOriginNew, false),
				originsRow("b", "b", store.FailureOriginInherited, false),
				originsRow("c", "c", store.FailureOriginUnknown, false),
			},
			want: map[string]any{"new": 1.0, "inherited": 1.0, "environment": 0.0, "unknown": 1.0}},
		{name: "a twin that is new makes the test new, not unknown",
			rows: []store.RunFailureRow{
				originsRow("a", "a", store.FailureOriginUnknown, false),
				originsRow("a", "a", store.FailureOriginNew, false),
			},
			want: map[string]any{"new": 1.0, "inherited": 0.0, "environment": 0.0, "unknown": 0.0}},
		{name: "environment is never unknown",
			rows: []store.RunFailureRow{originsRow("a", "a", store.FailureOriginUnknown, true)},
			want: map[string]any{"new": 0.0, "inherited": 1.0, "environment": 1.0, "unknown": 0.0}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := originsHandler(t, "p-1", len(tc.rows), func(context.Context, int64, string, int) ([]store.RunFailureRow, error) {
				return tc.rows, nil
			})
			code, agg := serveOrigins(t, h)
			if code != http.StatusOK {
				t.Fatalf("status = %d, want 200", code)
			}
			got, ok := agg["origins"].(map[string]any)
			if !ok {
				t.Fatalf("aggregate.origins = %v, want an object", agg["origins"])
			}
			for k, want := range tc.want {
				if got[k] != want {
					t.Errorf("origins.%s = %v, want %v (origins %v)", k, got[k], want, got)
				}
			}
		})
	}
}
