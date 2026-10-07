package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/handlers"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestRegisterRoutes_PipelineRunRouting pins that the single-run route and the
// run-failures route, which extends it by one segment, coexist on the real
// mux and each reaches its own handler with the run key unescaped.
func TestRegisterRoutes_PipelineRunRouting(t *testing.T) {
	ctx := context.Background()
	projects := testutil.NewMemProjectStore()
	if _, err := projects.CreateProject(ctx, "parent"); err != nil {
		t.Fatal(err)
	}
	var calls []string
	ps := &testutil.MockPipelineStore{
		GetPipelineRunFn: func(_ context.Context, _ int64, runKey string) ([]store.PipelineRunRow, error) {
			calls = append(calls, "run:"+runKey)
			return []store.PipelineRunRow{{PipelineID: runKey, CommitSHA: "abc", CreatedAt: time.Now(), ProjectID: 2, Slug: "child", GroupProjectID: 1}}, nil
		},
		ListRunFailuresFn: func(_ context.Context, _ int64, runKey string, _ int) ([]store.RunFailureRow, error) {
			calls = append(calls, "failures:"+runKey)
			return nil, nil
		},
	}
	hs := allHandlers(t)
	hs.pipeline = handlers.NewPipelineHandler(ps, projects, testutil.NewMemKnownIssueStore(), t.TempDir(), zap.NewNop())

	mux := http.NewServeMux()
	registerRoutes(routeDeps{
		mux:          mux,
		prefix:       "/api/v1",
		cfg:          &config.Config{MakeViewerEndpointsPublic: true},
		loginLimiter: middleware.NewIPRateLimiter(1000, 1000, time.Minute, false),
		h:            hs,
	})

	for _, tc := range []struct{ path, want string }{
		{"/api/v1/projects/1/pipeline-runs/196765", "run:196765"},
		{"/api/v1/projects/1/pipeline-runs/196765/failures", "failures:196765"},
		{"/api/v1/projects/parent/pipeline-runs/196765", "run:196765"},
		// The UI encodes the key with encodeURIComponent; a key with a slash in
		// it must stay one segment.
		{"/api/v1/projects/1/pipeline-runs/feat%2Fx", "run:feat/x"},
	} {
		calls = nil
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, tc.path, nil))
		if rr.Code != http.StatusOK || len(calls) != 1 || calls[0] != tc.want {
			t.Errorf("GET %s = %d, store calls %v; want 200 and [%s]", tc.path, rr.Code, calls, tc.want)
		}
	}
}

// pipelineRunMux mounts the real routes over a parent group (id 1) whose store
// knows exactly one run, "196765"; every other key is unknown to it.
func pipelineRunMux(t *testing.T, cfg *config.Config, jwtManager *security.JWTManager) *http.ServeMux {
	t.Helper()
	projects := testutil.NewMemProjectStore()
	if _, err := projects.CreateProject(context.Background(), "parent"); err != nil {
		t.Fatal(err)
	}
	ps := &testutil.MockPipelineStore{
		GetPipelineRunFn: func(_ context.Context, _ int64, runKey string) ([]store.PipelineRunRow, error) {
			if runKey != "196765" {
				return nil, nil
			}
			return []store.PipelineRunRow{{PipelineID: runKey, CommitSHA: "abc", CreatedAt: time.Now(), ProjectID: 2, Slug: "child", GroupProjectID: 1}}, nil
		},
	}
	hs := allHandlers(t)
	hs.pipeline = handlers.NewPipelineHandler(ps, projects, testutil.NewMemKnownIssueStore(), t.TempDir(), zap.NewNop())
	mux := http.NewServeMux()
	registerRoutes(routeDeps{
		mux:          mux,
		prefix:       "/api/v1",
		cfg:          cfg,
		jwtManager:   jwtManager,
		loginLimiter: middleware.NewIPRateLimiter(1000, 1000, time.Minute, false),
		h:            hs,
	})
	return mux
}

// TestRegisterRoutes_PipelineRunIsNeverCached: a run changes while its shards
// upload and a link can be opened before the first upload, so a cached 200 is a
// partial run and a cached 404 outlives the upload. no-store covers every
// status the handler answers with.
func TestRegisterRoutes_PipelineRunIsNeverCached(t *testing.T) {
	mux := pipelineRunMux(t, &config.Config{MakeViewerEndpointsPublic: true}, nil)
	for _, tc := range []struct {
		key  string
		want int
	}{{"196765", http.StatusOK}, {"unknown", http.StatusNotFound}} {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/projects/1/pipeline-runs/"+tc.key, nil))
		if rr.Code != tc.want {
			t.Fatalf("run %s: status = %d, want %d", tc.key, rr.Code, tc.want)
		}
		if got := rr.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("run %s (%d): Cache-Control = %q, want no-store", tc.key, rr.Code, got)
		}
	}
}

// TestRegisterRoutes_PipelineRunRequiresViewer: with viewer endpoints not
// public the route sits behind auth and the viewer role, like its siblings.
func TestRegisterRoutes_PipelineRunRequiresViewer(t *testing.T) {
	cfg := &config.Config{
		SecurityEnabled:   true,
		JWTSecret:         "test-secret",
		AccessTokenExpiry: config.DurationSeconds(15 * time.Minute),
	}
	jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
	mux := pipelineRunMux(t, cfg, jwtManager)

	token := func(sub, role string) string {
		tok, _, err := jwtManager.GenerateTokens(sub, role)
		if err != nil {
			t.Fatalf("GenerateTokens(%s): %v", role, err)
		}
		return tok
	}
	for _, tc := range []struct {
		name  string
		token string
		want  int
	}{
		{"anonymous", "", http.StatusUnauthorized},
		{"unknown role", token("1001", "guest"), http.StatusForbidden},
		{"viewer", token("1002", "viewer"), http.StatusOK},
	} {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/1/pipeline-runs/196765", nil)
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, req)
		if rr.Code != tc.want {
			t.Errorf("%s: status = %d, want %d", tc.name, rr.Code, tc.want)
		}
	}
}
