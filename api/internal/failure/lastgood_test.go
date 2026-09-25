package failure

import (
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// TestBuildLastGoodDiff covers the pure diff summariser: the diagnosed test's
// own transition, per-category counts, and SampleRegressed — capped at ten
// and never including the diagnosed test itself.
func TestBuildLastGoodDiff(t *testing.T) {
	const thisHID = "hThis"
	const fromBuild, toBuild = int64(80), int64(100)
	regressed := func(hid, full string, durA, durB int64) store.DiffEntry {
		return store.DiffEntry{TestName: full, FullName: full, HistoryID: hid, StatusA: "passed", StatusB: "failed",
			DurationA: durA, DurationB: durB, Category: store.DiffRegressed}
	}
	many := []store.DiffEntry{regressed(thisHID, "pkg.ThisTest", 0, 0)}
	for i := range 25 {
		hid := "hCo" + string(rune('A'+i))
		many = append(many, regressed(hid, "pkg.Co"+hid, 0, 0))
	}
	thisView := DiffView{HistoryID: thisHID, FullName: "pkg.ThisTest", StatusFrom: "passed", StatusTo: "failed"}

	tests := []struct {
		name                    string
		diffs                   []store.DiffEntry
		wantThis                DiffView
		regressed, fixed, added int
		sampleLen               int
	}{
		{
			name: "this test present with mixed categories",
			diffs: []store.DiffEntry{
				regressed(thisHID, "pkg.ThisTest", 1000, 1500),
				regressed("hCo", "pkg.CoRegressed", 200, 300),
				{TestName: "pkg.Fixed", FullName: "pkg.Fixed", HistoryID: "hFix", StatusA: "failed", StatusB: "passed", Category: store.DiffFixed},
				{TestName: "pkg.Added", FullName: "pkg.Added", HistoryID: "hAdd", StatusA: "", StatusB: "failed", Category: store.DiffAdded},
			},
			wantThis:  DiffView{HistoryID: thisHID, FullName: "pkg.ThisTest", StatusFrom: "passed", StatusTo: "failed", DurationDelta: 500},
			regressed: 2, fixed: 1, added: 1, sampleLen: 1, // co-regression only
		},
		{name: "empty diff"},
		{name: "this test absent from diff", diffs: []store.DiffEntry{regressed("hOther", "pkg.Other", 100, 200)}, regressed: 1, sampleLen: 1},
		{name: "sample capped at ten", diffs: many, wantThis: thisView, regressed: 26, sampleLen: 10},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := BuildLastGoodDiff(tc.diffs, thisHID, fromBuild, toBuild)
			if got.FromBuildID != fromBuild || got.ToBuildID != toBuild {
				t.Errorf("from/to build id: got %d/%d, want %d/%d", got.FromBuildID, got.ToBuildID, fromBuild, toBuild)
			}
			if got.ThisTest != tc.wantThis {
				t.Errorf("this_test: got %+v, want %+v", got.ThisTest, tc.wantThis)
			}
			if got.RegressedCount != tc.regressed || got.FixedCount != tc.fixed || got.AddedCount != tc.added {
				t.Errorf("counts regressed/fixed/added = %d/%d/%d, want %d/%d/%d",
					got.RegressedCount, got.FixedCount, got.AddedCount, tc.regressed, tc.fixed, tc.added)
			}
			if len(got.SampleRegressed) != tc.sampleLen {
				t.Errorf("sample_regressed len: got %d, want %d", len(got.SampleRegressed), tc.sampleLen)
			}
			for _, v := range got.SampleRegressed {
				if v.HistoryID == thisHID {
					t.Errorf("sample_regressed must exclude the diagnosed test, got %+v", v)
				}
			}
		})
	}
}
