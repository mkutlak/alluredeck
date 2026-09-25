package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestTestHistoryHandler_GetTestHistory(t *testing.T) {
	t.Parallel()
	rows := []struct {
		name    string
		query   string
		want    int
		wantLen int
	}{
		{"missing history_id", "", http.StatusBadRequest, 0},
		{"no results", "history_id=nonexistent", http.StatusOK, 0},
		{"results", "history_id=abc123", http.StatusOK, 3},
		{"unknown branch", "history_id=abc123&branch=nonexistent-branch", http.StatusNotFound, 0},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mocks := testutil.New()
			mocks.TestResults.GetTestHistoryFn = func(_ context.Context, _ int64, historyID string, _ *int64, _ int) ([]store.TestHistoryEntry, error) {
				if historyID != "abc123" {
					return []store.TestHistoryEntry{}, nil
				}
				now := time.Now()
				return []store.TestHistoryEntry{
					{BuildNumber: 1, BuildID: 101, Status: "passed", DurationMs: 400, CreatedAt: now},
					{BuildNumber: 2, BuildID: 102, Status: "passed", DurationMs: 800, CreatedAt: now},
					{BuildNumber: 3, BuildID: 103, Status: "failed", DurationMs: 1200, CreatedAt: now},
				}, nil
			}
			mocks.Branches.GetByNameFn = func(_ context.Context, _ int64, name string) (*store.Branch, error) {
				return nil, fmt.Errorf("%w: branch=%s", store.ErrBranchNotFound, name)
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/1/test-history?"+tc.query, nil)
			req.SetPathValue("project_id", "1")
			rr := httptest.NewRecorder()
			NewTestHistoryHandler(mocks.TestResults, mocks.Builds, mocks.Branches, mocks.Projects).GetTestHistory(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.want != http.StatusOK {
				return
			}
			var resp struct {
				Data struct {
					HistoryID string           `json:"history_id"`
					History   []map[string]any `json:"history"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			if resp.Data.History == nil || len(resp.Data.History) != tc.wantLen {
				t.Fatalf("history = %v, want an array of %d", resp.Data.History, tc.wantLen)
			}
			if tc.wantLen == 0 {
				return
			}
			if resp.Data.HistoryID != "abc123" {
				t.Errorf("history_id = %q, want abc123", resp.Data.HistoryID)
			}
			for _, field := range []string{"build_number", "build_id", "status", "duration_ms", "created_at"} {
				if _, ok := resp.Data.History[0][field]; !ok {
					t.Errorf("history entry lacks %q", field)
				}
			}
		})
	}
}
