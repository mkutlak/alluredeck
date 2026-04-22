package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// newAdminTestJobManager creates a MemJobManager suitable for handler tests.
func newAdminTestJobManager(t *testing.T, gen runner.ReportGenerator, poolSize int) runner.JobQueuer {
	t.Helper()
	jm := runner.NewMemJobManager(gen, poolSize, zap.NewNop())
	jm.Start(context.Background())
	t.Cleanup(func() { jm.Shutdown() })
	return jm
}

func newTestAdminHandler(t *testing.T, jm runner.JobQueuer, ms *storage.MockStore) *AdminHandler {
	t.Helper()
	return NewAdminHandler(jm, ms, zap.NewNop())
}

// blockingGen blocks until ch is closed (for keeping jobs in pending/running state).
type blockingGen struct {
	ch chan struct{}
}

func newBlockingGen() *blockingGen {
	return &blockingGen{ch: make(chan struct{})}
}

func (g *blockingGen) GenerateReport(_ context.Context, _ int64, _, _, _, _, _, _ string, _ bool, _, _, _, _ string) (string, error) {
	<-g.ch
	return "ok", nil
}

// contextGen blocks until ch or ctx.Done() (for cancellation-aware tests).
type contextGen struct {
	ch chan struct{}
}

func newContextGen() *contextGen {
	return &contextGen{ch: make(chan struct{})}
}

func (g *contextGen) GenerateReport(ctx context.Context, _ int64, _, _, _, _, _, _ string, _ bool, _, _, _, _ string) (string, error) {
	select {
	case <-g.ch:
		return "ok", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// ---------------------------------------------------------------------------
// ListJobs
// ---------------------------------------------------------------------------

func TestAdminListJobs_Empty(t *testing.T) {
	gen := newBlockingGen()
	defer close(gen.ch)

	jm := newAdminTestJobManager(t, gen, 2)
	h := newTestAdminHandler(t, jm, &storage.MockStore{})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/admin/jobs", nil)
	rr := httptest.NewRecorder()
	h.ListJobs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected data to be array, got %T", resp["data"])
	}
	if len(data) != 0 {
		t.Errorf("expected empty data array, got %d items", len(data))
	}
}

func TestAdminListJobs_ReturnsJobs(t *testing.T) {
	gen := newBlockingGen()
	defer close(gen.ch)

	jm := newAdminTestJobManager(t, gen, 4)
	jm.Submit(1, "proj-admin-1", runner.JobParams{})
	jm.Submit(2, "proj-admin-2", runner.JobParams{})

	h := newTestAdminHandler(t, jm, &storage.MockStore{})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/admin/jobs", nil)
	rr := httptest.NewRecorder()
	h.ListJobs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected data array, got %T", resp["data"])
	}
	if len(data) != 2 {
		t.Errorf("expected 2 jobs, got %d", len(data))
	}
}

// ---------------------------------------------------------------------------
// ListPendingResults
// ---------------------------------------------------------------------------

func TestAdminListPendingResults_Empty(t *testing.T) {
	gen := newBlockingGen()
	defer close(gen.ch)

	ms := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{"proj-a"}, nil
		},
		ReadDirFn: func(_ context.Context, _ string, _ string) ([]storage.DirEntry, error) {
			return nil, nil // no result files
		},
	}
	jm := newAdminTestJobManager(t, gen, 2)
	h := newTestAdminHandler(t, jm, ms)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/admin/results", nil)
	rr := httptest.NewRecorder()
	h.ListPendingResults(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected data array, got %T", resp["data"])
	}
	if len(data) != 0 {
		t.Errorf("expected empty result, got %d", len(data))
	}
}

func TestAdminListPendingResults_ReturnsProjects(t *testing.T) {
	gen := newBlockingGen()
	defer close(gen.ch)

	modTime := time.Now().UnixNano()
	ms := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{"proj-with-results", "proj-empty"}, nil
		},
		ReadDirFn: func(_ context.Context, projectID string, _ string) ([]storage.DirEntry, error) {
			if projectID == "proj-with-results" {
				return []storage.DirEntry{
					{Name: "result-001.json", Size: 1024, ModTime: modTime},
					{Name: "result-002.json", Size: 2048, ModTime: modTime},
				}, nil
			}
			return nil, nil
		},
	}
	jm := newAdminTestJobManager(t, gen, 2)
	h := newTestAdminHandler(t, jm, ms)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/admin/results", nil)
	rr := httptest.NewRecorder()
	h.ListPendingResults(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected data array, got %T", resp["data"])
	}
	if len(data) != 1 {
		t.Fatalf("expected 1 project with results, got %d", len(data))
	}

	entry, ok := data[0].(map[string]any)
	if !ok {
		t.Fatalf("expected entry to be object")
	}
	if entry["slug"] != "proj-with-results" {
		t.Errorf("wrong slug: %v", entry["slug"])
	}
	if entry["file_count"].(float64) != 2 {
		t.Errorf("expected file_count=2, got %v", entry["file_count"])
	}
	if entry["total_size"].(float64) != 3072 {
		t.Errorf("expected total_size=3072, got %v", entry["total_size"])
	}
}

// ---------------------------------------------------------------------------
// CancelJob
// ---------------------------------------------------------------------------

func TestAdminCancelJob_NotFound(t *testing.T) {
	gen := newBlockingGen()
	defer close(gen.ch)

	jm := newAdminTestJobManager(t, gen, 2)
	h := newTestAdminHandler(t, jm, &storage.MockStore{})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/admin/jobs/bogus-id/cancel", nil)
	req.SetPathValue("job_id", "bogus-id")
	rr := httptest.NewRecorder()
	h.CancelJob(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAdminCancelJob_AlreadyTerminal(t *testing.T) {
	gen := newBlockingGen()

	jm := newAdminTestJobManager(t, gen, 2)
	j := jm.Submit(1, "proj-terminal", runner.JobParams{})
	close(gen.ch)

	// Wait for completion.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := jm.Get(j.ID); got != nil && got.Status == runner.JobStatusCompleted {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	h := newTestAdminHandler(t, jm, &storage.MockStore{})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/admin/jobs/"+j.ID+"/cancel", nil)
	req.SetPathValue("job_id", j.ID)
	rr := httptest.NewRecorder()
	h.CancelJob(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("want 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAdminCancelJob_Success(t *testing.T) {
	gen := newContextGen()
	jm := newAdminTestJobManager(t, gen, 2)
	j := jm.Submit(1, "proj-cancel-handler", runner.JobParams{})

	// Wait for running.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := jm.Get(j.ID); got != nil && got.Status == runner.JobStatusRunning {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	h := newTestAdminHandler(t, jm, &storage.MockStore{})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/admin/jobs/"+j.ID+"/cancel", nil)
	req.SetPathValue("job_id", j.ID)
	rr := httptest.NewRecorder()
	h.CancelJob(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ---------------------------------------------------------------------------
// CleanProjectResults
// ---------------------------------------------------------------------------

func TestAdminCleanResults_NotFound(t *testing.T) {
	gen := newBlockingGen()
	defer close(gen.ch)

	ms := &storage.MockStore{}
	jm := newAdminTestJobManager(t, gen, 2)
	// Project 99 does not exist in the store → GetProject returns ErrProjectNotFound → 404.
	projStore := &testutil.MockProjectStore{
		GetProjectFn: func(_ context.Context, id int64) (*store.Project, error) {
			return nil, store.ErrProjectNotFound
		},
	}
	h := NewAdminHandlerWithProjects(jm, ms, projStore, zap.NewNop())

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/admin/results/99", nil)
	req.SetPathValue("project_id", "99")
	rr := httptest.NewRecorder()
	h.CleanProjectResults(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAdminCleanResults_Success(t *testing.T) {
	gen := newBlockingGen()
	defer close(gen.ch)

	cleaned := false
	ms := &storage.MockStore{
		CleanResultsFn: func(_ context.Context, _ string) error {
			cleaned = true
			return nil
		},
	}
	jm := newAdminTestJobManager(t, gen, 2)
	// Project 1 exists with slug "proj1".
	projStore := &testutil.MockProjectStore{
		GetProjectFn: func(_ context.Context, id int64) (*store.Project, error) {
			if id == 1 {
				return &store.Project{ID: 1, Slug: "proj1"}, nil
			}
			return nil, store.ErrProjectNotFound
		},
	}
	h := NewAdminHandlerWithProjects(jm, ms, projStore, zap.NewNop())

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/admin/results/1", nil)
	req.SetPathValue("project_id", "1")
	rr := httptest.NewRecorder()
	h.CleanProjectResults(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if !cleaned {
		t.Error("expected CleanResults to be called")
	}
}

// ---------------------------------------------------------------------------
// DeleteJob
// ---------------------------------------------------------------------------

func TestAdminDeleteJob_MissingID(t *testing.T) {
	gen := newBlockingGen()
	defer close(gen.ch)

	jm := newAdminTestJobManager(t, gen, 2)
	h := newTestAdminHandler(t, jm, &storage.MockStore{})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/admin/jobs/", nil)
	// job_id path value intentionally not set
	rr := httptest.NewRecorder()
	h.DeleteJob(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAdminDeleteJob_NotFound(t *testing.T) {
	gen := newBlockingGen()
	defer close(gen.ch)

	jm := newAdminTestJobManager(t, gen, 2)
	h := newTestAdminHandler(t, jm, &storage.MockStore{})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/admin/jobs/bogus-id", nil)
	req.SetPathValue("job_id", "bogus-id")
	rr := httptest.NewRecorder()
	h.DeleteJob(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAdminDeleteJob_NonTerminal(t *testing.T) {
	gen := newContextGen()
	jm := newAdminTestJobManager(t, gen, 2)
	j := jm.Submit(1, "proj-delete-running", runner.JobParams{})

	// Wait for the job to reach running state.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := jm.Get(j.ID); got != nil && got.Status == runner.JobStatusRunning {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	defer close(gen.ch)

	h := newTestAdminHandler(t, jm, &storage.MockStore{})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/admin/jobs/"+j.ID, nil)
	req.SetPathValue("job_id", j.ID)
	rr := httptest.NewRecorder()
	h.DeleteJob(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("want 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAdminDeleteJob_Success(t *testing.T) {
	gen := newBlockingGen()
	jm := newAdminTestJobManager(t, gen, 2)
	j := jm.Submit(1, "proj-delete-success", runner.JobParams{})
	close(gen.ch) // let the job complete

	// Wait for completion.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if got := jm.Get(j.ID); got != nil && got.Status == runner.JobStatusCompleted {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	h := newTestAdminHandler(t, jm, &storage.MockStore{})

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/admin/jobs/"+j.ID, nil)
	req.SetPathValue("job_id", j.ID)
	rr := httptest.NewRecorder()
	h.DeleteJob(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Job should no longer be retrievable.
	if got := jm.Get(j.ID); got != nil {
		t.Errorf("expected job to be deleted, but Get returned %+v", got)
	}
}

// ---------------------------------------------------------------------------
// ListPendingResults — storage_key resolution tests
// ---------------------------------------------------------------------------

// TestAdminListPendingResults_TopLevelProject verifies that a top-level project
// whose storage dir matches its slug is resolved correctly (project_id != 0, slug populated).
func TestAdminListPendingResults_TopLevelProject(t *testing.T) {
	gen := newBlockingGen()
	defer close(gen.ch)

	modTime := time.Now().UnixNano()
	ms := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{"my-top"}, nil
		},
		ReadDirFn: func(_ context.Context, _ string, _ string) ([]storage.DirEntry, error) {
			return []storage.DirEntry{
				{Name: "result-001.json", Size: 512, ModTime: modTime},
			}, nil
		},
	}
	jm := newAdminTestJobManager(t, gen, 2)
	projStore := &testutil.MockProjectStore{
		GetProjectFn: func(_ context.Context, _ int64) (*store.Project, error) {
			// "my-top" is not numeric — GetProject won't be called for it.
			return nil, store.ErrProjectNotFound
		},
		GetProjectBySlugFn: func(_ context.Context, slug string) (*store.Project, error) {
			if slug == "my-top" {
				return &store.Project{ID: 5, Slug: "my-top", StorageKey: "my-top"}, nil
			}
			return nil, store.ErrProjectNotFound
		},
	}
	h := NewAdminHandlerWithProjects(jm, ms, projStore, zap.NewNop())

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/admin/results", nil)
	rr := httptest.NewRecorder()
	h.ListPendingResults(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, ok := resp["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("expected 1 entry, got %v", resp["data"])
	}
	entry, ok := data[0].(map[string]any)
	if !ok {
		t.Fatalf("expected entry to be object")
	}
	if entry["project_id"].(float64) != 5 {
		t.Errorf("want project_id=5, got %v", entry["project_id"])
	}
	if entry["slug"] != "my-top" {
		t.Errorf("want slug=%q, got %v", "my-top", entry["slug"])
	}
	if entry["storage_key"] != "my-top" {
		t.Errorf("want storage_key=%q, got %v", "my-top", entry["storage_key"])
	}
}

// TestAdminListPendingResults_ChildProject verifies that a child project whose
// storage dir is its numeric storage_key (e.g. "82") is resolved to the correct
// project row (project_id=82, slug="ui-permissions").
func TestAdminListPendingResults_ChildProject(t *testing.T) {
	gen := newBlockingGen()
	defer close(gen.ch)

	modTime := time.Now().UnixNano()
	ms := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{"82"}, nil
		},
		ReadDirFn: func(_ context.Context, _ string, _ string) ([]storage.DirEntry, error) {
			return []storage.DirEntry{
				{Name: "result-001.json", Size: 256, ModTime: modTime},
			}, nil
		},
	}
	jm := newAdminTestJobManager(t, gen, 2)
	projStore := &testutil.MockProjectStore{
		GetProjectFn: func(_ context.Context, id int64) (*store.Project, error) {
			if id == 82 {
				return &store.Project{ID: 82, Slug: "ui-permissions", StorageKey: "82"}, nil
			}
			return nil, store.ErrProjectNotFound
		},
	}
	h := NewAdminHandlerWithProjects(jm, ms, projStore, zap.NewNop())

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/admin/results", nil)
	rr := httptest.NewRecorder()
	h.ListPendingResults(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, ok := resp["data"].([]any)
	if !ok || len(data) != 1 {
		t.Fatalf("expected 1 entry, got %v", resp["data"])
	}
	entry, ok := data[0].(map[string]any)
	if !ok {
		t.Fatalf("expected entry to be object")
	}
	if entry["project_id"].(float64) != 82 {
		t.Errorf("want project_id=82, got %v", entry["project_id"])
	}
	if entry["slug"] != "ui-permissions" {
		t.Errorf("want slug=%q, got %v", "ui-permissions", entry["slug"])
	}
	if entry["storage_key"] != "82" {
		t.Errorf("want storage_key=%q, got %v", "82", entry["storage_key"])
	}
}

// TestAdminListPendingResults_OrphanSkipped verifies that a storage dir with no
// matching DB row is silently skipped (not included in the response).
func TestAdminListPendingResults_OrphanSkipped(t *testing.T) {
	gen := newBlockingGen()
	defer close(gen.ch)

	modTime := time.Now().UnixNano()
	ms := &storage.MockStore{
		ListProjectsFn: func(_ context.Context) ([]string, error) {
			return []string{"99"}, nil
		},
		ReadDirFn: func(_ context.Context, _ string, _ string) ([]storage.DirEntry, error) {
			return []storage.DirEntry{
				{Name: "result-001.json", Size: 128, ModTime: modTime},
			}, nil
		},
	}
	jm := newAdminTestJobManager(t, gen, 2)
	projStore := &testutil.MockProjectStore{
		GetProjectFn: func(_ context.Context, _ int64) (*store.Project, error) {
			return nil, store.ErrProjectNotFound
		},
		GetProjectBySlugFn: func(_ context.Context, _ string) (*store.Project, error) {
			return nil, store.ErrProjectNotFound
		},
	}
	h := NewAdminHandlerWithProjects(jm, ms, projStore, zap.NewNop())

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/admin/results", nil)
	rr := httptest.NewRecorder()
	h.ListPendingResults(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, ok := resp["data"].([]any)
	if !ok {
		t.Fatalf("expected data array, got %T", resp["data"])
	}
	if len(data) != 0 {
		t.Errorf("expected orphan entry to be skipped, got %d entries", len(data))
	}
}
