package handlers

import (
	"net/http"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// AllureHandler handles HTTP requests for Allure report management.
type AllureHandler struct {
	cfg             *config.Config
	runner          *runner.Allure
	jobManager      runner.JobQueuer
	projectStore    store.ProjectStorer
	buildStore      store.BuildStorer
	knownIssueStore store.KnownIssueStorer
	testResultStore store.TestResultStorer
	searchStore     store.SearchStorer
	store           storage.Store
	branchStore     store.BranchStorer
	logger          *zap.Logger
}

// NewAllureHandler creates and returns a new AllureHandler.
func NewAllureHandler(cfg *config.Config, r *runner.Allure, jobManager runner.JobQueuer, projectStore store.ProjectStorer, buildStore store.BuildStorer, knownIssueStore store.KnownIssueStorer, testResultStore store.TestResultStorer, searchStore store.SearchStorer, st storage.Store, logger *zap.Logger) *AllureHandler {
	return &AllureHandler{
		cfg:             cfg,
		runner:          r,
		jobManager:      jobManager,
		projectStore:    projectStore,
		buildStore:      buildStore,
		knownIssueStore: knownIssueStore,
		testResultStore: testResultStore,
		searchStore:     searchStore,
		store:           st,
		logger:          logger,
	}
}

// SetBranchStore configures an optional branch store for branch-aware filtering.
func (h *AllureHandler) SetBranchStore(bs store.BranchStorer) {
	h.branchStore = bs
}

// NOTE: reservedProjectNames, validateProjectID, safeProjectID, and the
// package-level extractProjectID function are defined in project_id.go.
// Types, errors, and validation helpers are defined in types.go, errors.go, and project_id.go.

// extractProjectID delegates to the package-level extractProjectID using the
// handler's configured projects directory.
func (h *AllureHandler) extractProjectID(w http.ResponseWriter, r *http.Request) (string, bool) {
	return extractProjectID(w, r, h.cfg.ProjectsPath)
}

