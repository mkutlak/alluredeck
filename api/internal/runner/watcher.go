package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// Watcher manages the background polling of the results directories.
type Watcher struct {
	cfg          *config.Config
	allureCore   *Allure
	projectStore store.ProjectStorer
	store        storage.Store
	stop         chan struct{}
	wg           sync.WaitGroup
	logger       *zap.Logger
}

// NewWatcher creates a new Watcher
func NewWatcher(cfg *config.Config, allureCore *Allure, projectStore store.ProjectStorer, st storage.Store, logger *zap.Logger) *Watcher {
	return &Watcher{
		cfg:          cfg,
		allureCore:   allureCore,
		projectStore: projectStore,
		store:        st,
		stop:         make(chan struct{}),
		logger:       logger,
	}
}

// Start begins the polling loop in a background goroutine
func (w *Watcher) Start() {
	checkSecsStr := w.cfg.CheckResultsEverySeconds
	if strings.EqualFold(checkSecsStr, "NONE") || checkSecsStr == "" {
		w.logger.Info("background file watcher is disabled", zap.String("reason", "CHECK_RESULTS_EVERY_SECONDS=NONE"))
		return
	}

	interval, err := strconv.Atoi(checkSecsStr)
	if err != nil {
		w.logger.Warn("invalid CHECK_RESULTS_EVERY_SECONDS value, defaulting to 1s",
			zap.String("value", checkSecsStr))
		interval = 1
	}

	w.wg.Add(1)
	go w.watchLoop(time.Duration(interval) * time.Second)
}

// Stop gracefully shuts down the watcher
func (w *Watcher) Stop() {
	close(w.stop)
	w.wg.Wait()
}

func (w *Watcher) watchLoop(interval time.Duration) {
	defer w.wg.Done()
	w.logger.Info("starting background file watcher", zap.Duration("interval", interval))

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Track the content hash of each project's results directory (keyed by slug).
	projectHashes := make(map[string]string)

	for {
		select {
		case <-w.stop:
			w.logger.Info("stopping file watcher")
			return
		case <-ticker.C:
			w.checkProjects(projectHashes)
		}
	}
}

// checkProject checks one project for result changes and triggers report generation if needed.
// Returns the new hash for the project (empty string signals the project should be skipped).
// slug is the filesystem project identifier discovered by the storage layer.
func (w *Watcher) checkProject(ctx context.Context, slug, previousHash string, firstSeen bool) (newHash string, skip bool) {
	currentHash, err := w.store.ResultsDirHash(ctx, slug)
	if err != nil {
		w.logger.Error("watcher error hashing results",
			zap.String("slug", slug), zap.Error(err))
		return "", true
	}

	if firstSeen {
		// Initialize state on first pass without triggering report generation.
		return currentHash, false
	}

	// Trigger only when hash changed AND results dir is not empty.
	// currentHash == "" means S3 mode (no-op) or empty dir.
	if currentHash != previousHash && currentHash != "" && currentHash != emptyDirHash() {
		w.logger.Info("watcher detected changes, triggering report generation",
			zap.String("slug", slug))

		// Look up the numeric project ID from the database.
		proj, projErr := w.projectStore.GetProjectBySlugAny(ctx, slug)
		if projErr != nil {
			w.logger.Error("watcher failed to look up project",
				zap.String("slug", slug), zap.Error(projErr))
			return currentHash, false
		}

		// List pending batch directories and process the first one.
		// Remaining batches will be picked up on the next poll cycle.
		batches, batchErr := w.store.ListResultBatches(ctx, proj.StorageKey)
		if batchErr != nil {
			w.logger.Error("watcher failed to list result batches",
				zap.String("slug", slug), zap.Error(batchErr))
			return currentHash, false
		}
		if len(batches) == 0 {
			return currentHash, false
		}
		// Process first batch; remaining batches will be picked up on the next poll cycle.
		batchID := batches[0]
		if _, err := w.allureCore.GenerateReport(ctx, proj.ID, slug, proj.StorageKey, batchID, "", "", "", true, "", ""); err != nil {
			w.logger.Error("watcher failed to generate report",
				zap.String("slug", slug), zap.String("batch_id", batchID), zap.Error(err))
		}

	}

	return currentHash, false
}

func (w *Watcher) checkProjects(projectHashes map[string]string) {
	ctx := context.Background()

	slugs, err := w.store.ListProjects(ctx)
	if err != nil {
		w.logger.Error("watcher error listing projects", zap.Error(err))
		return
	}

	currentSlugs := make(map[string]bool)
	for _, slug := range slugs {
		currentSlugs[slug] = true

		// Auto-register in DB if discovered via filesystem (e.g. volume-mounted projects).
		if _, err := w.projectStore.GetProjectBySlugAny(ctx, slug); err != nil {
			if _, err := w.projectStore.CreateProject(ctx, slug); err != nil {
				w.logger.Error("watcher failed to register project in DB",
					zap.String("slug", slug), zap.Error(err))
			}
		}

		previousHash, firstSeen := projectHashes[slug]
		newHash, skip := w.checkProject(ctx, slug, previousHash, !firstSeen)
		if skip {
			continue
		}
		projectHashes[slug] = newHash
	}

	// Clean up removed projects from tracking map
	for slug := range projectHashes {
		if !currentSlugs[slug] {
			delete(projectHashes, slug)
		}
	}
}

// emptyDirHash returns the SHA256 hash of an empty input (no files to hash).
func emptyDirHash() string {
	h := sha256.New()
	return hex.EncodeToString(h.Sum(nil))
}
