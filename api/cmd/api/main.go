package main

import (
	"context"
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

// @title           AllureDeck API
// @version         2.0.0
// @description     API for managing Allure test reports.
// @host            localhost:8080
// @BasePath        /api/v1
func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		// Logger is not yet initialised; write to stderr and exit.
		fmt.Fprintf(os.Stderr, "FATAL: configuration error: %v\n", err)
		os.Exit(1)
	}

	logger := logging.Setup(cfg.DevMode, cfg.LogLevel)
	defer func() { _ = logger.Sync() }()

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

	// Declare metadata store interfaces — populated below by the PostgreSQL backend.
	var (
		projectStore    store.ProjectStorer
		buildStore      store.BuildStorer
		blacklistStore  store.BlacklistStorer
		testResultStore store.TestResultStorer
		branchStore     store.BranchStorer
		knownIssueStore store.KnownIssueStorer
		searchStore     store.SearchStorer
		analyticsStore  store.AnalyticsStorer
		apiKeyStore     store.APIKeyStorer
	)

	dataStore, err := createDataStore(cfg, logger)
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err))
	}

	initCtx := context.Background()

	pgDB, err := pg.Open(initCtx, cfg)
	if err != nil {
		logger.Fatal("failed to open PostgreSQL database", zap.Error(err))
	}
	defer func() { _ = pgDB.Close() }()

	pgProj := pg.NewProjectStore(pgDB, logger)
	pgBuild := pg.NewBuildStore(pgDB, logger)
	projectStore = pgProj
	buildStore = pgBuild
	blacklistStore = pg.NewBlacklistStore(pgDB)
	testResultStore = pg.NewTestResultStore(pgDB, logger)
	branchStore = pg.NewBranchStore(pgDB)
	knownIssueStore = pg.NewKnownIssueStore(pgDB)
	searchStore = pg.NewSearchStore(pgDB, logger)
	analyticsStore = pg.NewAnalyticsStore(pgDB)
	apiKeyStore = pg.NewAPIKeyStore(pgDB)
	attachmentStore := pg.NewAttachmentStore(pgDB)
	userStore := pg.NewUserStore(pgDB)
	sqlDB := pgDB.DB()
	var locker store.Locker = pgDB

	if err := pg.SyncMetadata(initCtx, dataStore, pgProj, pgBuild, logger); err != nil {
		logger.Warn("metadata sync failed (non-fatal)", zap.Error(err))
	}

	jwtManager := security.NewJWTManager(cfg, blacklistStore, logger)
	systemHandler := handlers.NewSystemHandler(cfg, sqlDB)
	authHandler := handlers.NewAuthHandler(cfg, jwtManager)
	defectStore := pg.NewDefectStore(pgDB)
	webhookStore := pg.NewWebhookStore(pgDB, encKey, logger)
	allureCore := runner.NewAllure(cfg, dataStore, buildStore, locker, testResultStore, branchStore, defectStore, logger)

	rjm, err := runner.NewRiverJobManager(pgDB.Pool(), allureCore, webhookStore, buildStore, encKey, cfg.ExternalURL, 2, logger)
	if err != nil {
		logger.Fatal("failed to create River job manager", zap.Error(err))
	}
	var jobManager runner.JobQueuer = rjm

	allureHandler := handlers.NewAllureHandler(cfg, allureCore, jobManager, projectStore, buildStore, knownIssueStore, testResultStore, searchStore, dataStore, logger)
	apiReportHandler := handlers.NewReportHandler(jobManager, allureCore, buildStore, branchStore, testResultStore, knownIssueStore, dataStore, cfg, logger)
	projectHandler := handlers.NewProjectHandler(projectStore, allureCore, dataStore, cfg, logger)
	resultUploadHandler := handlers.NewResultUploadHandler(dataStore, projectStore, allureCore, cfg, logger)
	adminHandler := handlers.NewAdminHandler(jobManager, dataStore, cfg.ProjectsPath, logger)
	branchHandler := handlers.NewBranchHandler(branchStore, buildStore, cfg.ProjectsPath)
	testHistoryHandler := handlers.NewTestHistoryHandler(testResultStore, buildStore, branchStore, cfg.ProjectsPath)
	allureHandler.SetBranchStore(branchStore)
	analyticsHandler := handlers.NewAnalyticsHandler(analyticsStore, branchStore, cfg.ProjectsPath, logger)
	attachmentHandler := handlers.NewAttachmentHandler(attachmentStore, buildStore, dataStore, cfg.ProjectsPath, logger)
	apiKeyHandler := handlers.NewAPIKeyHandler(apiKeyStore)
	parentHandler := handlers.NewProjectParentHandler(projectStore, buildStore, cfg.ProjectsPath, logger)

	// Conditionally construct OIDC provider and handler (Stage 2 SSO).
	var oidcHandler *handlers.OIDCHandler
	if cfg.OIDC.Enabled {
		oidcProv, oidcErr := security.NewOIDCProvider(initCtx, &cfg.OIDC, logger)
		if oidcErr != nil {
			logger.Fatal("OIDC discovery failed", zap.Error(oidcErr))
		}
		oidcHandler = handlers.NewOIDCHandler(cfg, oidcProv, jwtManager, userStore, logger)
		logger.Info("OIDC SSO enabled", zap.String("issuer", cfg.OIDC.IssuerURL))
	}

	backgroundWatcher := runner.NewWatcher(cfg, allureCore, projectStore, dataStore, logger)

	mux := http.NewServeMux()

	// Infrastructure endpoints (outside /api/v1)
	mux.HandleFunc("GET /health", systemHandler.Health)
	mux.HandleFunc("GET /healthz", systemHandler.Health)
	mux.HandleFunc("GET /ready", systemHandler.Ready)
	mux.HandleFunc("GET /readyz", systemHandler.Ready)

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
	reportCSP := "default-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob: https:; frame-ancestors " + frameAncestors
	reportHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", reportCSP)
		// Remove X-Frame-Options set by SecurityHeaders middleware; CSP frame-ancestors
		// is the modern replacement and takes precedence in all current browsers.
		w.Header().Del("X-Frame-Options")
		overlayFS.ServeHTTP(w, r)
	})
	mux.Handle("/api/v1/projects/", http.StripPrefix("/api/v1/projects/", reportHandler))

	// Rate limiter for login endpoint — 5 req/s, burst 10, 15min stale TTL (REVIEW #8).
	loginLimiter := middleware.NewIPRateLimiter(5, 10, 15*time.Minute, cfg.TrustForwardedFor)
	limiterDone := make(chan struct{})
	loginLimiter.StartCleanup(5*time.Minute, limiterDone)

	defectHandler := handlers.NewDefectHandler(defectStore, cfg.ProjectsPath, logger)
	webhookHandler := handlers.NewWebhookHandler(webhookStore, cfg.ProjectsPath, logger)
	registerRoutes(mux, "/api/v1", cfg, jwtManager, loginLimiter, systemHandler, authHandler, allureHandler, apiReportHandler, projectHandler, resultUploadHandler, adminHandler, branchHandler, testHistoryHandler, analyticsHandler, attachmentHandler, apiKeyHandler, apiKeyStore, oidcHandler, parentHandler, defectHandler, webhookHandler)

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

	startRetentionScheduler(ctx, cfg, projectStore, buildStore, dataStore, webhookStore, logger)

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
								zap.String("project_id", p.ID), zap.Error(err))
						} else {
							if err := dataStore.PruneReportDirs(ctx, p.ID, removed); err != nil {
								logger.Error("retention scheduler: prune report dirs failed",
									zap.String("project_id", p.ID), zap.Error(err))
							}
							totalPruned += len(removed)
						}
					}
					if cfg.KeepHistoryMaxAgeDays > 0 {
						cutoff := time.Now().AddDate(0, 0, -cfg.KeepHistoryMaxAgeDays)
						aged, err := buildStore.PruneBuildsByAge(ctx, p.ID, cutoff)
						if err != nil {
							logger.Error("retention scheduler: age prune failed",
								zap.String("project_id", p.ID), zap.Error(err))
						} else {
							if err := dataStore.PruneReportDirs(ctx, p.ID, aged); err != nil {
								logger.Error("retention scheduler: prune aged report dirs failed",
									zap.String("project_id", p.ID), zap.Error(err))
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
func registerRoutes(
	mux *http.ServeMux,
	prefix string,
	cfg *config.Config,
	jwtManager *security.JWTManager,
	loginLimiter *middleware.IPRateLimiter,
	system *handlers.SystemHandler,
	authHandler *handlers.AuthHandler,
	allure *handlers.AllureHandler,
	report *handlers.ReportHandler,
	projectHandler *handlers.ProjectHandler,
	resultUploadHandler *handlers.ResultUploadHandler,
	admin *handlers.AdminHandler,
	branchHandler *handlers.BranchHandler,
	testHistoryHandler *handlers.TestHistoryHandler,
	analyticsHandler *handlers.AnalyticsHandler,
	attachmentHandler *handlers.AttachmentHandler,
	apiKeyHandler *handlers.APIKeyHandler,
	apiKeyStore store.APIKeyStorer,
	oidcHandler *handlers.OIDCHandler,
	parentHandler *handlers.ProjectParentHandler,
	defectHandler *handlers.DefectHandler,
	webhookHandler *handlers.WebhookHandler,
) {
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return middleware.AuthMiddleware(cfg, jwtManager, false, apiKeyStore)(h)
	}
	rateLimit := middleware.RateLimitMiddleware(loginLimiter)

	// RBAC wrappers (REVIEW #9): auth + role enforcement.
	adminOnly := func(h http.HandlerFunc) http.HandlerFunc {
		return auth(middleware.RequireRole("admin")(h))
	}
	editorUp := func(h http.HandlerFunc) http.HandlerFunc {
		return auth(middleware.RequireRole("editor")(h))
	}
	// viewerUp: when MakeViewerEndpointsPublic is true, these endpoints are public (no auth).
	viewerUp := func(h http.HandlerFunc) http.HandlerFunc {
		if cfg.MakeViewerEndpointsPublic {
			return h
		}
		return auth(middleware.RequireRole("viewer")(h))
	}

	// Cache-control wrappers (PERF: HTTP caching headers).
	noStore := middleware.NoStore
	mutableCache := middleware.CacheControl(middleware.CacheMutable)
	shortCache := middleware.CacheControl(middleware.CacheShortLived)
	reportCache := middleware.ReportCache

	// Public endpoints — no-store for config/version, rate-limited login.
	mux.HandleFunc("GET "+prefix+"/version", noStore(system.Version))
	mux.HandleFunc("GET "+prefix+"/config", noStore(system.ConfigEndpoint))
	mux.HandleFunc("POST "+prefix+"/login", noStore(rateLimit(authHandler.Login)))

	// Auth only (no specific role required)
	mux.HandleFunc("DELETE "+prefix+"/logout", noStore(auth(authHandler.Logout)))

	// Viewer+ endpoints (public when MakeViewerEndpointsPublic=true) — mutable cache.
	mux.HandleFunc("GET "+prefix+"/search", viewerUp(noStore(allure.Search)))
	mux.HandleFunc("GET "+prefix+"/projects", viewerUp(noStore(projectHandler.GetProjects)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports", viewerUp(mutableCache(report.GetReportHistory)))

	// Admin write endpoints — no-store.
	mux.HandleFunc("POST "+prefix+"/projects", adminOnly(noStore(projectHandler.CreateProject)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}", adminOnly(noStore(projectHandler.DeleteProject)))
	mux.HandleFunc("PUT "+prefix+"/projects/{project_id}/rename", adminOnly(noStore(projectHandler.RenameProject)))
	mux.HandleFunc("POST "+prefix+"/projects/{project_id}/reports", editorUp(noStore(report.GenerateReport)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/jobs/{job_id}", adminOnly(noStore(report.GetJobStatus)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/reports/history", adminOnly(noStore(report.CleanHistory)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/results", adminOnly(noStore(resultUploadHandler.CleanResults)))
	mux.HandleFunc("POST "+prefix+"/projects/{project_id}/results", editorUp(noStore(resultUploadHandler.SendResults)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/reports/{report_id}", adminOnly(noStore(report.DeleteReport)))

	// Multi-build timeline endpoint (registered before report widget routes to avoid {report_id} matching "timeline").
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/timeline", viewerUp(allure.GetProjectTimeline))

	// Report widget endpoints — dynamic cache (immutable for numbered builds, short-lived for latest).
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/categories", viewerUp(reportCache(report.GetReportCategories)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/environment", viewerUp(reportCache(report.GetReportEnvironment)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/known-failures", viewerUp(reportCache(allure.GetReportKnownFailures)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/timeline", viewerUp(reportCache(report.GetReportTimeline)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/stability", viewerUp(reportCache(report.GetReportStability)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/summary", viewerUp(reportCache(report.GetReportSummary)))

	// Known issues list — mutable cache (changes when issues are created/updated/deleted).
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/known-issues", viewerUp(mutableCache(allure.ListKnownIssues)))
	mux.HandleFunc("POST "+prefix+"/projects/{project_id}/known-issues", editorUp(noStore(allure.CreateKnownIssue)))
	mux.HandleFunc("PUT "+prefix+"/projects/{project_id}/known-issues/{issue_id}", editorUp(noStore(allure.UpdateKnownIssue)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/known-issues/{issue_id}", editorUp(noStore(allure.DeleteKnownIssue)))

	// Analytics — short-lived cache.
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/analytics/low-performing", viewerUp(shortCache(allure.GetLowPerformingTests)))

	// Compare — short-lived cache.
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/compare", viewerUp(shortCache(allure.CompareBuilds)))

	// Dashboard — no-store (TanStack Query manages client-side freshness).
	mux.HandleFunc("GET "+prefix+"/dashboard", viewerUp(noStore(allure.GetDashboard)))

	// Project parent-child — admin write, viewer read.
	mux.HandleFunc("PUT "+prefix+"/projects/{project_id}/parent", adminOnly(noStore(parentHandler.SetParent)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/parent", adminOnly(noStore(parentHandler.ClearParent)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/children", viewerUp(mutableCache(parentHandler.ListChildren)))

	// Admin system monitor endpoints.
	mux.HandleFunc("GET "+prefix+"/admin/jobs", adminOnly(noStore(admin.ListJobs)))
	mux.HandleFunc("GET "+prefix+"/admin/results", adminOnly(noStore(admin.ListPendingResults)))
	mux.HandleFunc("POST "+prefix+"/admin/jobs/{job_id}/cancel", adminOnly(noStore(admin.CancelJob)))
	mux.HandleFunc("DELETE "+prefix+"/admin/jobs/{job_id}", adminOnly(noStore(admin.DeleteJob)))
	mux.HandleFunc("DELETE "+prefix+"/admin/results/{project_id}", adminOnly(noStore(admin.CleanProjectResults)))

	// Branch management endpoints.
	if branchHandler != nil {
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/branches", viewerUp(mutableCache(branchHandler.ListBranches)))
		mux.HandleFunc("PUT "+prefix+"/projects/{project_id}/branches/{branch_id}/default", editorUp(noStore(branchHandler.SetDefaultBranch)))
		mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/branches/{branch_id}", adminOnly(noStore(branchHandler.DeleteBranch)))
	}

	// Per-test history endpoint.
	if testHistoryHandler != nil {
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/tests/history", viewerUp(shortCache(testHistoryHandler.GetTestHistory)))
	}

	// Expanded analytics endpoints (PostgreSQL-backed analytics).
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/analytics/errors", viewerUp(shortCache(analyticsHandler.GetTopErrors)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/analytics/suites", viewerUp(shortCache(analyticsHandler.GetSuitePassRates)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/analytics/labels", viewerUp(shortCache(analyticsHandler.GetLabelBreakdown)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/analytics/trends", viewerUp(shortCache(analyticsHandler.GetTrends)))

	// Attachment viewer endpoints.
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/attachments", viewerUp(reportCache(attachmentHandler.ListAttachments)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/attachments/{source}", viewerUp(reportCache(attachmentHandler.ServeAttachment)))

	// API Keys (any authenticated user).
	if apiKeyHandler != nil {
		mux.HandleFunc("GET "+prefix+"/api-keys", auth(noStore(apiKeyHandler.List)))
		mux.HandleFunc("POST "+prefix+"/api-keys", auth(noStore(apiKeyHandler.Create)))
		mux.HandleFunc("DELETE "+prefix+"/api-keys/{id}", auth(noStore(apiKeyHandler.Delete)))
	}

	// Session endpoint (auth-required).
	mux.HandleFunc("GET "+prefix+"/auth/session", auth(noStore(authHandler.Session)))

	// OIDC SSO routes (public, rate-limited).
	if oidcHandler != nil {
		mux.HandleFunc("GET "+prefix+"/auth/oidc/login", noStore(rateLimit(oidcHandler.Login)))
		mux.HandleFunc("GET "+prefix+"/auth/oidc/callback", noStore(rateLimit(oidcHandler.Callback)))
	}

	// Defect fingerprint endpoints.
	if defectHandler != nil {
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/defects", viewerUp(noStore(defectHandler.ListProjectDefects)))
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/defects/summary", viewerUp(mutableCache(defectHandler.GetProjectDefectSummary)))
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/defects/{defect_id}", viewerUp(noStore(defectHandler.GetDefect)))
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/defects/{defect_id}/tests", viewerUp(noStore(defectHandler.GetDefectTests)))
		mux.HandleFunc("PATCH "+prefix+"/projects/{project_id}/defects/{defect_id}", editorUp(noStore(defectHandler.UpdateDefect)))
		mux.HandleFunc("POST "+prefix+"/projects/{project_id}/defects/bulk", editorUp(noStore(defectHandler.BulkUpdateDefects)))
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/builds/{build_id}/defects", viewerUp(noStore(defectHandler.ListBuildDefects)))
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/builds/{build_id}/defects/summary", viewerUp(mutableCache(defectHandler.GetBuildDefectSummary)))
	}

	// Webhook management endpoints.
	if webhookHandler != nil {
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/webhooks", editorUp(noStore(webhookHandler.List)))
		mux.HandleFunc("POST "+prefix+"/projects/{project_id}/webhooks", editorUp(noStore(webhookHandler.Create)))
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/webhooks/{webhook_id}", editorUp(noStore(webhookHandler.Get)))
		mux.HandleFunc("PUT "+prefix+"/projects/{project_id}/webhooks/{webhook_id}", editorUp(noStore(webhookHandler.Update)))
		mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/webhooks/{webhook_id}", editorUp(noStore(webhookHandler.Delete)))
		mux.HandleFunc("POST "+prefix+"/projects/{project_id}/webhooks/{webhook_id}/test", editorUp(noStore(webhookHandler.Test)))
		mux.HandleFunc("GET "+prefix+"/projects/{project_id}/webhooks/{webhook_id}/deliveries", editorUp(noStore(webhookHandler.ListDeliveries)))
	}
}
