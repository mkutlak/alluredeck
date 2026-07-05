package tools

import "github.com/mkutlak/alluredeck/api/internal/store"

// ---------------------------------------------------------------------------
// last-good pointer + failed-vs-last-good diff
//
// These types are part of the diagnose_failure output contract (see
// DiagnoseTest). LastGood points at the build where a failing test last passed;
// LastGoodDiff summarises the whole-build comparison between that last-good
// build and the current failing build for one test. buildLastGoodDiff is a pure
// transform over store.DiffEntry so it is unit-testable without a database.
// ---------------------------------------------------------------------------

// lastGoodSampleCap bounds the number of co-regressed tests surfaced in
// LastGoodDiff.SampleRegressed, keeping the tool output compact.
const lastGoodSampleCap = 10

// LastGood points at the build where a test last passed (branch-scoped when
// available).
type LastGood struct {
	BuildID     int64  `json:"build_id"`
	BuildNumber int    `json:"build_number"`
	CommitSHA   string `json:"commit_sha,omitempty"`
	CreatedAt   string `json:"created_at"`
	// BuildsSince is the number of prior builds of this test between the
	// last-good build and the current one (exclusive of both). It is derived
	// from the diagnose history window and so is bounded by diagnoseHistoryDepth:
	// a test that last passed further back than that depth reports at most
	// diagnoseHistoryDepth-1 here even though LastGood itself is found via an
	// uncapped query.
	BuildsSince int `json:"builds_since"`
}

// DiffView is a compact per-test status transition between two builds.
type DiffView struct {
	HistoryID     string `json:"history_id"`
	FullName      string `json:"full_name"`
	StatusFrom    string `json:"status_from"`       // status in the last-good build
	StatusTo      string `json:"status_to"`         // status in the current (failing) build
	DurationDelta int64  `json:"duration_delta_ms"` // current − last-good
}

// LastGoodDiff summarizes CompareBuildsByHistoryID(lastGood, current) for one
// failure.
type LastGoodDiff struct {
	FromBuildID     int64      `json:"from_build_id"`
	ToBuildID       int64      `json:"to_build_id"`
	ThisTest        DiffView   `json:"this_test"`
	RegressedCount  int        `json:"regressed_count"`
	FixedCount      int        `json:"fixed_count"`
	AddedCount      int        `json:"added_count"`
	SampleRegressed []DiffView `json:"sample_regressed,omitempty"` // cap 10, excludes this test
}

// diffEntryToView maps a store.DiffEntry onto the compact DiffView projection.
// The caller establishes A=last-good, B=current via the argument order of
// CompareBuildsByHistoryID(ctx, projectID, lastGood, current).
func diffEntryToView(e store.DiffEntry) DiffView {
	return DiffView{
		HistoryID:     e.HistoryID,
		FullName:      e.FullName,
		StatusFrom:    e.StatusA,
		StatusTo:      e.StatusB,
		DurationDelta: e.DurationB - e.DurationA,
	}
}

// buildLastGoodDiff filters and summarizes a whole-build diff for one test.
// diffs is the output of CompareBuildsByHistoryID(fromBuildID, toBuildID), so
// each entry's StatusA is the last-good status and StatusB the current status.
// The entry matching thisHistoryID (the diagnosed test) is projected into
// ThisTest; category counts cover every entry; SampleRegressed collects up to
// lastGoodSampleCap regressed entries excluding the diagnosed test itself.
func buildLastGoodDiff(diffs []store.DiffEntry, thisHistoryID string, fromBuildID, toBuildID int64) LastGoodDiff {
	out := LastGoodDiff{
		FromBuildID: fromBuildID,
		ToBuildID:   toBuildID,
	}
	for _, e := range diffs {
		if e.HistoryID == thisHistoryID {
			out.ThisTest = diffEntryToView(e)
		}
		switch e.Category {
		case store.DiffRegressed:
			out.RegressedCount++
			if e.HistoryID != thisHistoryID && len(out.SampleRegressed) < lastGoodSampleCap {
				out.SampleRegressed = append(out.SampleRegressed, diffEntryToView(e))
			}
		case store.DiffFixed:
			out.FixedCount++
		case store.DiffAdded:
			out.AddedCount++
		case store.DiffRemoved:
			// Removed tests are not surfaced in the last-good summary.
		}
	}
	return out
}
