package failure

import (
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// TestBuildLastGoodDiff is a table-driven test for the pure diff summariser.
// It verifies the this-test transition mapping, category counts, and the
// capped/self-excluding SampleRegressed collection.
func TestBuildLastGoodDiff(t *testing.T) {
	const thisHID = "hThis"
	const fromBuild, toBuild = int64(80), int64(100)

	// mkRegressed builds a regressed diff entry with a distinct history id.
	mkRegressed := func(hid, full string, durA, durB int64) store.DiffEntry {
		return store.DiffEntry{
			TestName: full, FullName: full, HistoryID: hid,
			StatusA: "passed", StatusB: "failed",
			DurationA: durA, DurationB: durB,
			Category: store.DiffRegressed,
		}
	}

	tests := []struct {
		name           string
		diffs          []store.DiffEntry
		wantThisTest   DiffView
		wantRegressed  int
		wantFixed      int
		wantAdded      int
		wantSampleLen  int
		sampleExcludes string // history id that must NOT appear in SampleRegressed
	}{
		{
			name: "this test present with mixed categories",
			diffs: []store.DiffEntry{
				mkRegressed(thisHID, "pkg.ThisTest", 1000, 1500),
				mkRegressed("hCo", "pkg.CoRegressed", 200, 300),
				{TestName: "pkg.Fixed", FullName: "pkg.Fixed", HistoryID: "hFix", StatusA: "failed", StatusB: "passed", Category: store.DiffFixed},
				{TestName: "pkg.Added", FullName: "pkg.Added", HistoryID: "hAdd", StatusA: "", StatusB: "failed", Category: store.DiffAdded},
			},
			wantThisTest: DiffView{
				HistoryID: thisHID, FullName: "pkg.ThisTest",
				StatusFrom: "passed", StatusTo: "failed", DurationDelta: 500,
			},
			wantRegressed:  2, // this test + one co-regression
			wantFixed:      1,
			wantAdded:      1,
			wantSampleLen:  1, // co-regression only; this test excluded
			sampleExcludes: thisHID,
		},
		{
			name:          "empty diff yields zero counts and empty this test",
			diffs:         nil,
			wantThisTest:  DiffView{},
			wantRegressed: 0,
			wantFixed:     0,
			wantAdded:     0,
			wantSampleLen: 0,
		},
		{
			name: "this test absent from diff yields empty this test but counts hold",
			diffs: []store.DiffEntry{
				mkRegressed("hOther", "pkg.Other", 100, 200),
			},
			wantThisTest:   DiffView{},
			wantRegressed:  1,
			wantFixed:      0,
			wantAdded:      0,
			wantSampleLen:  1,
			sampleExcludes: thisHID,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildLastGoodDiff(tc.diffs, thisHID, fromBuild, toBuild)

			if got.FromBuildID != fromBuild || got.ToBuildID != toBuild {
				t.Errorf("from/to build id: got %d/%d, want %d/%d", got.FromBuildID, got.ToBuildID, fromBuild, toBuild)
			}
			if got.ThisTest != tc.wantThisTest {
				t.Errorf("this_test: got %+v, want %+v", got.ThisTest, tc.wantThisTest)
			}
			if got.RegressedCount != tc.wantRegressed {
				t.Errorf("regressed_count: got %d, want %d", got.RegressedCount, tc.wantRegressed)
			}
			if got.FixedCount != tc.wantFixed {
				t.Errorf("fixed_count: got %d, want %d", got.FixedCount, tc.wantFixed)
			}
			if got.AddedCount != tc.wantAdded {
				t.Errorf("added_count: got %d, want %d", got.AddedCount, tc.wantAdded)
			}
			if len(got.SampleRegressed) != tc.wantSampleLen {
				t.Errorf("sample_regressed len: got %d, want %d", len(got.SampleRegressed), tc.wantSampleLen)
			}
			for _, v := range got.SampleRegressed {
				if tc.sampleExcludes != "" && v.HistoryID == tc.sampleExcludes {
					t.Errorf("sample_regressed must exclude %q but contained it: %+v", tc.sampleExcludes, v)
				}
			}
		})
	}
}

// TestBuildLastGoodDiff_SampleCappedAtTen verifies the SampleRegressed slice is
// capped at 10 entries and never includes the diagnosed test itself, even when
// far more than ten co-regressions exist.
func TestBuildLastGoodDiff_SampleCappedAtTen(t *testing.T) {
	const thisHID = "hThis"

	diffs := []store.DiffEntry{
		// The diagnosed test's own regression.
		{TestName: "pkg.ThisTest", FullName: "pkg.ThisTest", HistoryID: thisHID, StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed},
	}
	// 25 co-regressions, all distinct history ids.
	const coRegressions = 25
	for i := 0; i < coRegressions; i++ {
		hid := "hCo" + string(rune('A'+i))
		diffs = append(diffs, store.DiffEntry{
			TestName: hid, FullName: "pkg.Co" + hid, HistoryID: hid,
			StatusA: "passed", StatusB: "failed", Category: store.DiffRegressed,
		})
	}

	got := BuildLastGoodDiff(diffs, thisHID, 10, 20)

	// All regressions are counted (this test + 25 co-regressions).
	if got.RegressedCount != coRegressions+1 {
		t.Errorf("regressed_count: got %d, want %d", got.RegressedCount, coRegressions+1)
	}
	// The sample is capped at 10.
	if len(got.SampleRegressed) != 10 {
		t.Fatalf("sample_regressed len: got %d, want 10 (cap)", len(got.SampleRegressed))
	}
	// The diagnosed test must never appear in the sample.
	for _, v := range got.SampleRegressed {
		if v.HistoryID == thisHID {
			t.Errorf("sample_regressed must exclude the diagnosed test %q", thisHID)
		}
	}
}
