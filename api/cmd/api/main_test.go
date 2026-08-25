package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/handlers"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

func TestRegisterRoutes(t *testing.T) {
	cfg := &config.Config{SecurityEnabled: true, JWTSecret: "test-secret"}

	mocks := testutil.New()

	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())
	localStore := storage.NewLocalStore(cfg)
	allureCore := runner.NewAllure(runner.AllureDeps{
		Config:     cfg,
		Store:      localStore,
		BuildStore: mocks.Builds,
		Locker:     mocks.Locker,
		Logger:     zap.NewNop(),
	})

	loginLimiter := middleware.NewIPRateLimiter(5, 10, 15*time.Minute, false)

	mux := http.NewServeMux()
	registerRoutes(routeDeps{
		mux:          mux,
		prefix:       "/api/v1",
		cfg:          cfg,
		jwtManager:   jwtManager,
		loginLimiter: loginLimiter,
		apiKeyStore:  mocks.APIKeys,
		h: handlerSet{
			system:       handlers.NewSystemHandler(cfg, nil, nil, nil, nil),
			auth:         handlers.NewAuthHandler(cfg, jwtManager, nil),
			project:      handlers.NewProjectHandler(mocks.Projects, allureCore, localStore, cfg, zap.NewNop()),
			resultUpload: handlers.NewResultUploadHandler(localStore, mocks.Projects, runner.NewMemJobManager(nil, 0, zap.NewNop()), allureCore, cfg, zap.NewNop()),
			admin:        handlers.NewAdminHandler(nil, nil, zap.NewNop()),
		},
	})

	tests := []struct {
		method string
		path   string
	}{
		{"GET", "/api/v1/version"},
		{"GET", "/api/v1/config"},
		{"POST", "/api/v1/login"},
		{"DELETE", "/api/v1/projects/testproj/reports/history"},
		{"DELETE", "/api/v1/projects/testproj/results"},
		{"POST", "/api/v1/projects/testproj/reports"},
		{"POST", "/api/v1/projects/testproj/results"},
		{"GET", "/api/v1/projects/testproj/reports"},
		{"DELETE", "/api/v1/projects/testproj/reports/42"},
	}

	for _, tc := range tests {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			// Login with security enabled will return 400 (Bad Request) because of missing body,
			// which is fine as it confirms the route is registered and reached the handler.
			if rr.Code == http.StatusNotFound {
				t.Errorf("Path %s %s not registered. Response: %d", tc.method, tc.path, rr.Code)
			}
		})
	}
}

func TestBareRoutes_Return404(t *testing.T) {
	cfg := &config.Config{SecurityEnabled: false, JWTSecret: "test-secret"}

	mocks := testutil.New()

	jwtManager := security.NewJWTManager(cfg, mocks.Blacklist, zap.NewNop())
	localStore := storage.NewLocalStore(cfg)
	allureCore := runner.NewAllure(runner.AllureDeps{
		Config:     cfg,
		Store:      localStore,
		BuildStore: mocks.Builds,
		Locker:     mocks.Locker,
		Logger:     zap.NewNop(),
	})

	loginLimiter := middleware.NewIPRateLimiter(5, 10, 15*time.Minute, false)

	mux := http.NewServeMux()
	registerRoutes(routeDeps{
		mux:          mux,
		prefix:       "/api/v1",
		cfg:          cfg,
		jwtManager:   jwtManager,
		loginLimiter: loginLimiter,
		apiKeyStore:  mocks.APIKeys,
		h: handlerSet{
			system:       handlers.NewSystemHandler(cfg, nil, nil, nil, nil),
			auth:         handlers.NewAuthHandler(cfg, jwtManager, nil),
			project:      handlers.NewProjectHandler(mocks.Projects, allureCore, localStore, cfg, zap.NewNop()),
			resultUpload: handlers.NewResultUploadHandler(localStore, mocks.Projects, runner.NewMemJobManager(nil, 0, zap.NewNop()), allureCore, cfg, zap.NewNop()),
			admin:        handlers.NewAdminHandler(nil, nil, zap.NewNop()),
		},
	})

	// Bare routes (no /api/v1 prefix) should return 404.
	bareRoutes := []struct {
		method string
		path   string
	}{
		{"GET", "/version"},
		{"GET", "/config"},
		{"POST", "/login"},
	}

	for _, tc := range bareRoutes {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			if rr.Code != http.StatusNotFound {
				t.Errorf("Bare route %s %s should return 404, got %d", tc.method, tc.path, rr.Code)
			}
		})
	}
}

// TestRunRetentionSweep_UsesStorageKey runs a single retention sweep over a
// child project whose StorageKey (numeric) differs from its Slug and asserts
// every storage prune call addresses the StorageKey. Pruning by slug is the
// regression under test: child projects store under their numeric key, so a
// slug-addressed prune silently deletes nothing and leaks report data forever.
// It also pins the batched-GC contract — build_orders returned by
// PruneStaleBranches alongside a per-branch error must still be pruned from
// storage — and that orphan branch rows are cleaned on every sweep.
func TestRunRetentionSweep_UsesStorageKey(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	projects := testutil.NewMemProjectStore()
	parent, err := projects.CreateProject(ctx, "parent")
	if err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	child, err := projects.CreateProjectWithParent(ctx, "child", parent.ID)
	if err != nil {
		t.Fatalf("CreateProjectWithParent: %v", err)
	}
	if child.StorageKey == child.Slug {
		t.Fatalf("test setup: child StorageKey %q must differ from slug %q", child.StorageKey, child.Slug)
	}

	var prunedKeys []string
	var prunedOrders [][]int
	dataStore := &storage.MockStore{
		PruneReportDirsFn: func(_ context.Context, projectID string, buildNumbers []int) error {
			prunedKeys = append(prunedKeys, projectID)
			prunedOrders = append(prunedOrders, buildNumbers)
			return nil
		},
	}

	orphansDeleted := 0
	builds := &testutil.MockBuildStore{
		PruneBuildsBranchFn: func(_ context.Context, _ int64, _ int, _ *int64) ([]int, error) {
			return []int{1}, nil
		},
		PruneBuildsByAgeFn: func(_ context.Context, _ int64, _ time.Time) ([]int, error) {
			return []int{2}, nil
		},
		// Partial success: one branch failed, but build 3 was deleted — its
		// report dir must still be pruned from storage.
		PruneStaleBranchesFn: func(_ context.Context, _ int64, _ time.Time) ([]int, error) {
			return []int{3}, errors.New("branch \"stuck\": lock timeout")
		},
		DeleteOrphanBranchesFn: func(_ context.Context, _ int64) (int64, error) {
			orphansDeleted++
			return 2, nil
		},
	}

	cfg := &config.Config{KeepHistoryLatest: 10, KeepHistoryMaxAgeDays: 3}
	runRetentionSweep(ctx, cfg, projects, builds, dataStore, testutil.NewMemWebhookStore(), logger)

	// Two projects × three prune paths each (count, age, stale-partial).
	if len(prunedKeys) != 6 {
		t.Fatalf("expected 6 PruneReportDirs calls, got %d (%v)", len(prunedKeys), prunedKeys)
	}
	for i, key := range prunedKeys {
		if key == child.Slug {
			t.Errorf("call %d: PruneReportDirs addressed by slug %q — must use StorageKey", i, key)
		}
		if key != parent.StorageKey && key != child.StorageKey {
			t.Errorf("call %d: unexpected storage key %q", i, key)
		}
	}
	// The stale-branch orders returned alongside the error were still pruned.
	staleSeen := false
	for _, orders := range prunedOrders {
		if len(orders) == 1 && orders[0] == 3 {
			staleSeen = true
		}
	}
	if !staleSeen {
		t.Error("stale-branch build 3 was never pruned from storage despite being deleted from the database")
	}
	if orphansDeleted != 2 {
		t.Errorf("expected DeleteOrphanBranches once per project (2), got %d", orphansDeleted)
	}
}
