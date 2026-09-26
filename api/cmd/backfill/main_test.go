package main

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/bootstrap"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestBackfillFingerprints_DryRun walks a two-project topology (project 1:
// builds 10, 11; project 2: builds 20, 21) through the scope filters and the
// per-project and per-build failure paths. Dry runs never reach allureCore.
func TestBackfillFingerprints_DryRun(t *testing.T) {
	all := []string{"1/10", "1/11", "2/20", "2/21"}
	each := func(n int) map[int64]int { return map[int64]int{10: n, 11: n, 20: n, 21: n} }
	tests := []struct {
		name        string
		flags       backfillFlags
		failed      map[int64]int // failed tests per build; -1 fails the lookup
		listErrFor  int64         // project whose ListBuilds fails
		want        backfillResult
		wantQueried []string // project/build pairs whose failed tests were listed
	}{
		{"all scope", backfillFlags{}, each(2), 0, backfillResult{succeeded: 4, totalFailedTests: 8}, all},
		{"project scope", backfillFlags{projectID: 2}, each(1), 0, backfillResult{succeeded: 2, totalFailedTests: 2}, all[2:]},
		{"since-build scope", backfillFlags{sinceBuild: 20}, each(1), 0, backfillResult{succeeded: 2, totalFailedTests: 2}, all[2:]},
		{"failed lookup is recorded and the run continues", backfillFlags{}, map[int64]int{10: 1, 11: -1, 20: 1, 21: 1}, 0,
			backfillResult{succeeded: 3, totalFailedTests: 3, failedBuildIDs: []int64{11}}, all},
		{"ListBuilds failure skips only that project", backfillFlags{}, each(1), 1, backfillResult{succeeded: 2, totalFailedTests: 2}, all[2:]},
		{"builds without failures are not counted", backfillFlags{}, map[int64]int{10: 3}, 0, backfillResult{succeeded: 1, totalFailedTests: 3}, all},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var queried []string
			stores := &bootstrap.Stores{
				Project: &testutil.MockProjectStore{
					ListProjectsFn: func(context.Context) ([]store.Project, error) {
						return []store.Project{{ID: 1, Slug: "alpha"}, {ID: 2, Slug: "beta"}}, nil
					},
				},
				Build: &testutil.MockBuildStore{
					ListBuildsFn: func(_ context.Context, projectID int64) ([]store.Build, error) {
						if projectID == tc.listErrFor {
							return nil, errors.New("simulated list builds failure")
						}
						base := projectID * 10
						return []store.Build{{ID: base, ProjectID: projectID}, {ID: base + 1, ProjectID: projectID}}, nil
					},
				},
				TestResult: &testutil.MockTestResultStore{
					ListFailedForFingerprintingFn: func(_ context.Context, projectID, buildID int64) ([]store.FailedTestResult, error) {
						queried = append(queried, fmt.Sprintf("%d/%d", projectID, buildID))
						n := tc.failed[buildID]
						if n < 0 {
							return nil, errors.New("simulated query failure")
						}
						return make([]store.FailedTestResult, n), nil
					},
				},
			}
			tc.flags.dryRun = true

			got := backfillFingerprints(context.Background(), nil, stores, tc.flags, zap.NewNop())
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("result = %+v, want %+v", got, tc.want)
			}
			if !slices.Equal(queried, tc.wantQueried) {
				t.Errorf("queried = %v, want %v", queried, tc.wantQueried)
			}
		})
	}
}
