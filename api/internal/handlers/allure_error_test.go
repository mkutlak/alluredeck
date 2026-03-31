package handlers

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// assertNoLeakage checks the response has the expected status and that the body
// does not contain the injected internal error string.
func assertNoLeakage(t *testing.T, rr *httptest.ResponseRecorder, wantStatus int, leakString string) {
	t.Helper()
	if rr.Code != wantStatus {
		t.Fatalf("want status %d, got %d: %s", wantStatus, rr.Code, rr.Body.String())
	}
	if strings.Contains(rr.Body.String(), leakString) {
		t.Errorf("response body leaks internal error detail %q; body: %s", leakString, rr.Body.String())
	}
}

// TestListProjects_StoreError_NoLeakage verifies that a DB error from
// ListProjectsPaginated returns 500 without leaking the internal error string.
func TestListProjects_StoreError_NoLeakage(t *testing.T) {
	projectsDir := t.TempDir()
	mocks := testutil.New()

	cfg := &config.Config{ProjectsPath: projectsDir}
	st := storage.NewLocalStore(cfg)
	logger := zap.NewNop()

	mockProj := &testutil.MockProjectStore{
		ListProjectsPaginatedFn: func(_ context.Context, _, _ int) ([]store.Project, int, error) {
			return nil, 0, fmt.Errorf("pq: connection refused")
		},
	}

	r := runner.NewAllure(runner.AllureDeps{
		Config:     cfg,
		Store:      st,
		BuildStore: mocks.MemBuilds,
		Locker:     mocks.Locker,
		Logger:     logger,
	})
	h := NewProjectHandler(mockProj, r, st, cfg, logger)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/projects", nil)
	rr := httptest.NewRecorder()
	h.GetProjects(rr, req)

	assertNoLeakage(t, rr, http.StatusInternalServerError, "connection refused")
}

// TestCreateProject_RunnerError_NoLeakage verifies that a non-ErrProjectExists
// runner error returns 500 without leaking the internal error string.
func TestCreateProject_RunnerError_NoLeakage(t *testing.T) {
	projectsDir := t.TempDir()
	mocks := testutil.New()

	cfg := &config.Config{ProjectsPath: projectsDir}
	mockSt := &storage.MockStore{
		CreateProjectFn: func(_ context.Context, _ string) error {
			return fmt.Errorf("pq: connection refused")
		},
	}
	logger := zap.NewNop()
	r := runner.NewAllure(runner.AllureDeps{
		Config:     cfg,
		Store:      mockSt,
		BuildStore: mocks.MemBuilds,
		Locker:     mocks.Locker,
		Logger:     logger,
	})
	h := NewProjectHandler(mocks.Projects, r, mockSt, cfg, logger)

	body := strings.NewReader(`{"id":"newproject"}`)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, "/api/v1/projects", body)
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.CreateProject(rr, req)

	assertNoLeakage(t, rr, http.StatusInternalServerError, "connection refused")
}

// TestGetReportHistory_StoreError_NoLeakage verifies that a DB error from
// ListBuildsPaginatedBranch returns 500 without leaking the internal error string.
func TestGetReportHistory_StoreError_NoLeakage(t *testing.T) {
	projectsDir := t.TempDir()
	mocks := testutil.New()
	mocks.Builds.ListBuildsPaginatedBranchFn = func(_ context.Context, _ string, _, _ int, _ *int64) ([]store.Build, int, error) {
		return nil, 0, fmt.Errorf("pq: connection refused")
	}

	h := newTestReportHandlerWithMocks(t, projectsDir, mocks)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/reports", nil)
	req.SetPathValue("project_id", "proj1")
	rr := httptest.NewRecorder()
	h.GetReportHistory(rr, req)

	assertNoLeakage(t, rr, http.StatusInternalServerError, "connection refused")
}

// TestDeleteProject_RunnerError_NoLeakage verifies that a non-ErrProjectNotFound
// runner error from DeleteProject returns 500 without leaking the internal error string.
func TestDeleteProject_RunnerError_NoLeakage(t *testing.T) {
	projectsDir := t.TempDir()
	mocks := testutil.New()

	cfg := &config.Config{ProjectsPath: projectsDir}
	mockSt := &storage.MockStore{
		DeleteProjectFn: func(_ context.Context, _ string) error {
			return fmt.Errorf("pq: connection refused")
		},
	}
	logger := zap.NewNop()
	r := runner.NewAllure(runner.AllureDeps{
		Config:     cfg,
		Store:      mockSt,
		BuildStore: mocks.MemBuilds,
		Locker:     mocks.Locker,
		Logger:     logger,
	})
	h := NewProjectHandler(mocks.Projects, r, mockSt, cfg, logger)

	req, _ := http.NewRequestWithContext(context.Background(), http.MethodDelete,
		"/api/v1/projects/proj1", nil)
	req.SetPathValue("project_id", "proj1")
	rr := httptest.NewRecorder()
	h.DeleteProject(rr, req)

	assertNoLeakage(t, rr, http.StatusInternalServerError, "connection refused")
}

// TestSendResults_ProjectCheckError_NoLeakage verifies that a storage error from
// ProjectExists returns 500 without leaking the internal error string.
func TestSendResults_ProjectCheckError_NoLeakage(t *testing.T) {
	projectsDir := t.TempDir()
	mocks := testutil.New()

	cfg := &config.Config{ProjectsPath: projectsDir}
	mockSt := &storage.MockStore{
		ProjectExistsFn: func(_ context.Context, _ string) (bool, error) {
			return false, fmt.Errorf("pq: connection refused")
		},
	}
	logger := zap.NewNop()
	r := runner.NewAllure(runner.AllureDeps{
		Config:     cfg,
		Store:      mockSt,
		BuildStore: mocks.MemBuilds,
		Locker:     mocks.Locker,
		Logger:     logger,
	})
	h := NewResultUploadHandler(mockSt, mocks.Projects, r, cfg, logger)

	body := strings.NewReader(`{"results":[]}`)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/projects/proj1/results", body)
	req.Header.Set("Content-Type", "application/json")
	req.SetPathValue("project_id", "proj1")
	rr := httptest.NewRecorder()
	h.SendResults(rr, req)

	assertNoLeakage(t, rr, http.StatusInternalServerError, "connection refused")
}
