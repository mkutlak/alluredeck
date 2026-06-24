package handlers

import (
	"math"
	"testing"
)

func TestPassRateExclSkipped(t *testing.T) {
	tests := []struct {
		name    string
		passed  int
		total   int
		skipped int
		want    float64
	}{
		{
			name:    "all passed no skipped",
			passed:  100,
			total:   100,
			skipped: 0,
			want:    100.0,
		},
		{
			name:    "skipped excluded from denom",
			passed:  31,
			total:   36,
			skipped: 5,
			want:    100.0, // 31/(36-5) = 31/31 = 100%
		},
		{
			name:    "partial pass with skipped",
			passed:  85,
			total:   100,
			skipped: 10,
			want:    float64(85) / float64(90) * 100,
		},
		{
			name:    "zero total returns 0",
			passed:  0,
			total:   0,
			skipped: 0,
			want:    0,
		},
		{
			name:    "denom zero when all skipped returns 0",
			passed:  0,
			total:   5,
			skipped: 5,
			want:    0,
		},
		{
			name:    "denom negative returns 0",
			passed:  0,
			total:   3,
			skipped: 5,
			want:    0,
		},
		{
			name:    "broken and unknown still count against rate",
			passed:  80,
			total:   100,
			skipped: 5,
			want:    float64(80) / float64(95) * 100,
		},
		{
			name:    "no tests ran total zero skipped zero",
			passed:  0,
			total:   0,
			skipped: 0,
			want:    0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := passRateExclSkipped(tt.passed, tt.total, tt.skipped)
			if math.Abs(got-tt.want) > 1e-9 {
				t.Errorf("passRateExclSkipped(%d, %d, %d) = %v, want %v",
					tt.passed, tt.total, tt.skipped, got, tt.want)
			}
		})
	}
}
