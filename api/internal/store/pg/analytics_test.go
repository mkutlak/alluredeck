package pg_test

import (
	"math"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// flaky is a stability row with the given duration and flaky/retry data.
func (f *fixture) flaky(buildID int64, fullName, status string, durationMs int64, isFlaky bool, retries int) store.TestResult {
	r := f.result(buildID, fullName, status, "")
	r.DurationMs, r.Flaky, r.Retries = durationMs, isFlaky, retries
	return r
}

// TestListFlakyImpact_AggregatesFlakyAndRetryData: "t1" is flaky in builds 1
// and 3 and fails cleanly in build 2; "t2" is never flaky and is excluded
// (HAVING flaky_count > 0). t1's row aggregates flaky_count, retry_sum,
// wasted_ms (retries*duration_ms), failure_rate and runs across the window,
// with first/last-seen build order and id, the last-seen build's CI URL and
// created_at.
func TestListFlakyImpact_AggregatesFlakyAndRetryData(t *testing.T) {
	f := newFixture(t)
	b1, b2, b3 := f.build(1), f.build(2), f.build(3)
	const ciURL = "https://ci.example.com/build/3"
	if err := f.builds.UpdateBuildCIMetadata(f.ctx, f.id, 3, store.CIMetadata{BuildURL: ciURL}); err != nil {
		t.Fatalf("UpdateBuildCIMetadata: %v", err)
	}
	f.insert(
		f.flaky(b1, "spec/foo.ts > t1", "passed", 1000, true, 2), f.flaky(b1, "spec/foo.ts > t2", "passed", 500, false, 0),
		f.flaky(b2, "spec/foo.ts > t1", "failed", 1200, false, 0), f.flaky(b2, "spec/foo.ts > t2", "passed", 500, false, 0),
		f.flaky(b3, "spec/foo.ts > t1", "passed", 800, true, 1), f.flaky(b3, "spec/foo.ts > t2", "passed", 500, false, 0),
	)

	impact, err := pg.NewAnalyticsStore(f.s).ListFlakyImpact(f.ctx, f.id, nil, 10, 10)
	if err != nil {
		t.Fatalf("ListFlakyImpact: %v", err)
	}
	if len(impact) != 1 {
		t.Fatalf("impact rows = %+v, want only t1", impact)
	}
	got := impact[0]
	build3, err := f.builds.GetBuildByID(f.ctx, f.id, b3)
	if err != nil {
		t.Fatalf("GetBuildByID: %v", err)
	}
	if d := got.LastSeenAt.Sub(build3.CreatedAt); d > time.Second || d < -time.Second {
		t.Errorf("LastSeenAt = %v, want ~build 3 created_at %v", got.LastSeenAt, build3.CreatedAt)
	}
	if math.Abs(got.FailureRate-1.0/3.0) > 0.001 {
		t.Errorf("FailureRate = %f, want ~1/3", got.FailureRate)
	}
	got.LastSeenAt, got.FailureRate = time.Time{}, 0
	want := store.FlakyImpact{
		FullName: "spec/foo.ts > t1", FlakyCount: 2, RetrySum: 3, WastedMs: 2*1000 + 0*1200 + 1*800, Runs: 3,
		BuildsAffected: 2, FirstSeenBuildOrder: 1, FirstSeenBuildID: b1, LastSeenBuildOrder: 3, LastSeenBuildID: b3, CIBuildURL: ciURL,
	}
	if got != want {
		t.Errorf("impact = %+v, want %+v", got, want)
	}
}

// TestListFlakyImpact_RespectsLimitAndBranch: limit caps the rows and a
// branch filter excludes tests whose builds are on another branch.
func TestListFlakyImpact_RespectsLimitAndBranch(t *testing.T) {
	f := newFixture(t)
	main := f.branch("main")
	for i, name := range []string{"main", "feature"} {
		b := f.build(i + 1)
		if err := f.builds.UpdateBuildBranchID(f.ctx, f.id, i+1, f.branch(name).ID); err != nil {
			t.Fatalf("UpdateBuildBranchID: %v", err)
		}
		f.insert(f.flaky(b, "spec/branch.ts > "+name+"-only", "passed", 100, true, 1))
	}
	as := pg.NewAnalyticsStore(f.s)
	for _, tt := range []struct {
		name     string
		branchID *int64
		limit    int
		count    int
		first    string // "" = only the count is checked
	}{
		{"unfiltered", nil, 10, 2, ""},
		{"limit caps rows", nil, 1, 1, ""},
		{"branch filter", &main.ID, 10, 1, "spec/branch.ts > main-only"},
	} {
		got, err := as.ListFlakyImpact(f.ctx, f.id, tt.branchID, 10, tt.limit)
		if err != nil {
			t.Fatalf("%s: %v", tt.name, err)
		}
		if len(got) != tt.count || tt.first != "" && got[0].FullName != tt.first {
			t.Errorf("%s: impact = %+v, want %d rows, first %q", tt.name, got, tt.count, tt.first)
		}
	}
}
