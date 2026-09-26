package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/handlers"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// access is the least-privileged caller a route admits. Levels are ordered:
// a principal may call a route iff its level is at least the route's access.
type access int

const (
	accessPublic access = iota // no token required
	accessAuth                 // any valid token, whatever its role
	accessViewer
	accessEditor
	accessAdmin
)

func (a access) String() string {
	return [...]string{"public", "any authenticated caller", "viewer", "editor", "admin"}[a]
}

// publicRoutes mount no auth middleware: login and refresh establish a
// session, version/config are read before login, and the signed attachment
// download is authenticated by the HMAC in its own URL.
var publicRoutes = map[string]bool{
	"GET /api/v1/version":            true,
	"GET /api/v1/config":             true,
	"POST /api/v1/login":             true,
	"POST /api/v1/auth/refresh":      true,
	"GET /api/v1/auth/oidc/login":    true,
	"GET /api/v1/auth/oidc/callback": true,
	"GET /attachments/{id}":          true,
}

// selfServiceRoutes act only on the caller's own session, preferences, API
// keys or profile, so any authenticated caller may use them whatever its role.
// They are the writes exempt from the "writes need editor" default.
var selfServiceRoutes = map[string]bool{
	"DELETE /api/v1/logout":          true,
	"GET /api/v1/auth/session":       true,
	"GET /api/v1/preferences":        true,
	"PUT /api/v1/preferences":        true,
	"GET /api/v1/api-keys":           true,
	"POST /api/v1/api-keys":          true,
	"DELETE /api/v1/api-keys/{id}":   true,
	"GET /api/v1/users/me":           true,
	"PATCH /api/v1/users/me":         true,
	"POST /api/v1/users/me/password": true,
}

// adminProjectRoutes are the project routes stricter than their method
// default: creating, renaming, re-parenting and deleting projects, destroying
// history, results, reports or branches, polling job status, and applying
// MCP proposals.
var adminProjectRoutes = map[string]bool{
	"POST /api/v1/projects":                                      true,
	"DELETE /api/v1/projects/{project_id}":                       true,
	"PUT /api/v1/projects/{project_id}/rename":                   true,
	"PUT /api/v1/projects/{project_id}/parent":                   true,
	"DELETE /api/v1/projects/{project_id}/parent":                true,
	"GET /api/v1/projects/{project_id}/jobs/{job_id}":            true,
	"DELETE /api/v1/projects/{project_id}/reports/history/group": true,
	"DELETE /api/v1/projects/{project_id}/reports/history":       true,
	"DELETE /api/v1/projects/{project_id}/reports/{report_id}":   true,
	"DELETE /api/v1/projects/{project_id}/results":               true,
	"DELETE /api/v1/projects/{project_id}/branches/{branch_id}":  true,
	"POST /api/v1/proposals/{type}/{id}/approve":                 true,
	"POST /api/v1/proposals/{type}/{id}/reject":                  true,
}

// ciIngestionRoutes are called by CI pipelines outside this repo to upload
// results, generate reports and read report history. The matrix only probes
// routes that are mounted, so these are pinned to exist.
var ciIngestionRoutes = map[string]bool{
	"POST /api/v1/projects/{project_id}/results": true,
	"POST /api/v1/projects/{project_id}/reports": true,
	"GET /api/v1/projects/{project_id}/reports":  true,
}

// requiredAccess derives a route's access from the RBAC invariants rather
// than a per-route copy of main.go: the explicit public and self-service
// lists; user management and the admin monitor are admin-only; webhook
// configuration is editor-only, reads included; the explicit admin project
// routes; otherwise reads need viewer and writes need editor.
func requiredAccess(method, path string) access {
	pattern := method + " " + path
	switch {
	case publicRoutes[pattern]:
		return accessPublic
	case selfServiceRoutes[pattern]:
		return accessAuth
	case strings.HasPrefix(path, "/api/v1/users"), strings.HasPrefix(path, "/api/v1/admin/"):
		return accessAdmin
	case strings.Contains(path, "/webhooks"):
		return accessEditor
	case adminProjectRoutes[pattern]:
		return accessAdmin
	case method == http.MethodGet:
		return accessViewer
	default:
		return accessEditor
	}
}

// recordingMux records every pattern registerRoutes mounts while serving
// through a real ServeMux, so the matrix covers exactly the mounted routes.
type recordingMux struct {
	*http.ServeMux
	patterns []string
}

func (m *recordingMux) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	m.patterns = append(m.patterns, pattern)
	m.ServeMux.HandleFunc(pattern, handler)
}

// allHandlers returns a handlerSet whose every field is a zero-value handler,
// so each conditionally mounted route group (branches, users, OIDC, MCP
// proposals, webhooks, ...) is registered. The handlers have nil
// dependencies: past the RBAC wrappers they fail validation or panic.
func allHandlers(t *testing.T) handlerSet {
	t.Helper()
	hs := handlerSet{
		system:          &handlers.SystemHandler{},
		auth:            &handlers.AuthHandler{},
		report:          &handlers.ReportHandler{},
		project:         &handlers.ProjectHandler{},
		resultUpload:    &handlers.ResultUploadHandler{},
		playwright:      &handlers.PlaywrightHandler{},
		admin:           &handlers.AdminHandler{},
		branch:          &handlers.BranchHandler{},
		testHistory:     &handlers.TestHistoryHandler{},
		analytics:       &handlers.AnalyticsHandler{},
		search:          &handlers.SearchHandler{},
		compare:         &handlers.CompareHandler{},
		dashboard:       &handlers.DashboardHandler{},
		lowPerf:         &handlers.LowPerformingHandler{},
		flakyImpact:     &handlers.FlakyImpactHandler{},
		projectTimeline: &handlers.ProjectTimelineHandler{},
		knownIssue:      &handlers.KnownIssueHandler{},
		attachment:      &handlers.AttachmentHandler{},
		attachmentDL:    &handlers.AttachmentDownloadHandler{},
		apiKey:          &handlers.APIKeyHandler{},
		user:            &handlers.UserHandler{},
		parent:          &handlers.ProjectParentHandler{},
		defect:          &handlers.DefectHandler{},
		buildTests:      &handlers.BuildTestsHandler{},
		webhook:         &handlers.WebhookHandler{},
		pipeline:        &handlers.PipelineHandler{},
		preferences:     &handlers.PreferenceHandler{},
		oidc:            &handlers.OIDCHandler{},
		proposals:       &handlers.ProposalsHandler{},
		failureSummary:  &handlers.FailureSummaryHandler{},
	}
	v := reflect.ValueOf(hs)
	for i := range v.NumField() {
		if v.Field(i).IsNil() {
			t.Fatalf("handlerSet.%s is unset: add it to allHandlers so its routes are covered", v.Type().Field(i).Name)
		}
	}
	return hs
}

// serve runs req through h and returns the status. A panic counts as having
// reached the handler and maps to the 500 the Recovery middleware would send.
func serve(h http.Handler, req *http.Request) (code int) {
	defer func() {
		if recover() != nil {
			code = http.StatusInternalServerError
		}
	}()
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr.Code
}

// TestRegisterRoutes_RBAC drives every route registerRoutes mounts through the
// real router with security enabled, once per principal. RBAC is enforced only
// by the wrappers chosen in registerRoutes — handlers never re-check roles —
// so this is the single place route authorization is pinned: a caller below a
// route's access must be rejected (401 without a token, 403 with one) and a
// caller at or above it must get through to the handler.
func TestRegisterRoutes_RBAC(t *testing.T) {
	// A valid token carrying an unrecognised role separates auth-only routes
	// from viewer routes. Subs are numeric like DB-backed users: handlers
	// refuse profile and password writes for env accounts' non-numeric subs.
	principals := []struct {
		name   string
		role   string // empty = anonymous, no token
		level  access
		denied int
	}{
		{"anonymous", "", accessPublic, http.StatusUnauthorized},
		{"unknown-role", "guest", accessAuth, http.StatusForbidden},
		{"viewer", "viewer", accessViewer, http.StatusForbidden},
		{"editor", "editor", accessEditor, http.StatusForbidden},
		{"admin", "admin", accessAdmin, 0},
	}

	for _, viewerPublic := range []bool{false, true} {
		t.Run(fmt.Sprintf("MakeViewerEndpointsPublic=%t", viewerPublic), func(t *testing.T) {
			cfg := &config.Config{
				SecurityEnabled:           true,
				JWTSecret:                 "test-secret",
				AccessTokenExpiry:         config.DurationSeconds(15 * time.Minute),
				MCPServerEnabled:          true,
				MakeViewerEndpointsPublic: viewerPublic,
			}
			jwtManager := security.NewJWTManager(cfg, testutil.NewMemBlacklist(), zap.NewNop())
			mux := &recordingMux{ServeMux: http.NewServeMux()}
			registerRoutes(routeDeps{
				mux:        mux,
				prefix:     "/api/v1",
				cfg:        cfg,
				jwtManager: jwtManager,
				// Generous limit: several principals hit each rate-limited
				// route, and the matrix must observe RBAC rather than 429s.
				loginLimiter: middleware.NewIPRateLimiter(1000, 1000, time.Minute, false),
				h:            allHandlers(t),
			})

			tokens := make(map[string]string, len(principals))
			for i, p := range principals {
				if p.role == "" {
					continue
				}
				token, _, err := jwtManager.GenerateTokens(strconv.Itoa(1000+i), p.role)
				if err != nil {
					t.Fatalf("GenerateTokens(%s): %v", p.role, err)
				}
				tokens[p.name] = token
			}

			mounted := make(map[string]bool, len(mux.patterns))
			for _, pattern := range mux.patterns {
				mounted[pattern] = true
				method, path, ok := strings.Cut(pattern, " ")
				if !ok {
					t.Fatalf("route %q has no method; RBAC defaults are per method", pattern)
				}
				// Everything lives under the API prefix except the signed
				// attachment download, whose URL the MCP server signs as
				// {EXTERNAL_URL}/attachments/{id}.
				if !strings.HasPrefix(path, "/api/v1/") && pattern != "GET /attachments/{id}" {
					t.Errorf("route %q is outside the /api/v1 prefix", pattern)
				}

				want := requiredAccess(method, path)
				if viewerPublic && want == accessViewer {
					want = accessPublic
				}

				// Fill wildcards with "1" so handlers' numeric ID parsing passes.
				segs := strings.Split(path, "/")
				for i, s := range segs {
					if strings.HasPrefix(s, "{") {
						segs[i] = "1"
					}
				}
				url := strings.Join(segs, "/")

				for _, p := range principals {
					req := httptest.NewRequest(method, url, nil)
					if token := tokens[p.name]; token != "" {
						req.Header.Set("Authorization", "Bearer "+token)
					}
					if _, matched := mux.Handler(req); matched != pattern {
						t.Fatalf("probe %s %s routed to %q, want %q", method, url, matched, pattern)
					}
					got := serve(mux, req)
					switch {
					case p.level >= want && (got == http.StatusUnauthorized || got == http.StatusForbidden):
						t.Errorf("%s as %s: got %d, want the handler reached (route requires %s)", pattern, p.name, got, want)
					case p.level < want && got != p.denied:
						t.Errorf("%s as %s: got %d, want %d (route requires %s)", pattern, p.name, got, p.denied, want)
					}
				}
			}

			// The exception lists must name mounted routes, so a renamed or
			// dropped route cannot leave a stale entry that silently checks
			// nothing; the CI ingestion routes must stay mounted.
			for _, list := range []map[string]bool{publicRoutes, selfServiceRoutes, adminProjectRoutes, ciIngestionRoutes} {
				for pattern := range list {
					if !mounted[pattern] {
						t.Errorf("route list names %q, which registerRoutes does not mount", pattern)
					}
				}
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
	dataStore := &testutil.MockStorage{
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
