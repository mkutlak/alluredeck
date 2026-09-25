package handlers

import (
	"math"
	"testing"
)

// TestPassRateExclSkipped pins the pass-rate rule used everywhere:
// passed/(total-skipped); failed, broken and unknown still count against it.
func TestPassRateExclSkipped(t *testing.T) {
	tests := []struct {
		name                   string
		passed, total, skipped int
		want                   float64
	}{
		{"all passed no skipped", 100, 100, 0, 100},
		{"skipped excluded from denom", 31, 36, 5, 100},
		{"broken and unknown still count against rate", 85, 100, 10, float64(85) / 90 * 100},
		{"zero total returns 0", 0, 0, 0, 0},
		{"denom zero when all skipped returns 0", 0, 5, 5, 0},
		{"denom negative returns 0", 0, 3, 5, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := passRateExclSkipped(tt.passed, tt.total, tt.skipped); math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("passRateExclSkipped(%d, %d, %d) = %v, want %v", tt.passed, tt.total, tt.skipped, got, tt.want)
			}
		})
	}
}
