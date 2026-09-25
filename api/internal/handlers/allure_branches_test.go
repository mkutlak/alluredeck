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

// TestBranchHandler serves every branch endpoint against a store where project
// 1 owns branch 1 (the default) and branch 2; no other project/branch pair
// exists.
func TestBranchHandler(t *testing.T) {
	t.Parallel()
	list, setDefault, del := (*BranchHandler).ListBranches, (*BranchHandler).SetDefaultBranch, (*BranchHandler).DeleteBranch
	rows := []struct {
		name              string
		endpoint          func(*BranchHandler, http.ResponseWriter, *http.Request)
		project, branchID string
		want              int
		wantBranches      int // ListBranches only
	}{
		{"list", list, "1", "", http.StatusOK, 2},
		{"list empty", list, "2", "", http.StatusOK, 0},
		{"set default", setDefault, "1", "2", http.StatusOK, 0},
		{"set default unknown branch", setDefault, "1", "9999", http.StatusNotFound, 0},
		{"set default branch of another project", setDefault, "2", "2", http.StatusNotFound, 0},
		{"set default invalid branch id", setDefault, "1", "notanumber", http.StatusBadRequest, 0},
		{"delete", del, "1", "2", http.StatusNoContent, 0},
		{"delete default branch", del, "1", "1", http.StatusConflict, 0},
		{"delete unknown branch", del, "1", "9999", http.StatusNotFound, 0},
		{"delete invalid branch id", del, "1", "notanumber", http.StatusBadRequest, 0},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			mocks := testutil.New()
			notFound := func(pid, bid int64) error {
				return fmt.Errorf("%w: branch=%d project=%d", store.ErrBranchNotFound, bid, pid)
			}
			mocks.Branches.ListFn = func(_ context.Context, pid int64) ([]store.Branch, error) {
				if pid != 1 {
					return []store.Branch{}, nil
				}
				return []store.Branch{
					{ID: 1, ProjectID: 1, Name: "main", IsDefault: true, CreatedAt: time.Now()},
					{ID: 2, ProjectID: 1, Name: "dev", CreatedAt: time.Now()},
				}, nil
			}
			mocks.Branches.SetDefaultFn = func(_ context.Context, pid, bid int64) error {
				if pid == 1 && (bid == 1 || bid == 2) {
					return nil
				}
				return notFound(pid, bid)
			}
			mocks.Branches.DeleteFn = func(_ context.Context, pid, bid int64) error {
				switch {
				case pid == 1 && bid == 1:
					return store.ErrCannotDeleteDefaultBranch
				case pid == 1 && bid == 2:
					return nil
				}
				return notFound(pid, bid)
			}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/"+tc.project+"/branches", nil)
			req.SetPathValue("project_id", tc.project)
			req.SetPathValue("branch_id", tc.branchID)
			rr := httptest.NewRecorder()
			tc.endpoint(NewBranchHandler(mocks.Branches, mocks.Builds, mocks.Projects), rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.want != http.StatusOK || tc.branchID != "" {
				return
			}
			var resp struct {
				Data struct {
					Branches []map[string]any `json:"branches"`
				} `json:"data"`
			}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Data.Branches == nil || len(resp.Data.Branches) != tc.wantBranches {
				t.Fatalf("branches = %v, want an array of %d", resp.Data.Branches, tc.wantBranches)
			}
			for _, b := range resp.Data.Branches {
				for _, field := range []string{"id", "project_id", "name", "is_default", "created_at"} {
					if _, ok := b[field]; !ok {
						t.Errorf("branch %v lacks %q", b, field)
					}
				}
			}
		})
	}
}
