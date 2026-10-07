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
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestRegisterRoutes_PipelineRunListsAreNeverCached: a run in the feed carries
// aggregate.awaiting_suites, which depends on the server clock (a run is
// awaiting only while its newest upload is under 30 minutes old), so a cached
// page keeps saying "awaiting" after the window ends, or the reverse.
func TestRegisterRoutes_PipelineRunListsAreNeverCached(t *testing.T) {
	ctx := context.Background()
	projects := testutil.NewMemProjectStore()
	parent, err := projects.CreateProject(ctx, "parent")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := projects.CreateProjectWithParent(ctx, "child", parent.ID); err != nil {
		t.Fatal(err)
	}
	rows := []store.PipelineRunRow{{PipelineID: "196765", CommitSHA: "abc", CreatedAt: time.Now(), ProjectID: 2, Slug: "child", GroupProjectID: parent.ID}}
	ps := &testutil.MockPipelineStore{
		ListPipelineRunsFn: func(context.Context, int64, string, int, int) ([]store.PipelineRunRow, int, error) {
			return rows, 1, nil
		},
		ListAllPipelineRunsFn: func(context.Context, string, []int64, int, int) ([]store.PipelineRunRow, int, error) {
			return rows, 1, nil
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

	for _, path := range []string{"/api/v1/projects/1/pipeline-runs", "/api/v1/pipeline-runs"} {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("GET %s: status = %d, want 200", path, rr.Code)
		}
		if got := rr.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("GET %s: Cache-Control = %q, want no-store", path, got)
		}
	}
}
