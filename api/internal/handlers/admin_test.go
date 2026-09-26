package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// adminJobQueue answers the admin job endpoints from a fixed job list and
// error and records the job ID it was asked about. The embedded nil interface
// makes every other JobQueuer method an intentional panic.
type adminJobQueue struct {
	runner.JobQueuer
	jobs  []*runner.Job
	err   error
	gotID string
}

var _ runner.JobQueuer = (*adminJobQueue)(nil)

func (q *adminJobQueue) ListJobs(context.Context) []*runner.Job { return q.jobs }
func (q *adminJobQueue) Cancel(_ context.Context, id string) error {
	q.gotID = id
	return q.err
}
func (q *adminJobQueue) Retry(_ context.Context, id string) error {
	q.gotID = id
	return q.err
}
func (q *adminJobQueue) Delete(_ context.Context, id string) error {
	q.gotID = id
	return q.err
}

func TestAdminHandler_ListJobs(t *testing.T) {
	t.Parallel()
	jobs := make([]*runner.Job, 5)
	for i := range jobs {
		jobs[i] = &runner.Job{ID: fmt.Sprintf("job-%d", i+1)}
	}
	pages := func(page, perPage, total, totalPages int) map[string]int {
		return map[string]int{"page": page, "per_page": perPage, "total": total, "total_pages": totalPages}
	}
	rows := []struct {
		name    string
		query   string
		queued  int // how many of the five jobs the queue holds
		wantIDs []string
		wantPg  map[string]int
	}{
		{"empty", "", 0, nil, pages(1, 20, 0, 0)},
		{"defaults to 20 per page", "", 2, []string{"job-1", "job-2"}, pages(1, 20, 2, 1)},
		{"first page", "page=1&per_page=2", 5, []string{"job-1", "job-2"}, pages(1, 2, 5, 3)},
		{"second page", "page=2&per_page=2", 5, []string{"job-3", "job-4"}, pages(2, 2, 5, 3)},
		{"beyond the last page", "page=100&per_page=2", 5, nil, pages(100, 2, 5, 3)},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := NewAdminHandler(&adminJobQueue{jobs: jobs[:tc.queued]}, &testutil.MockStorage{}, zap.NewNop())
			rr := httptest.NewRecorder()
			h.ListJobs(rr, httptest.NewRequest(http.MethodGet, "/api/v1/admin/jobs?"+tc.query, nil))
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
			}
			var resp struct {
				Data []struct {
					ID string `json:"job_id"`
				} `json:"data"`
				Pagination map[string]int `json:"pagination"`
			}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Data == nil {
				t.Error("data = null, want an array")
			}
			var ids []string
			for _, j := range resp.Data {
				ids = append(ids, j.ID)
			}
			if !slices.Equal(ids, tc.wantIDs) {
				t.Errorf("job ids = %v, want %v", ids, tc.wantIDs)
			}
			if !maps.Equal(resp.Pagination, tc.wantPg) {
				t.Errorf("pagination = %v, want %v", resp.Pagination, tc.wantPg)
			}
		})
	}
}

func TestAdminHandler_JobActions(t *testing.T) {
	t.Parallel()
	cancel, retry, del := (*AdminHandler).CancelJob, (*AdminHandler).RetryJob, (*AdminHandler).DeleteJob
	notFound := fmt.Errorf("job %q: %w", "j1", runner.ErrJobNotFound)
	rows := []struct {
		name   string
		action func(*AdminHandler, http.ResponseWriter, *http.Request)
		jobID  string
		err    error // what the job queue answers
		want   int
	}{
		{"cancel", cancel, "j1", nil, http.StatusOK},
		{"cancel unknown job", cancel, "j1", notFound, http.StatusNotFound},
		// Any other queue error means the job already reached a terminal state.
		{"cancel finished job", cancel, "j1", errors.New(`job "j1" is already in terminal state`), http.StatusConflict},
		{"retry unknown job", retry, "j1", notFound, http.StatusNotFound},
		{"delete", del, "j1", nil, http.StatusOK},
		{"delete without job id", del, "", nil, http.StatusBadRequest},
		{"delete unknown job", del, "j1", notFound, http.StatusNotFound},
		{"delete running job", del, "j1", fmt.Errorf("job %q: %w", "j1", runner.ErrJobNotTerminal), http.StatusConflict},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			q := &adminJobQueue{err: tc.err}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/jobs/"+tc.jobID, nil)
			req.SetPathValue("job_id", tc.jobID)
			rr := httptest.NewRecorder()
			tc.action(NewAdminHandler(q, &testutil.MockStorage{}, zap.NewNop()), rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.want == http.StatusOK && q.gotID != tc.jobID {
				t.Errorf("queue got job %q, want %q", q.gotID, tc.jobID)
			}
		})
	}
}

func TestAdminHandler_ListPendingResults(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	projects := testutil.NewMemProjectStore()
	top, _ := projects.CreateProject(ctx, "my-top")
	parent, _ := projects.CreateProject(ctx, "parent")
	child, _ := projects.CreateProjectWithParent(ctx, "ui-permissions", parent.ID)
	pending := func(p *store.Project) []pendingResultsEntry {
		return []pendingResultsEntry{{ProjectID: p.ID, Slug: p.Slug, StorageKey: p.StorageKey, FileCount: 2, TotalSize: 3072}}
	}
	rows := []struct {
		name     string
		dirs     []string // storage dirs; every dir but "empty" holds two result files
		projects store.ProjectStorer
		want     []pendingResultsEntry
	}{
		{"no pending files", []string{"empty"}, projects, []pendingResultsEntry{}},
		{"without a project store the storage key doubles as slug", []string{"proj-a", "empty"}, nil,
			pending(&store.Project{Slug: "proj-a", StorageKey: "proj-a"})},
		// cf6e4f3: storage dirs are storage keys and resolve to the owning project
		// row — a top-level project by slug, a child by its numeric storage key.
		{"top-level project", []string{top.StorageKey}, projects, pending(top)},
		{"child project", []string{child.StorageKey}, projects, pending(child)},
		{"orphan storage dir skipped", []string{"99"}, projects, []pendingResultsEntry{}},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ms := &testutil.MockStorage{
				ListProjectsFn: func(context.Context) ([]string, error) { return tc.dirs, nil },
				ReadDirFn: func(_ context.Context, dir, _ string) ([]storage.DirEntry, error) {
					if dir == "empty" {
						return nil, nil
					}
					mod := time.Now().UnixNano()
					return []storage.DirEntry{{Name: "r1.json", Size: 1024, ModTime: mod}, {Name: "r2.json", Size: 2048, ModTime: mod}}, nil
				},
			}
			h := NewAdminHandlerWithProjects(&adminJobQueue{}, ms, tc.projects, zap.NewNop())
			rr := httptest.NewRecorder()
			h.ListPendingResults(rr, httptest.NewRequest(http.MethodGet, "/api/v1/admin/results", nil))
			if rr.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
			}
			var resp struct {
				Data []pendingResultsEntry `json:"data"`
			}
			if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
				t.Fatalf("decode: %v", err)
			}
			if resp.Data == nil {
				t.Error("data = null, want an array")
			}
			for i := range resp.Data {
				resp.Data[i].LastModified = time.Time{}
			}
			if !slices.Equal(resp.Data, tc.want) {
				t.Errorf("data = %+v, want %+v", resp.Data, tc.want)
			}
		})
	}
}

func TestAdminHandler_CleanProjectResults(t *testing.T) {
	t.Parallel()
	projects := testutil.NewMemProjectStore()
	parent, _ := projects.CreateProject(context.Background(), "parent")
	child, _ := projects.CreateProjectWithParent(context.Background(), "child", parent.ID)
	rows := []struct {
		name, projectID string
		want            int
		wantCleaned     string
	}{
		// Result files live under the storage key, not the slug.
		{"cleans by storage key", strconv.FormatInt(child.ID, 10), http.StatusOK, child.StorageKey},
		{"unknown project", "99", http.StatusNotFound, ""},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var cleaned string
			ms := &testutil.MockStorage{CleanResultsFn: func(_ context.Context, key string) error {
				cleaned = key
				return nil
			}}
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/results/"+tc.projectID, nil)
			req.SetPathValue("project_id", tc.projectID)
			rr := httptest.NewRecorder()
			NewAdminHandlerWithProjects(&adminJobQueue{}, ms, projects, zap.NewNop()).CleanProjectResults(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if cleaned != tc.wantCleaned {
				t.Errorf("cleaned storage key = %q, want %q", cleaned, tc.wantCleaned)
			}
		})
	}
}
