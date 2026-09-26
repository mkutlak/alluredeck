package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestInternalErrorsDoNotLeak (269fdc0): a store or storage failure answers 500
// without echoing the internal error text to the client.
func TestInternalErrorsDoNotLeak(t *testing.T) {
	t.Parallel()
	const leak = "pq: connection refused"
	failing := func(context.Context, string) error { return errors.New(leak) }
	allure := func(t *testing.T, st storage.Store) (*runner.Allure, *config.Config) {
		cfg := &config.Config{ProjectsPath: t.TempDir(), MaxUploadSizeMB: 100}
		mocks := testutil.New()
		return runner.NewAllure(runner.AllureDeps{Config: cfg, Store: st, BuildStore: mocks.MemBuilds, Locker: mocks.Locker, Logger: zap.NewNop()}), cfg
	}
	projectHandler := func(t *testing.T, ps store.ProjectStorer, st storage.Store) *ProjectHandler {
		r, cfg := allure(t, st)
		return NewProjectHandler(ps, r, st, cfg, zap.NewNop())
	}
	rows := []struct {
		name  string
		serve func(t *testing.T, rr *httptest.ResponseRecorder)
	}{
		{"list projects", func(t *testing.T, rr *httptest.ResponseRecorder) {
			ps := &testutil.MockProjectStore{ListProjectsPaginatedFn: func(context.Context, int, int) ([]store.Project, int, error) {
				return nil, 0, errors.New(leak)
			}}
			projectHandler(t, ps, storage.NewLocalStore(&config.Config{ProjectsPath: t.TempDir()})).
				GetProjects(rr, httptest.NewRequest(http.MethodGet, "/api/v1/projects", nil))
		}},
		{"create project", func(t *testing.T, rr *httptest.ResponseRecorder) {
			req := httptest.NewRequest(http.MethodPost, "/api/v1/projects", strings.NewReader(`{"id":"newproject"}`))
			req.Header.Set("Content-Type", "application/json")
			projectHandler(t, testutil.NewMemProjectStore(), &testutil.MockStorage{CreateProjectFn: failing}).CreateProject(rr, req)
		}},
		{"delete project", func(t *testing.T, rr *httptest.ResponseRecorder) {
			ps := testutil.NewMemProjectStore()
			p, _ := ps.CreateProject(context.Background(), "proj1")
			req := httptest.NewRequest(http.MethodDelete, "/api/v1/projects/1", nil)
			req.SetPathValue("project_id", strconv.FormatInt(p.ID, 10))
			projectHandler(t, ps, &testutil.MockStorage{DeleteProjectFn: failing}).DeleteProject(rr, req)
		}},
		{"report history", func(t *testing.T, rr *httptest.ResponseRecorder) {
			mocks := testutil.New()
			mocks.Builds.ListBuildsPaginatedBranchFn = func(context.Context, int64, int, int, *int64) ([]store.Build, int, error) {
				return nil, 0, errors.New(leak)
			}
			p, _ := mocks.Projects.CreateProject(context.Background(), "proj1")
			req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/1/reports", nil)
			req.SetPathValue("project_id", strconv.FormatInt(p.ID, 10))
			newTestReportHandlerWithMocks(t, t.TempDir(), mocks).GetReportHistory(rr, req)
		}},
		// force_project_creation makes the upload create the project in storage.
		{"send results", func(t *testing.T, rr *httptest.ResponseRecorder) {
			st := &testutil.MockStorage{CreateProjectFn: failing}
			r, cfg := allure(t, st)
			h := NewResultUploadHandler(st, testutil.NewMemProjectStore(), runner.NewMemJobManager(nil, 0, nil), r, cfg, zap.NewNop())
			req := httptest.NewRequest(http.MethodPost, "/api/v1/projects/proj1/results?force_project_creation=true", strings.NewReader(`{"results":[]}`))
			req.Header.Set("Content-Type", "application/json")
			req.SetPathValue("project_id", "proj1")
			h.SendResults(rr, req)
		}},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rr := httptest.NewRecorder()
			tc.serve(t, rr)
			if rr.Code != http.StatusInternalServerError {
				t.Fatalf("status = %d, want 500: %s", rr.Code, rr.Body.String())
			}
			if strings.Contains(rr.Body.String(), "connection refused") {
				t.Errorf("response leaks the internal error: %s", rr.Body.String())
			}
		})
	}
}
