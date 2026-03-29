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
	return NewAdminHandler(jm, ms, t.TempDir(), zap.NewNop())
}

// blockingGen blocks until ch is closed (for keeping jobs in pending/running state).
type blockingGen struct {
	ch chan struct{}
}

func newBlockingGen() *blockingGen {
	return &blockingGen{ch: make(chan struct{})}
}

func (g *blockingGen) GenerateReport(_ context.Context, _, _, _, _ string, _ bool, _, _ string) (string, error) {
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

func (g *contextGen) GenerateReport(ctx context.Context, _, _, _, _ string, _ bool, _, _ string) (string, error) {
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
	jm.Submit("proj-admin-1", runner.JobParams{})
	jm.Submit("proj-admin-2", runner.JobParams{})

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
	if entry["project_id"] != "proj-with-results" {
		t.Errorf("wrong project_id: %v", entry["project_id"])
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
	j := jm.Submit("proj-terminal", runner.JobParams{})
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
	j := jm.Submit("proj-cancel-handler", runner.JobParams{})

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

	ms := &storage.MockStore{
		ProjectExistsFn: func(_ context.Context, _ string) (bool, error) {
			return false, nil
		},
	}
	jm := newAdminTestJobManager(t, gen, 2)
	h := newTestAdminHandler(t, jm, ms)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/admin/results/missing-proj", nil)
	req.SetPathValue("project_id", "missing-proj")
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
		ProjectExistsFn: func(_ context.Context, _ string) (bool, error) {
			return true, nil
		},
		CleanResultsFn: func(_ context.Context, _ string) error {
			cleaned = true
			return nil
		},
	}
	jm := newAdminTestJobManager(t, gen, 2)
	h := newTestAdminHandler(t, jm, ms)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/admin/results/proj-clean", nil)
	req.SetPathValue("project_id", "proj-clean")
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
	j := jm.Submit("proj-delete-running", runner.JobParams{})

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
	j := jm.Submit("proj-delete-success", runner.JobParams{})
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
