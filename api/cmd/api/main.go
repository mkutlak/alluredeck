package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	httpSwagger "github.com/swaggo/http-swagger/v2"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/handlers"
	"github.com/mkutlak/alluredeck/api/internal/logging"
	"github.com/mkutlak/alluredeck/api/internal/middleware"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
	swaggerDocs "github.com/mkutlak/alluredeck/api/internal/swagger"
)

// stores groups all database store instances.
type stores struct {
	project    store.ProjectStorer
	build      store.BuildStorer
	blacklist  store.BlacklistStorer
	testResult store.TestResultStorer
	branch     store.BranchStorer
	knownIssue store.KnownIssueStorer
	search     store.SearchStorer
	analytics  store.AnalyticsStorer
	apiKey     store.APIKeyStorer
	attachment store.AttachmentStorer
	user       store.UserStorer
	defect     store.DefectStorer
	webhook    store.WebhookStorer
	pipeline   store.PipelineStorer
	preference store.PreferenceStorer
}

// handlerSet groups all HTTP handler instances.
type handlerSet struct {
	system          *handlers.SystemHandler
	auth            *handlers.AuthHandler
	report          *handlers.ReportHandler
	project         *handlers.ProjectHandler
	resultUpload    *handlers.ResultUploadHandler
	playwright      *handlers.PlaywrightHandler
	admin           *handlers.AdminHandler
	branch          *handlers.BranchHandler
	testHistory     *handlers.TestHistoryHandler
	analytics       *handlers.AnalyticsHandler
	search          *handlers.SearchHandler
	compare         *handlers.CompareHandler
	dashboard       *handlers.DashboardHandler
	lowPerf         *handlers.LowPerformingHandler
	projectTimeline *handlers.ProjectTimelineHandler
	knownIssue      *handlers.KnownIssueHandler
	attachment      *handlers.AttachmentHandler
	apiKey          *handlers.APIKeyHandler
	parent          *handlers.ProjectParentHandler
	defect          *handlers.DefectHandler
	webhook         *handlers.WebhookHandler
	pipeline        *handlers.PipelineHandler
	preferences     *handlers.PreferenceHandler
	oidc            *handlers.OIDCHandler // may be nil
}

// routeDeps bundles all dependencies needed by registerRoutes.
type routeDeps struct {
	mux          *http.ServeMux
	prefix       string
	cfg          *config.Config
	jwtManager   *security.JWTManager
	loginLimiter *middleware.IPRateLimiter
	apiKeyStore  store.APIKeyStorer
	h            handlerSet
}

// @title           AllureDeck API
// @version         2.0.0
// @description     API for managing Allure test reports.
// @host            localhost:8080
// @BasePath        /api/v1
func main() {
	cfg, encKey, logger := mustLoadConfig()
	defer func() { _ = logger.Sync() }()

	dataStore, err := createDataStore(cfg, logger)
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}

	s, sqlDB, locker, pgDB := mustInitStores(cfg, dataStore, encKey, logger)
	defer func() { _ = pgDB.Close() }()

	allureCore := runner.NewAllure(runner.AllureDeps{
		Config:          cfg,
		Store:           dataStore,
		BuildStore:      s.build,
		Locker:          locker,
		TestResultStore: s.testResult,
		BranchStore:     s.branch,
		DefectStore:     s.defect,
		AttachmentStore: s.attachment,
		Logger:          logger,
	})

	pwRunner := runner.NewPlaywrightRunner(runner.PlaywrightRunnerDeps{
		Config:          cfg,
		Store:           dataStore,
		BuildStore:      s.build,
		Locker:          locker,
		TestResultStore: s.testResult,
		BranchStore:     s.branch,
		DefectStore:     s.defect,
		Logger:          logger,
	})

	rjm, err := runner.NewRiverJobManager(pgDB.Pool(), allureCore, pwRunner, s.webhook, s.build, encKey, cfg.ExternalURL, 2, logger)
	if err != nil {
		logger.Fatal("failed to create River job manager", zap.Error(err))
	}
	var jobManager runner.JobQueuer = rjm

	jwtManager := security.NewJWTManager(cfg, s.blacklist, logger)

	h := wireHandlers(cfg, s, sqlDB, dataStore, allureCore, jobManager, jwtManager, logger)

	backgroundWatcher := runner.NewWatcher(cfg, allureCore, s.project, dataStore, logger)

	mux := http.NewServeMux()

	// Infrastructure endpoints (outside /api/v1)
	mux.HandleFunc("GET /health", h.system.Health)
	mux.HandleFunc("GET /healthz", h.system.Health)
	mux.HandleFunc("GET /ready", h.system.Ready)
	mux.HandleFunc("GET /readyz", h.system.Ready)

	// Override Swagger host: empty string = use browser's current host (works for all environments).
	swaggerDocs.SwaggerInfo.Host = cfg.SwaggerHost
	swaggerDocs.SwaggerInfo.Schemes = []string{}

	// Swagger UI
	if cfg.SwaggerEnabled {
		mux.HandleFunc("GET /swagger/", httpSwagger.Handler(
			httpSwagger.URL("/swagger/doc.json"),
		))
	}

	// Overlay file server — serves generated Allure HTML reports.
	// Numbered build dirs contain only variable content (data/, widgets/, history/).
	// Static assets (index.html, JS, CSS, plugins/) are served from reports/latest/
	// via a fallback overlay, achieving ~90% disk reduction per build.
	var overlayFS http.Handler
	if cfg.StorageType == "s3" {
		overlayFS = newS3ReportHandler(dataStore)
	} else {
		overlayFS = newOverlayHandler(cfg.ProjectsPath)
	}
	// Allure report pages are third-party static content requiring inline scripts/styles.
	// Override the strict API CSP with a permissive one for report routes.
	// Build frame-ancestors from CORS_ALLOWED_ORIGINS so only the configured
	// dashboard origin can embed reports in iframes. Falls back to '*' if no
	// origins are configured (e.g. local dev without explicit CORS setting).
	frameAncestors := strings.Join(cfg.CORSAllowedOrigins, " ")
	if frameAncestors == "" {
		frameAncestors = "*"
	}
	// Playwright trace viewer — serves embedded static files; no auth required.
	// Uses the same dynamic frame-ancestors so the UI can embed the viewer.
	mux.Handle("/trace/", newTraceViewerHandler(frameAncestors))

	reportCSP := "default-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob: https:; frame-ancestors " + frameAncestors
	reportHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", reportCSP)
		// Remove X-Frame-Options set by SecurityHeaders middleware; CSP frame-ancestors
		// is the modern replacement and takes precedence in all current browsers.
		w.Header().Del("X-Frame-Options")
		overlayFS.ServeHTTP(w, r)
	})
	mux.Handle("/api/v1/projects/", http.StripPrefix("/api/v1/projects/", reportHandler))

	// Playwright report file server — serves self-contained HTML reports from
	// playwright-reports/{reportID}/. Each numbered build is a complete report so
	// no overlay fallback is needed. The more-specific pattern takes precedence over
	// the catch-all /api/v1/projects/ handle above (Go 1.22+ longest-match routing).
	pwReportHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", reportCSP)
		w.Header().Del("X-Frame-Options")
		newPlaywrightReportHandler(dataStore).ServeHTTP(w, r)
	})
	mux.Handle("GET /api/v1/projects/{projectID}/playwright-reports/{reportID}/{rest...}", pwReportHandler)

	// Rate limiter for login endpoint — 5 req/s, burst 10, 15min stale TTL (REVIEW #8).
	loginLimiter := middleware.NewIPRateLimiter(5, 10, 15*time.Minute, cfg.TrustForwardedFor)
	limiterDone := make(chan struct{})
	loginLimiter.StartCleanup(5*time.Minute, limiterDone)

	registerRoutes(routeDeps{
		mux:          mux,
		prefix:       "/api/v1",
		cfg:          cfg,
		jwtManager:   jwtManager,
		loginLimiter: loginLimiter,
		apiKeyStore:  s.apiKey,
		h:            h,
	})

	// Chain middleware: Recovery → RequestID → Logging → SecurityHeaders → CSRF → CORS → mux (AUDIT 3.1, 2.6, REVIEW #11, #16).
	handler := middleware.Recovery(
		middleware.RequestID(
			middleware.LoggingMiddleware(logger)(
				middleware.SecurityHeaders(
					middleware.CSRFMiddleware(cfg)(
						middleware.CORSMiddleware(cfg, mux),
					),
				),
			),
		),
	)

	addr := "0.0.0.0:" + cfg.Port
	srv := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,  // AUDIT 2.5
		WriteTimeout: 60 * time.Second,  // AUDIT 2.5
		IdleTimeout:  120 * time.Second, // AUDIT 2.5
	}

	// Graceful shutdown on SIGTERM / SIGINT
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	jobManager.Start(ctx)

	// Start JWT blacklist cleanup goroutine — prunes expired JTIs every 15 min (AUDIT 2.1).
	jwtManager.StartCleanup(ctx, 15*time.Minute)

	startRetentionScheduler(ctx, cfg, s.project, s.build, dataStore, s.webhook, logger)

	if cfg.StorageType != "s3" {
		backgroundWatcher.Start()
	} else {
		logger.Info("background file watcher is disabled", zap.String("reason", "StorageType=s3"))
	}

	go func() {
		logger.Info("starting Allure API server", zap.String("addr", addr), zap.Bool("dev_mode", cfg.DevMode))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	awaitShutdown(ctx, srv, backgroundWatcher, jobManager, cfg, limiterDone, logger)
}

// mustLoadConfig loads and validates configuration, sets up the logger,
// hashes passwords (if security is enabled), and derives the encryption key.
// It terminates the process on any fatal configuration error.
func mustLoadConfig() (*config.Config, []byte, *zap.Logger) {
	cfg, err := config.LoadConfig()
	if err != nil {
		// Logger is not yet initialised; write to stderr and exit.
		fmt.Fprintf(os.Stderr, "FATAL: configuration error: %v\n", err)
		os.Exit(1)
	}

	logger := logging.Setup(cfg.DevMode, cfg.LogLevel)

	// Fail fast if security is enabled with an insecure default secret (AUDIT 1.3).
	if err := cfg.Validate(); err != nil {
		logger.Fatal("configuration error", zap.Error(err))
	}

	// Bcrypt-hash plaintext passwords and zero out the originals (C3 fix).
	if cfg.SecurityEnabled {
		if err := cfg.HashPasswords(); err != nil {
			logger.Fatal("failed to hash passwords", zap.Error(err))
		}
	}

	encKey := security.DeriveEncryptionKey(cfg.JWTSecret)
	return cfg, encKey, logger
}

// mustInitStores opens the PostgreSQL database connection and initialises all
// store instances. encKey is required for the webhook store. Returns the
// populated stores struct, the *sql.DB handle needed by SystemHandler, the
// Locker interface, and the raw *pg.PGStore so the caller can pass its pool
// to the job manager and register a deferred close.
func mustInitStores(cfg *config.Config, dataStore storage.Store, encKey []byte, logger *zap.Logger) (stores, *sql.DB, store.Locker, *pg.PGStore) {
	initCtx := context.Background()

	pgDB, err := pg.Open(initCtx, cfg)
	if err != nil {
		logger.Fatal("failed to open PostgreSQL database", zap.Error(err))
	}

	pgProj := pg.NewProjectStore(pgDB, logger)
	pgBuild := pg.NewBuildStore(pgDB, logger)

	s := stores{
		project:    pgProj,
		build:      pgBuild,
		blacklist:  pg.NewBlacklistStore(pgDB),
		testResult: pg.NewTestResultStore(pgDB, logger),
		branch:     pg.NewBranchStore(pgDB),
		knownIssue: pg.NewKnownIssueStore(pgDB),
		search:     pg.NewSearchStore(pgDB, logger),
		analytics:  pg.NewAnalyticsStore(pgDB),
		apiKey:     pg.NewAPIKeyStore(pgDB),
		attachment: pg.NewAttachmentStore(pgDB),
		user:       pg.NewUserStore(pgDB),
		defect:     pg.NewDefectStore(pgDB),
		webhook:    pg.NewWebhookStore(pgDB, encKey, logger),
		pipeline:   pg.NewPipelineStore(pgDB),
		preference: pg.NewPreferenceStore(pgDB),
	}

	if err := pg.SyncMetadata(initCtx, dataStore, pgProj, pgBuild, logger); err != nil {
		logger.Warn("metadata sync failed (non-fatal)", zap.Error(err))
	}

	sqlDB := pgDB.DB()
	var locker store.Locker = pgDB
	return s, sqlDB, locker, pgDB
}

// wireHandlers constructs every HTTP handler and returns them in a handlerSet.
// The conditional OIDC handler is nil when OIDC is not enabled.
func wireHandlers(
	cfg *config.Config,
	s stores,
	sqlDB *sql.DB,
	dataStore storage.Store,
	allureCore *runner.Allure,
	jobManager runner.JobQueuer,
	jwtManager *security.JWTManager,
	logger *zap.Logger,
) handlerSet {
	var oidcHandler *handlers.OIDCHandler
	if cfg.OIDC.Enabled {
		initCtx := context.Background()
		oidcProv, oidcErr := security.NewOIDCProvider(initCtx, &cfg.OIDC, logger)
		if oidcErr != nil {
			logger.Fatal("OIDC discovery failed", zap.Error(oidcErr))
		}
		oidcHandler = handlers.NewOIDCHandler(cfg, oidcProv, jwtManager, s.user, logger)
		logger.Info("OIDC SSO enabled", zap.String("issuer", cfg.OIDC.IssuerURL))
	}

	return handlerSet{
		system: handlers.NewSystemHandler(cfg, sqlDB),
		auth:   handlers.NewAuthHandler(cfg, jwtManager),
		report: handlers.NewReportHandler(handlers.ReportHandlerDeps{
			JobManager:      jobManager,
			Runner:          allureCore,
			BuildStore:      s.build,
			BranchStore:     s.branch,
			TestResultStore: s.testResult,
			KnownIssueStore: s.knownIssue,
			ProjectStore:    s.project,
			Store:           dataStore,
			Config:          cfg,
			Logger:          logger,
		}),
		project:         handlers.NewProjectHandler(s.project, allureCore, dataStore, cfg, logger),
		resultUpload:    handlers.NewResultUploadHandler(dataStore, s.project, jobManager, allureCore, cfg, logger),
		playwright:      handlers.NewPlaywrightHandler(dataStore, s.project, s.build, jobManager, cfg, logger),
		admin:           handlers.NewAdminHandlerWithProjects(jobManager, dataStore, s.project, logger),
		branch:          handlers.NewBranchHandler(s.branch, s.build, s.project),
		testHistory:     handlers.NewTestHistoryHandler(s.testResult, s.build, s.branch, s.project),
		analytics:       handlers.NewAnalyticsHandler(s.analytics, s.branch, s.project, logger),
		search:          handlers.NewSearchHandler(s.search),
		compare:         handlers.NewCompareHandler(s.testResult, s.project),
		dashboard:       handlers.NewDashboardHandler(s.build, logger),
		lowPerf:         handlers.NewLowPerformingHandler(s.testResult, s.branch, s.project, logger),
		projectTimeline: handlers.NewProjectTimelineHandler(s.build, s.testResult, s.branch, s.project),
		knownIssue:      handlers.NewKnownIssueHandler(s.knownIssue, s.project, dataStore, logger),
		attachment:      handlers.NewAttachmentHandler(s.attachment, s.build, s.project, dataStore, logger),
		apiKey:          handlers.NewAPIKeyHandler(s.apiKey),
		parent:          handlers.NewProjectParentHandler(s.project, logger),
		defect:          handlers.NewDefectHandler(s.defect, s.project, logger),
		webhook:         handlers.NewWebhookHandler(s.webhook, s.project, logger),
		pipeline:        handlers.NewPipelineHandler(s.pipeline, s.project, cfg.ProjectsPath, logger),
		preferences:     handlers.NewPreferenceHandler(s.preference),
		oidc:            oidcHandler,
	}
}

// awaitShutdown waits for the context to be cancelled then drains the HTTP
// server, stops the background watcher, and waits for in-flight jobs before returning.
func awaitShutdown(ctx context.Context, srv *http.Server, watcher *runner.Watcher, jobManager runner.JobQueuer, cfg *config.Config, limiterDone chan struct{}, logger *zap.Logger) {
	<-ctx.Done()
	logger.Info("shutdown signal received, draining connections")

	close(limiterDone) // stop rate limiter cleanup goroutine

	if cfg.StorageType != "s3" {
		watcher.Stop()
	}

	jobManager.Shutdown()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("server shutdown error", zap.Error(err))
	}
	logger.Info("server stopped cleanly")
}

// startRetentionScheduler runs a daily goroutine that prunes old builds for all projects.
// It applies count-based and age-based pruning according to the configured retention settings.
func startRetentionScheduler(ctx context.Context, cfg *config.Config, projectStore store.ProjectStorer,
	buildStore store.BuildStorer, dataStore storage.Store, webhookStore store.WebhookStorer, logger *zap.Logger) {
	if cfg.KeepHistoryLatest <= 0 && cfg.KeepHistoryMaxAgeDays <= 0 {
		logger.Info("retention scheduler disabled")
		return
	}
	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				projects, err := projectStore.ListProjects(ctx)
				if err != nil {
					logger.Error("retention scheduler: list projects failed", zap.Error(err))
					continue
				}
				totalPruned := 0
				for _, p := range projects {
					if cfg.KeepHistoryLatest > 0 {
						removed, err := buildStore.PruneBuildsBranch(ctx, p.ID, cfg.KeepHistoryLatest, nil)
						if err != nil {
							logger.Error("retention scheduler: count prune failed",
								zap.Int64("project_id", p.ID), zap.Error(err))
						} else {
							if err := dataStore.PruneReportDirs(ctx, p.Slug, removed); err != nil {
								logger.Error("retention scheduler: prune report dirs failed",
									zap.Int64("project_id", p.ID), zap.Error(err))
							}
							totalPruned += len(removed)
						}
					}
					if cfg.KeepHistoryMaxAgeDays > 0 {
						cutoff := time.Now().AddDate(0, 0, -cfg.KeepHistoryMaxAgeDays)
						aged, err := buildStore.PruneBuildsByAge(ctx, p.ID, cutoff)
						if err != nil {
							logger.Error("retention scheduler: age prune failed",
								zap.Int64("project_id", p.ID), zap.Error(err))
						} else {
							if err := dataStore.PruneReportDirs(ctx, p.Slug, aged); err != nil {
								logger.Error("retention scheduler: prune aged report dirs failed",
									zap.Int64("project_id", p.ID), zap.Error(err))
							}
							totalPruned += len(aged)
						}
					}
				}
				logger.Info("retention scheduler: daily run complete", zap.Int("builds_pruned", totalPruned))

				// Prune webhook deliveries older than 30 days.
				whCutoff := time.Now().AddDate(0, 0, -30)
				if n, err := webhookStore.PruneDeliveries(ctx, whCutoff); err != nil {
					logger.Error("retention scheduler: webhook delivery prune failed", zap.Error(err))
				} else if n > 0 {
					logger.Info("retention scheduler: pruned webhook deliveries", zap.Int64("count", n))
				}

				// Prune stale pending results.
				if cfg.PendingResultsMaxAgeDays > 0 {
					pendingCutoff := time.Now().AddDate(0, 0, -cfg.PendingResultsMaxAgeDays)
					pendingCleaned := 0
					for _, p := range projects {
						entries, err := dataStore.ReadDir(ctx, p.Slug, "results")
						if err != nil || len(entries) == 0 {
							continue
						}
						var newest int64
						for _, e := range entries {
							if !e.IsDir && e.ModTime > newest {
								newest = e.ModTime
							}
						}
						if newest > 0 && time.Unix(0, newest).Before(pendingCutoff) {
							if err := dataStore.CleanResults(ctx, p.Slug); err != nil {
								logger.Warn("retention scheduler: pending results cleanup failed",
									zap.String("slug", p.Slug), zap.Error(err))
								continue
							}
							pendingCleaned++
						}
					}
					if pendingCleaned > 0 {
						logger.Info("retention scheduler: pending results cleanup",
							zap.Int("projects_cleaned", pendingCleaned))
					}
				}
			}
		}
	}()
}

// createDataStore initialises the storage backend based on StorageType config.
func createDataStore(cfg *config.Config, logger *zap.Logger) (storage.Store, error) {
	switch cfg.StorageType {
	case "s3":
		st, err := storage.NewS3Store(cfg, logger)
		if err != nil {
			return nil, fmt.Errorf("init S3 store: %w", err)
		}
		return st, nil
	default:
		return storage.NewLocalStore(cfg), nil
	}
}

// registerRoutes mounts all API routes under the given URL prefix.
func registerRoutes(d routeDeps) {
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return middleware.AuthMiddleware(d.cfg, d.jwtManager, false, d.apiKeyStore)(h)
	}
	rateLimit := middleware.RateLimitMiddleware(d.loginLimiter)

	// RBAC wrappers (REVIEW #9): auth + role enforcement.
	adminOnly := func(h http.HandlerFunc) http.HandlerFunc {
		return auth(middleware.RequireRole("admin")(h))
	}
	editorUp := func(h http.HandlerFunc) http.HandlerFunc {
		return auth(middleware.RequireRole("editor")(h))
	}
	// viewerUp: when MakeViewerEndpointsPublic is true, these endpoints are public (no auth).
	viewerUp := func(h http.HandlerFunc) http.HandlerFunc {
		if d.cfg.MakeViewerEndpointsPublic {
			return h
		}
		return auth(middleware.RequireRole("viewer")(h))
	}

	// Cache-control wrappers (PERF: HTTP caching headers).
	noStore := middleware.NoStore
	mutableCache := middleware.CacheControl(middleware.CacheMutable)
	shortCache := middleware.CacheControl(middleware.CacheShortLived)
	reportCache := middleware.ReportCache

	prefix := d.prefix
	mux := d.mux

	// Public endpoints — no-store for config/version, rate-limited login.
	mux.HandleFunc("GET "+prefix+"/version", noStore(d.h.system.Version))
	mux.HandleFunc("GET "+prefix+"/config", noStore(d.h.system.ConfigEndpoint))
	mux.HandleFunc("POST "+prefix+"/login", noStore(rateLimit(d.h.auth.Login)))

	// Auth only (no specific role required)
	mux.HandleFunc("DELETE "+prefix+"/logout", noStore(auth(d.h.auth.Logout)))

	// User preferences (any authenticated user)
	mux.HandleFunc("GET "+prefix+"/preferences", auth(noStore(d.h.preferences.GetPreferences)))
	mux.HandleFunc("PUT "+prefix+"/preferences", auth(noStore(d.h.preferences.UpsertPreferences)))

	// Viewer+ endpoints (public when MakeViewerEndpointsPublic=true) — mutable cache.
	mux.HandleFunc("GET "+prefix+"/search", viewerUp(noStore(d.h.search.Search)))
	mux.HandleFunc("GET "+prefix+"/projects", viewerUp(noStore(d.h.project.GetProjects)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports", viewerUp(mutableCache(d.h.report.GetReportHistory)))

	// Admin write endpoints — no-store.
	mux.HandleFunc("POST "+prefix+"/projects", adminOnly(noStore(d.h.project.CreateProject)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}", adminOnly(noStore(d.h.project.DeleteProject)))
	mux.HandleFunc("PUT "+prefix+"/projects/{project_id}/rename", adminOnly(noStore(d.h.project.RenameProject)))
	mux.HandleFunc("POST "+prefix+"/projects/{project_id}/reports", editorUp(noStore(d.h.report.GenerateReport)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/jobs/{job_id}", adminOnly(noStore(d.h.report.GetJobStatus)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/reports/history/group", adminOnly(noStore(d.h.report.CleanGroupHistory)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/reports/history", adminOnly(noStore(d.h.report.CleanHistory)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/results", adminOnly(noStore(d.h.resultUpload.CleanResults)))
	mux.HandleFunc("POST "+prefix+"/projects/{project_id}/results", editorUp(noStore(d.h.resultUpload.SendResults)))
	mux.HandleFunc("POST "+prefix+"/projects/{project_id}/playwright", editorUp(noStore(d.h.playwright.UploadReport)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/reports/{report_id}", adminOnly(noStore(d.h.report.DeleteReport)))

	// Multi-build timeline endpoint (registered before report widget routes to avoid {report_id} matching "timeline").
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/timeline", viewerUp(d.h.projectTimeline.GetProjectTimeline))

	// Report widget endpoints — dynamic cache (immutable for numbered builds, short-lived for latest).
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/categories", viewerUp(reportCache(d.h.report.GetReportCategories)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/environment", viewerUp(reportCache(d.h.report.GetReportEnvironment)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/known-failures", viewerUp(reportCache(d.h.knownIssue.GetReportKnownFailures)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/timeline", viewerUp(reportCache(d.h.report.GetReportTimeline)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/stability", viewerUp(reportCache(d.h.report.GetReportStability)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/summary", viewerUp(reportCache(d.h.report.GetReportSummary)))

	// Known issues list — mutable cache (changes when issues are created/updated/deleted).
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/known-issues", viewerUp(mutableCache(d.h.knownIssue.ListKnownIssues)))
	mux.HandleFunc("POST "+prefix+"/projects/{project_id}/known-issues", editorUp(noStore(d.h.knownIssue.CreateKnownIssue)))
	mux.HandleFunc("PUT "+prefix+"/projects/{project_id}/known-issues/{issue_id}", editorUp(noStore(d.h.knownIssue.UpdateKnownIssue)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/known-issues/{issue_id}", editorUp(noStore(d.h.knownIssue.DeleteKnownIssue)))

	// Analytics — short-lived cache.
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/analytics/low-performing", viewerUp(shortCache(d.h.lowPerf.GetLowPerformingTests)))

	// Compare — short-lived cache.
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/compare", viewerUp(shortCache(d.h.compare.CompareBuilds)))

	// Dashboard — no-store (TanStack Query manages client-side freshness).
	mux.HandleFunc("GET "+prefix+"/dashboard", viewerUp(noStore(d.h.dashboard.GetDashboard)))

	// Project parent-child — admin write, viewer read.
	mux.HandleFunc("PUT "+prefix+"/projects/{project_id}/parent", adminOnly(noStore(d.h.parent.SetParent)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/parent", adminOnly(noStore(d.h.parent.ClearParent)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/children", viewerUp(mutableCache(d.h.parent.ListChildren)))

	// Pipeline runs (parent project aggregation by commit SHA).
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/pipeline-runs", viewerUp(shortCache(d.h.pipeline.GetPipelineRuns)))

	// Admin system monitor endpoints.
	mux.HandleFunc("GET "+prefix+"/admin/jobs", adminOnly(noStore(d.h.admin.ListJobs)))
	mux.HandleFunc("GET "+prefix+"/admin/results", adminOnly(noStore(d.h.admin.ListPendingResults)))
	mux.HandleFunc("POST "+prefix+"/admin/jobs/{job_id}/cancel", adminOnly(noStore(d.h.admin.CancelJob)))
	mux.HandleFunc("DELETE "+prefix+"/admin/jobs/{job_id}", adminOnly(noStore(d.h.admin.DeleteJob)))
	mux.HandleFunc("DELETE "+prefix+"/admin/results", adminOnly(noStore(d.h.admin.CleanBulkResults)))
	mux.HandleFunc("DELETE "+prefix+"/admin/results/{project_id}", adminOnly(noStore(d.h.admin.CleanProjectResults)))
	mux.HandleFunc("DELETE "+prefix+"/admin/results/group/{project_id}", adminOnly(noStore(d.h.admin.CleanGroupResults)))

	// Branch management endpoints.
	if d.h.branch != nil {
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/branches", viewerUp(mutableCache(d.h.branch.ListBranches)))
		mux.HandleFunc("PUT "+prefix+"/projects/{project_id}/branches/{branch_id}/default", editorUp(noStore(d.h.branch.SetDefaultBranch)))
		mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/branches/{branch_id}", adminOnly(noStore(d.h.branch.DeleteBranch)))
	}

	// Per-test history endpoint.
	if d.h.testHistory != nil {
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/tests/history", viewerUp(shortCache(d.h.testHistory.GetTestHistory)))
	}

	// Expanded analytics endpoints (PostgreSQL-backed analytics).
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/analytics/errors", viewerUp(shortCache(d.h.analytics.GetTopErrors)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/analytics/suites", viewerUp(shortCache(d.h.analytics.GetSuitePassRates)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/analytics/labels", viewerUp(shortCache(d.h.analytics.GetLabelBreakdown)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/analytics/trends", viewerUp(shortCache(d.h.analytics.GetTrends)))

	// Attachment viewer endpoints.
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/attachments", viewerUp(reportCache(d.h.attachment.ListAttachments)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/attachments/{source}", viewerUp(reportCache(d.h.attachment.ServeAttachment)))

	// API Keys (any authenticated user).
	if d.h.apiKey != nil {
		mux.HandleFunc("GET "+prefix+"/api-keys", auth(noStore(d.h.apiKey.List)))
		mux.HandleFunc("POST "+prefix+"/api-keys", auth(noStore(d.h.apiKey.Create)))
		mux.HandleFunc("DELETE "+prefix+"/api-keys/{id}", auth(noStore(d.h.apiKey.Delete)))
	}

	// Session endpoint (auth-required).
	mux.HandleFunc("GET "+prefix+"/auth/session", auth(noStore(d.h.auth.Session)))

	// OIDC SSO routes (public, rate-limited).
	if d.h.oidc != nil {
		mux.HandleFunc("GET "+prefix+"/auth/oidc/login", noStore(rateLimit(d.h.oidc.Login)))
		mux.HandleFunc("GET "+prefix+"/auth/oidc/callback", noStore(rateLimit(d.h.oidc.Callback)))
	}

	// Defect fingerprint endpoints.
	if d.h.defect != nil {
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/defects", viewerUp(noStore(d.h.defect.ListProjectDefects)))
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/defects/summary", viewerUp(mutableCache(d.h.defect.GetProjectDefectSummary)))
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/defects/{defect_id}", viewerUp(noStore(d.h.defect.GetDefect)))
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/defects/{defect_id}/tests", viewerUp(noStore(d.h.defect.GetDefectTests)))
		mux.HandleFunc("PATCH "+prefix+"/projects/{project_id}/defects/{defect_id}", editorUp(noStore(d.h.defect.UpdateDefect)))
		mux.HandleFunc("POST "+prefix+"/projects/{project_id}/defects/bulk", editorUp(noStore(d.h.defect.BulkUpdateDefects)))
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/builds/{build_id}/defects", viewerUp(noStore(d.h.defect.ListBuildDefects)))
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/builds/{build_id}/defects/summary", viewerUp(mutableCache(d.h.defect.GetBuildDefectSummary)))
	}

	// Webhook management endpoints.
	if d.h.webhook != nil {
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/webhooks", editorUp(noStore(d.h.webhook.List)))
		mux.HandleFunc("POST "+prefix+"/projects/{project_id}/webhooks", editorUp(noStore(d.h.webhook.Create)))
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/webhooks/{webhook_id}", editorUp(noStore(d.h.webhook.Get)))
		mux.HandleFunc("PUT "+prefix+"/projects/{project_id}/webhooks/{webhook_id}", editorUp(noStore(d.h.webhook.Update)))
		mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/webhooks/{webhook_id}", editorUp(noStore(d.h.webhook.Delete)))
		mux.HandleFunc("POST "+prefix+"/projects/{project_id}/webhooks/{webhook_id}/test", editorUp(noStore(d.h.webhook.Test)))
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/webhooks/{webhook_id}/deliveries", editorUp(noStore(d.h.webhook.ListDeliveries)))
	}
}
