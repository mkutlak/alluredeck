package runner

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/config"
	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/storage"
	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/store"
)

// Watcher manages the background polling of the results directories.
// It replaces checkAllureResultsFiles.sh
type Watcher struct {
	cfg          *config.Config
	allureCore   *Allure
	projectStore *store.ProjectStore
	store        storage.Store
	stop         chan struct{}
	wg           sync.WaitGroup
}

// NewWatcher creates a new Watcher
func NewWatcher(cfg *config.Config, allureCore *Allure, projectStore *store.ProjectStore, st storage.Store) *Watcher {
	return &Watcher{
		cfg:          cfg,
		allureCore:   allureCore,
		projectStore: projectStore,
		store:        st,
		stop:         make(chan struct{}),
	}
}

// Start begins the polling loop in a background goroutine
func (w *Watcher) Start() {
	checkSecsStr := w.cfg.CheckResultsSecs
	if strings.EqualFold(checkSecsStr, "NONE") || checkSecsStr == "" {
		log.Println("Background file watcher is disabled (CHECK_RESULTS_EVERY_SECONDS=NONE)")
		return
	}

	interval, err := strconv.Atoi(checkSecsStr)
	if err != nil {
		log.Printf("Invalid CHECK_RESULTS_EVERY_SECONDS value: %s, defaulting to 1s", checkSecsStr)
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
	log.Printf("Starting background file watcher (Interval: %v)", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Track the content hash of each project's results directory
	projectHashes := make(map[string]string)

	for {
		select {
		case <-w.stop:
			log.Println("Stopping file watcher...")
			return
		case <-ticker.C:
			w.checkProjects(projectHashes)
		}
	}
}

// checkProject checks one project for result changes and triggers report generation if needed.
// Returns the new hash for the project (empty string signals the project should be skipped).
func (w *Watcher) checkProject(ctx context.Context, projectID, previousHash string, firstSeen bool) (newHash string, skip bool) {
	currentHash, err := w.store.ResultsDirHash(ctx, projectID)
	if err != nil {
		log.Printf("Watcher error hashing results for %s: %v", projectID, err)
		return "", true
	}

	if firstSeen {
		// Initialize state on first pass without triggering report generation.
		return currentHash, false
	}

	// Trigger only when hash changed AND results dir is not empty.
	// currentHash == "" means S3 mode (no-op) or empty dir.
	if currentHash != previousHash && currentHash != "" && currentHash != emptyDirHash() {
		log.Printf("Watcher detected changes in '%s' results. Triggering report generation...", projectID)

		// Bug 1 fix: call KeepHistory (preserves history) instead of CleanHistory (deletes all history).
		if err := w.allureCore.KeepHistory(projectID); err != nil {
			log.Printf("Watcher failed to keep history for '%s': %v", projectID, err)
		}

		// Bug 4 fix: storeResults=true so numbered history dirs (reports/1/, reports/2/, …) are created.
		if _, err := w.allureCore.GenerateReport(projectID, "", "", "", true); err != nil {
			log.Printf("Watcher failed to generate report for '%s': %v", projectID, err)
		}

		if _, err := w.allureCore.RenderEmailableReport(projectID); err != nil {
			log.Printf("Watcher failed to render emailable report for '%s': %v", projectID, err)
		}
	}

	return currentHash, false
}

func (w *Watcher) checkProjects(projectHashes map[string]string) {
	ctx := context.Background()

	projects, err := w.store.ListProjects(ctx)
	if err != nil {
		log.Printf("Watcher error listing projects: %v", err)
		return
	}

	currentProjects := make(map[string]bool)
	for _, projectID := range projects {
		currentProjects[projectID] = true

		// Auto-register in DB if discovered via filesystem (e.g. volume-mounted projects).
		if exists, err := w.projectStore.ProjectExists(ctx, projectID); err == nil && !exists {
			if err := w.projectStore.CreateProject(ctx, projectID); err != nil {
				log.Printf("Watcher: failed to register project '%s' in DB: %v", projectID, err)
			}
		}

		previousHash, firstSeen := projectHashes[projectID]
		newHash, skip := w.checkProject(ctx, projectID, previousHash, !firstSeen)
		if skip {
			continue
		}
		projectHashes[projectID] = newHash
	}

	// Clean up removed projects from tracking map
	for id := range projectHashes {
		if !currentProjects[id] {
			delete(projectHashes, id)
		}
	}
}

// emptyDirHash returns the SHA256 hash of an empty input (no files to hash).
func emptyDirHash() string {
	h := sha256.New()
	return hex.EncodeToString(h.Sum(nil))
}
