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
	_ "github.com/mkutlak/alluredeck/api/internal/swagger"
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

	// Open SQLite metadata store.
	db, err := store.Open(cfg.DatabasePath)
	if err != nil {
		logger.Fatal("failed to open database", zap.String("path", cfg.DatabasePath), zap.Error(err))
	}
	defer func() { _ = db.Close() }()

	projectStore := store.NewProjectStore(db, logger)
	buildStore := store.NewBuildStore(db, logger)
	blacklistStore := store.NewBlacklistStore(db)
	lockManager := store.NewLockManager()

	dataStore, err := createDataStore(cfg, logger)
	if err != nil {
		logger.Fatal("failed to initialize storage", zap.Error(err)) //nolint:gocritic // exitAfterDefer: in-process SQLite; OS cleans up on exit
	}

	// Import existing filesystem projects and builds into the database on startup.
	if err := store.SyncMetadata(context.Background(), dataStore, db, logger); err != nil {
		logger.Warn("metadata sync failed (non-fatal)", zap.Error(err))
	}

	jwtManager := security.NewJWTManager(cfg, blacklistStore)
	systemHandler := handlers.NewSystemHandler(cfg, db.DB())
	authHandler := handlers.NewAuthHandler(cfg, jwtManager)

	testResultStore := store.NewTestResultStore(db, logger)
	allureCore := runner.NewAllure(cfg, dataStore, buildStore, lockManager, testResultStore, logger)
	knownIssueStore := store.NewKnownIssueStore(db)
	searchStore := store.NewSearchStore(db, logger)
	jobManager := runner.NewJobManager(allureCore, 2, logger)
	allureHandler := handlers.NewAllureHandler(cfg, allureCore, jobManager, projectStore, buildStore, knownIssueStore, testResultStore, searchStore, dataStore)

	backgroundWatcher := runner.NewWatcher(cfg, allureCore, projectStore, dataStore, logger)

	mux := http.NewServeMux()

	// Infrastructure endpoints (outside /api/v1)
	mux.HandleFunc("GET /health", systemHandler.Health)
	mux.HandleFunc("GET /healthz", systemHandler.Health)
	mux.HandleFunc("GET /ready", systemHandler.Ready)
	mux.HandleFunc("GET /readyz", systemHandler.Ready)

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

	registerRoutes(mux, "/api/v1", cfg, jwtManager, loginLimiter, systemHandler, authHandler, allureHandler)

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
func awaitShutdown(ctx context.Context, srv *http.Server, watcher *runner.Watcher, jobManager *runner.JobManager, cfg *config.Config, limiterDone chan struct{}, logger *zap.Logger) {
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
) {
	auth := func(h http.HandlerFunc) http.HandlerFunc {
		return middleware.AuthMiddleware(cfg, jwtManager, false)(h)
	}
	rateLimit := middleware.RateLimitMiddleware(loginLimiter)

	// RBAC wrappers (REVIEW #9): auth + role enforcement.
	adminOnly := func(h http.HandlerFunc) http.HandlerFunc {
		return auth(middleware.RequireRole("admin")(h))
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
	mux.HandleFunc("GET "+prefix+"/projects", viewerUp(mutableCache(allure.GetProjects)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports", viewerUp(mutableCache(allure.GetReportHistory)))

	// Admin write endpoints — no-store.
	mux.HandleFunc("POST "+prefix+"/projects", adminOnly(noStore(allure.CreateProject)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}", adminOnly(noStore(allure.DeleteProject)))
	mux.HandleFunc("POST "+prefix+"/projects/{project_id}/reports", adminOnly(noStore(allure.GenerateReport)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/jobs/{job_id}", adminOnly(noStore(allure.GetJobStatus)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/reports/history", adminOnly(noStore(allure.CleanHistory)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/results", adminOnly(noStore(allure.CleanResults)))
	mux.HandleFunc("POST "+prefix+"/projects/{project_id}/results", adminOnly(noStore(allure.SendResults)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/reports/{report_id}", adminOnly(noStore(allure.DeleteReport)))

	// Report widget endpoints — dynamic cache (immutable for numbered builds, short-lived for latest).
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/categories", viewerUp(reportCache(allure.GetReportCategories)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/environment", viewerUp(reportCache(allure.GetReportEnvironment)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/known-failures", viewerUp(reportCache(allure.GetReportKnownFailures)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/timeline", viewerUp(reportCache(allure.GetReportTimeline)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/stability", viewerUp(reportCache(allure.GetReportStability)))
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/reports/{report_id}/summary", viewerUp(reportCache(allure.GetReportSummary)))

	// Known issues list — mutable cache (changes when issues are created/updated/deleted).
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/known-issues", viewerUp(mutableCache(allure.ListKnownIssues)))
	mux.HandleFunc("POST "+prefix+"/projects/{project_id}/known-issues", adminOnly(noStore(allure.CreateKnownIssue)))
	mux.HandleFunc("PUT "+prefix+"/projects/{project_id}/known-issues/{issue_id}", adminOnly(noStore(allure.UpdateKnownIssue)))
	mux.HandleFunc("DELETE "+prefix+"/projects/{project_id}/known-issues/{issue_id}", adminOnly(noStore(allure.DeleteKnownIssue)))

	// Analytics — short-lived cache.
	mux.HandleFunc("GET "+prefix+"/projects/{project_id}/analytics/low-performing", viewerUp(shortCache(allure.GetLowPerformingTests)))

	// Dashboard — short-lived cache.
	mux.HandleFunc("GET "+prefix+"/dashboard", viewerUp(shortCache(allure.GetDashboard)))
}
