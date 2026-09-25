package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// analyticsStub returns an AnalyticsStorer whose four list methods return n
// rows, or err, and record the branch ID they were asked to filter by.
func analyticsStub(n int, err error, gotBranch **int64) *testutil.MockAnalyticsStore {
	return &testutil.MockAnalyticsStore{
		ListTopErrorsFn: func(_ context.Context, _ []int64, _, _ int, b *int64) ([]store.ErrorCluster, error) {
			*gotBranch = b
			return make([]store.ErrorCluster, n), err
		},
		ListSuitePassRatesFn: func(_ context.Context, _ []int64, _ int, b *int64) ([]store.SuitePassRate, error) {
			*gotBranch = b
			return make([]store.SuitePassRate, n), err
		},
		ListLabelBreakdownFn: func(_ context.Context, _ []int64, _ string, _ int, b *int64) ([]store.LabelCount, error) {
			*gotBranch = b
			return make([]store.LabelCount, n), err
		},
		ListTrendPointsFn: func(_ context.Context, _ []int64, _ int, b *int64) ([]store.TrendPoint, error) {
			*gotBranch = b
			return make([]store.TrendPoint, n), err
		},
	}
}

// TestAnalyticsHandler runs every row against each analytics endpoint. The
// trends endpoint answers with parallel status/pass_rate/duration series and a
// KPI block instead of a flat list.
func TestAnalyticsHandler(t *testing.T) {
	endpoints := []struct {
		name  string
		serve func(*AnalyticsHandler, http.ResponseWriter, *http.Request)
	}{
		{"errors", (*AnalyticsHandler).GetTopErrors},
		{"suites", (*AnalyticsHandler).GetSuitePassRates},
		{"labels", (*AnalyticsHandler).GetLabelBreakdown},
		{"trends", (*AnalyticsHandler).GetTrends},
	}
	const branchMain = 42
	branches := &testutil.MockBranchStore{
		GetByNameFn: func(_ context.Context, _ int64, name string) (*store.Branch, error) {
			if name == "main" {
				return &store.Branch{ID: branchMain, Name: "main"}, nil
			}
			return nil, store.ErrBranchNotFound
		},
	}
	rows := []struct {
		name       string
		noStore    bool // analytics unavailable: every endpoint degrades to empty data
		query      string
		n          int
		err        error
		want       int
		wantRows   int
		wantBranch int64 // 0: the query is not filtered by branch
	}{
		{name: "no analytics store", noStore: true, want: http.StatusOK},
		{name: "rows", n: 3, want: http.StatusOK, wantRows: 3},
		{name: "branch resolves to its id", query: "branch=main", want: http.StatusOK, wantBranch: branchMain},
		// An unknown branch leaves the query unfiltered rather than failing.
		{name: "unknown branch", query: "branch=nonexistent", want: http.StatusOK},
		// Store errors map to 500 without leaking driver details.
		{name: "store error", err: errors.New("pq: connection refused"), want: http.StatusInternalServerError},
	}
	for _, ep := range endpoints {
		for _, tc := range rows {
			t.Run(ep.name+"/"+tc.name, func(t *testing.T) {
				var gotBranch *int64
				var as store.AnalyticsStorer = analyticsStub(tc.n, tc.err, &gotBranch)
				if tc.noStore {
					as = nil
				}
				h := NewAnalyticsHandler(as, branches, nil, zap.NewNop())
				code, body := serveJSON(t, func(w http.ResponseWriter, r *http.Request) { ep.serve(h, w, r) },
					http.MethodGet, "/api/v1/projects/1/analytics?"+tc.query, "", "project_id", "1")
				if code != tc.want {
					t.Fatalf("status = %d, want %d: %v", code, tc.want, body)
				}
				if tc.err != nil {
					if strings.Contains(fmt.Sprint(body), "connection refused") {
						t.Errorf("response leaks the store error: %v", body)
					}
					return
				}
				if got := derefInt64(gotBranch); got != tc.wantBranch {
					t.Errorf("branch id = %d, want %d", got, tc.wantBranch)
				}
				if ep.name != "trends" {
					wantJSON(t, body, map[string]any{"data#": tc.wantRows})
					return
				}
				wantJSON(t, body, map[string]any{"data.status#": tc.wantRows, "data.pass_rate#": tc.wantRows, "data.duration#": tc.wantRows})
				if kpi := jsonAt(body, "data.kpi"); (kpi != nil) != (tc.wantRows > 0) {
					t.Errorf("kpi = %v, want present only when there are trend points", kpi)
				}
			})
		}
	}
}

// TestBuildTrendsResponse_KPI pins the KPI block: sparklines hold the last ten
// builds and the headline values come from the newest one.
func TestBuildTrendsResponse_KPI(t *testing.T) {
	points := make([]store.TrendPoint, 15)
	for i := range points {
		points[i] = store.TrendPoint{BuildNumber: i + 1, Total: 47 + i, PassRate: float64(i), DurationMs: int64(60000 - i*1000)}
	}
	kpi := buildTrendsResponse(points).Kpi
	if kpi == nil {
		t.Fatal("kpi = nil, want a KPI block")
	}
	if len(kpi.PassRateTrend) != 10 || len(kpi.DurationTrend) != 10 {
		t.Errorf("sparkline lengths = %d/%d, want 10/10", len(kpi.PassRateTrend), len(kpi.DurationTrend))
	}
	if kpi.PassRate != points[14].PassRate || kpi.TotalTests != points[14].Total {
		t.Errorf("kpi pass_rate/total_tests = %v/%d, want the newest build's %v/%d", kpi.PassRate, kpi.TotalTests, points[14].PassRate, points[14].Total)
	}
}
