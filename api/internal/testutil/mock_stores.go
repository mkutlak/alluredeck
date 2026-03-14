package testutil

import (
	"context"
	"time"

	parser "github.com/mkutlak/alluredeck/api/internal/parser"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// Compile-time interface checks.
var (
	_ store.ProjectStorer    = (*MockProjectStore)(nil)
	_ store.BuildStorer      = (*MockBuildStore)(nil)
	_ store.TestResultStorer = (*MockTestResultStore)(nil)
	_ store.KnownIssueStorer = (*MockKnownIssueStore)(nil)
	_ store.BlacklistStorer  = (*MockBlacklistStore)(nil)
	_ store.BranchStorer     = (*MockBranchStore)(nil)
	_ store.SearchStorer     = (*MockSearchStore)(nil)
	_ store.Locker           = (*MockLocker)(nil)
	_ store.APIKeyStorer     = (*MockAPIKeyStore)(nil)
)

// MockStores bundles all mock store implementations for easy test setup.
type MockStores struct {
	Projects    *MemProjectStore // stateful in-memory project store
	Builds      *MockBuildStore  // zero-value mock; set Fn fields for fine-grained control
	MemBuilds   *MemBuildStore   // stateful in-memory build store for handler tests
	TestResults *MockTestResultStore
	KnownIssues *MemKnownIssueStore // stateful in-memory store; supports Create/List/Get/Update/Delete
	Blacklist   *MockBlacklistStore
	Branches    *MockBranchStore
	Search      *MockSearchStore
	Locker      *MockLocker
	APIKeys     *MemAPIKeyStore // stateful in-memory store for API key handler tests
}

// New returns a MockStores with all fields initialised.
// KnownIssues and MemBuilds use stateful in-memory implementations so that
// handler tests can create records and immediately query them back.
func New() *MockStores {
	return &MockStores{
		Projects:    NewMemProjectStore(),
		Builds:      &MockBuildStore{},
		MemBuilds:   NewMemBuildStore(),
		TestResults: &MockTestResultStore{},
		KnownIssues: NewMemKnownIssueStore(),
		Blacklist:   &MockBlacklistStore{},
		Branches:    &MockBranchStore{},
		Search:      &MockSearchStore{},
		Locker:      &MockLocker{},
		APIKeys:     NewMemAPIKeyStore(),
	}
}

// ---------------------------------------------------------------------------
// MockProjectStore
// ---------------------------------------------------------------------------

// MockProjectStore is a test double for store.ProjectStorer.
// Set function fields to control behaviour; unset fields return zero values.
type MockProjectStore struct {
	CreateProjectFn         func(ctx context.Context, id string) error
	GetProjectFn            func(ctx context.Context, id string) (*store.Project, error)
	ListProjectsFn          func(ctx context.Context) ([]store.Project, error)
	ListProjectsPaginatedFn func(ctx context.Context, page, perPage int, tag string) ([]store.Project, int, error)
	ListAllTagsFn           func(ctx context.Context) ([]string, error)
	SetTagsFn               func(ctx context.Context, projectID string, tags []string) error
	DeleteProjectFn         func(ctx context.Context, id string) error
	ProjectExistsFn         func(ctx context.Context, id string) (bool, error)
}

func (m *MockProjectStore) CreateProject(ctx context.Context, id string) error {
	if m.CreateProjectFn != nil {
		return m.CreateProjectFn(ctx, id)
	}
	return nil
}

func (m *MockProjectStore) GetProject(ctx context.Context, id string) (*store.Project, error) {
	if m.GetProjectFn != nil {
		return m.GetProjectFn(ctx, id)
	}
	return nil, nil
}

func (m *MockProjectStore) ListProjects(ctx context.Context) ([]store.Project, error) {
	if m.ListProjectsFn != nil {
		return m.ListProjectsFn(ctx)
	}
	return nil, nil
}

func (m *MockProjectStore) ListProjectsPaginated(ctx context.Context, page, perPage int, tag string) ([]store.Project, int, error) {
	if m.ListProjectsPaginatedFn != nil {
		return m.ListProjectsPaginatedFn(ctx, page, perPage, tag)
	}
	return nil, 0, nil
}

func (m *MockProjectStore) ListAllTags(ctx context.Context) ([]string, error) {
	if m.ListAllTagsFn != nil {
		return m.ListAllTagsFn(ctx)
	}
	return nil, nil
}

func (m *MockProjectStore) SetTags(ctx context.Context, projectID string, tags []string) error {
	if m.SetTagsFn != nil {
		return m.SetTagsFn(ctx, projectID, tags)
	}
	return nil
}

func (m *MockProjectStore) DeleteProject(ctx context.Context, id string) error {
	if m.DeleteProjectFn != nil {
		return m.DeleteProjectFn(ctx, id)
	}
	return nil
}

func (m *MockProjectStore) ProjectExists(ctx context.Context, id string) (bool, error) {
	if m.ProjectExistsFn != nil {
		return m.ProjectExistsFn(ctx, id)
	}
	return false, nil
}

// ---------------------------------------------------------------------------
// MockBuildStore
// ---------------------------------------------------------------------------

// MockBuildStore is a test double for store.BuildStorer.
type MockBuildStore struct {
	NextBuildOrderFn            func(ctx context.Context, projectID string) (int, error)
	InsertBuildFn               func(ctx context.Context, projectID string, buildOrder int) error
	UpdateBuildStatsFn          func(ctx context.Context, projectID string, buildOrder int, stats store.BuildStats) error
	UpdateBuildCIMetadataFn     func(ctx context.Context, projectID string, buildOrder int, ciMeta store.CIMetadata) error
	GetBuildByOrderFn           func(ctx context.Context, projectID string, buildOrder int) (store.Build, error)
	GetPreviousBuildFn          func(ctx context.Context, projectID string, buildOrder int) (store.Build, error)
	GetLatestBuildFn            func(ctx context.Context, projectID string) (store.Build, error)
	ListBuildsFn                func(ctx context.Context, projectID string) ([]store.Build, error)
	ListBuildsPaginatedFn       func(ctx context.Context, projectID string, page, perPage int) ([]store.Build, int, error)
	PruneBuildsFn               func(ctx context.Context, projectID string, keep int) ([]int, error)
	SetLatestFn                 func(ctx context.Context, projectID string, buildOrder int) error
	DeleteAllBuildsFn           func(ctx context.Context, projectID string) error
	GetDashboardDataFn          func(ctx context.Context, sparklineDepth int, tag string) ([]store.DashboardProject, error)
	DeleteBuildFn               func(ctx context.Context, projectID string, buildOrder int) error
	UpdateBuildBranchIDFn       func(ctx context.Context, projectID string, buildOrder int, branchID int64) error
	SetLatestBranchFn           func(ctx context.Context, projectID string, buildOrder int, branchID *int64) error
	PruneBuildsBranchFn         func(ctx context.Context, projectID string, keep int, branchID *int64) ([]int, error)
	ListBuildsPaginatedBranchFn func(ctx context.Context, projectID string, page, perPage int, branchID *int64) ([]store.Build, int, error)
}

func (m *MockBuildStore) NextBuildOrder(ctx context.Context, projectID string) (int, error) {
	if m.NextBuildOrderFn != nil {
		return m.NextBuildOrderFn(ctx, projectID)
	}
	return 0, nil
}

func (m *MockBuildStore) InsertBuild(ctx context.Context, projectID string, buildOrder int) error {
	if m.InsertBuildFn != nil {
		return m.InsertBuildFn(ctx, projectID, buildOrder)
	}
	return nil
}

func (m *MockBuildStore) UpdateBuildStats(ctx context.Context, projectID string, buildOrder int, stats store.BuildStats) error {
	if m.UpdateBuildStatsFn != nil {
		return m.UpdateBuildStatsFn(ctx, projectID, buildOrder, stats)
	}
	return nil
}

func (m *MockBuildStore) UpdateBuildCIMetadata(ctx context.Context, projectID string, buildOrder int, ciMeta store.CIMetadata) error {
	if m.UpdateBuildCIMetadataFn != nil {
		return m.UpdateBuildCIMetadataFn(ctx, projectID, buildOrder, ciMeta)
	}
	return nil
}

func (m *MockBuildStore) GetBuildByOrder(ctx context.Context, projectID string, buildOrder int) (store.Build, error) {
	if m.GetBuildByOrderFn != nil {
		return m.GetBuildByOrderFn(ctx, projectID, buildOrder)
	}
	return store.Build{}, nil
}

func (m *MockBuildStore) GetPreviousBuild(ctx context.Context, projectID string, buildOrder int) (store.Build, error) {
	if m.GetPreviousBuildFn != nil {
		return m.GetPreviousBuildFn(ctx, projectID, buildOrder)
	}
	return store.Build{}, nil
}

func (m *MockBuildStore) GetLatestBuild(ctx context.Context, projectID string) (store.Build, error) {
	if m.GetLatestBuildFn != nil {
		return m.GetLatestBuildFn(ctx, projectID)
	}
	return store.Build{}, nil
}

func (m *MockBuildStore) ListBuilds(ctx context.Context, projectID string) ([]store.Build, error) {
	if m.ListBuildsFn != nil {
		return m.ListBuildsFn(ctx, projectID)
	}
	return nil, nil
}

func (m *MockBuildStore) ListBuildsPaginated(ctx context.Context, projectID string, page, perPage int) ([]store.Build, int, error) {
	if m.ListBuildsPaginatedFn != nil {
		return m.ListBuildsPaginatedFn(ctx, projectID, page, perPage)
	}
	return nil, 0, nil
}

func (m *MockBuildStore) PruneBuilds(ctx context.Context, projectID string, keep int) ([]int, error) {
	if m.PruneBuildsFn != nil {
		return m.PruneBuildsFn(ctx, projectID, keep)
	}
	return nil, nil
}

func (m *MockBuildStore) SetLatest(ctx context.Context, projectID string, buildOrder int) error {
	if m.SetLatestFn != nil {
		return m.SetLatestFn(ctx, projectID, buildOrder)
	}
	return nil
}

func (m *MockBuildStore) DeleteAllBuilds(ctx context.Context, projectID string) error {
	if m.DeleteAllBuildsFn != nil {
		return m.DeleteAllBuildsFn(ctx, projectID)
	}
	return nil
}

func (m *MockBuildStore) GetDashboardData(ctx context.Context, sparklineDepth int, tag string) ([]store.DashboardProject, error) {
	if m.GetDashboardDataFn != nil {
		return m.GetDashboardDataFn(ctx, sparklineDepth, tag)
	}
	return nil, nil
}

func (m *MockBuildStore) DeleteBuild(ctx context.Context, projectID string, buildOrder int) error {
	if m.DeleteBuildFn != nil {
		return m.DeleteBuildFn(ctx, projectID, buildOrder)
	}
	return nil
}

func (m *MockBuildStore) UpdateBuildBranchID(ctx context.Context, projectID string, buildOrder int, branchID int64) error {
	if m.UpdateBuildBranchIDFn != nil {
		return m.UpdateBuildBranchIDFn(ctx, projectID, buildOrder, branchID)
	}
	return nil
}

func (m *MockBuildStore) SetLatestBranch(ctx context.Context, projectID string, buildOrder int, branchID *int64) error {
	if m.SetLatestBranchFn != nil {
		return m.SetLatestBranchFn(ctx, projectID, buildOrder, branchID)
	}
	return nil
}

func (m *MockBuildStore) PruneBuildsBranch(ctx context.Context, projectID string, keep int, branchID *int64) ([]int, error) {
	if m.PruneBuildsBranchFn != nil {
		return m.PruneBuildsBranchFn(ctx, projectID, keep, branchID)
	}
	return nil, nil
}

func (m *MockBuildStore) ListBuildsPaginatedBranch(ctx context.Context, projectID string, page, perPage int, branchID *int64) ([]store.Build, int, error) {
	if m.ListBuildsPaginatedBranchFn != nil {
		return m.ListBuildsPaginatedBranchFn(ctx, projectID, page, perPage, branchID)
	}
	return nil, 0, nil
}

// ---------------------------------------------------------------------------
// MockTestResultStore
// ---------------------------------------------------------------------------

// MockTestResultStore is a test double for store.TestResultStorer.
type MockTestResultStore struct {
	InsertBatchFn              func(ctx context.Context, results []store.TestResult) error
	InsertBatchFullFn          func(ctx context.Context, buildID int64, projectID string, results []*parser.Result) error
	GetBuildIDFn               func(ctx context.Context, projectID string, buildOrder int) (int64, error)
	ListSlowestFn              func(ctx context.Context, projectID string, builds, limit int) ([]store.LowPerformingTest, error)
	ListLeastReliableFn        func(ctx context.Context, projectID string, builds, limit int) ([]store.LowPerformingTest, error)
	ListTimelineFn             func(ctx context.Context, projectID string, buildID int64, limit int) ([]store.TimelineRow, error)
	ListFailedByBuildFn        func(ctx context.Context, projectID string, buildID int64, limit int) ([]store.TestResult, error)
	GetTestHistoryFn           func(ctx context.Context, projectID, historyID string, branchID *int64, limit int) ([]store.TestHistoryEntry, error)
	DeleteByBuildFn            func(ctx context.Context, buildID int64) error
	DeleteByProjectFn          func(ctx context.Context, projectID string) error
	CompareBuildsByHistoryIDFn func(ctx context.Context, projectID string, buildIDA, buildIDB int64) ([]store.DiffEntry, error)
}

func (m *MockTestResultStore) InsertBatch(ctx context.Context, results []store.TestResult) error {
	if m.InsertBatchFn != nil {
		return m.InsertBatchFn(ctx, results)
	}
	return nil
}

func (m *MockTestResultStore) InsertBatchFull(ctx context.Context, buildID int64, projectID string, results []*parser.Result) error {
	if m.InsertBatchFullFn != nil {
		return m.InsertBatchFullFn(ctx, buildID, projectID, results)
	}
	return nil
}

func (m *MockTestResultStore) GetBuildID(ctx context.Context, projectID string, buildOrder int) (int64, error) {
	if m.GetBuildIDFn != nil {
		return m.GetBuildIDFn(ctx, projectID, buildOrder)
	}
	return 0, nil
}

func (m *MockTestResultStore) ListSlowest(ctx context.Context, projectID string, builds, limit int) ([]store.LowPerformingTest, error) {
	if m.ListSlowestFn != nil {
		return m.ListSlowestFn(ctx, projectID, builds, limit)
	}
	return nil, nil
}

func (m *MockTestResultStore) ListLeastReliable(ctx context.Context, projectID string, builds, limit int) ([]store.LowPerformingTest, error) {
	if m.ListLeastReliableFn != nil {
		return m.ListLeastReliableFn(ctx, projectID, builds, limit)
	}
	return nil, nil
}

func (m *MockTestResultStore) ListTimeline(ctx context.Context, projectID string, buildID int64, limit int) ([]store.TimelineRow, error) {
	if m.ListTimelineFn != nil {
		return m.ListTimelineFn(ctx, projectID, buildID, limit)
	}
	return nil, nil
}

func (m *MockTestResultStore) ListFailedByBuild(ctx context.Context, projectID string, buildID int64, limit int) ([]store.TestResult, error) {
	if m.ListFailedByBuildFn != nil {
		return m.ListFailedByBuildFn(ctx, projectID, buildID, limit)
	}
	return nil, nil
}

func (m *MockTestResultStore) GetTestHistory(ctx context.Context, projectID, historyID string, branchID *int64, limit int) ([]store.TestHistoryEntry, error) {
	if m.GetTestHistoryFn != nil {
		return m.GetTestHistoryFn(ctx, projectID, historyID, branchID, limit)
	}
	return nil, nil
}

func (m *MockTestResultStore) DeleteByBuild(ctx context.Context, buildID int64) error {
	if m.DeleteByBuildFn != nil {
		return m.DeleteByBuildFn(ctx, buildID)
	}
	return nil
}

func (m *MockTestResultStore) DeleteByProject(ctx context.Context, projectID string) error {
	if m.DeleteByProjectFn != nil {
		return m.DeleteByProjectFn(ctx, projectID)
	}
	return nil
}

func (m *MockTestResultStore) CompareBuildsByHistoryID(ctx context.Context, projectID string, buildIDA, buildIDB int64) ([]store.DiffEntry, error) {
	if m.CompareBuildsByHistoryIDFn != nil {
		return m.CompareBuildsByHistoryIDFn(ctx, projectID, buildIDA, buildIDB)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// MockKnownIssueStore
// ---------------------------------------------------------------------------

// MockKnownIssueStore is a test double for store.KnownIssueStorer.
type MockKnownIssueStore struct {
	CreateFn        func(ctx context.Context, projectID, testName, pattern, ticketURL, description string) (*store.KnownIssue, error)
	GetFn           func(ctx context.Context, id int64) (*store.KnownIssue, error)
	ListFn          func(ctx context.Context, projectID string, activeOnly bool) ([]store.KnownIssue, error)
	ListPaginatedFn func(ctx context.Context, projectID string, activeOnly bool, page, perPage int) ([]store.KnownIssue, int, error)
	UpdateFn        func(ctx context.Context, id int64, projectID, ticketURL, description string, isActive bool) error
	DeleteFn        func(ctx context.Context, id int64, projectID string) error
	IsKnownFn       func(ctx context.Context, projectID, testName string) (bool, error)
}

func (m *MockKnownIssueStore) Create(ctx context.Context, projectID, testName, pattern, ticketURL, description string) (*store.KnownIssue, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, projectID, testName, pattern, ticketURL, description)
	}
	return nil, nil
}

func (m *MockKnownIssueStore) Get(ctx context.Context, id int64) (*store.KnownIssue, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, id)
	}
	return nil, nil
}

func (m *MockKnownIssueStore) List(ctx context.Context, projectID string, activeOnly bool) ([]store.KnownIssue, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, projectID, activeOnly)
	}
	return nil, nil
}

func (m *MockKnownIssueStore) ListPaginated(ctx context.Context, projectID string, activeOnly bool, page, perPage int) ([]store.KnownIssue, int, error) {
	if m.ListPaginatedFn != nil {
		return m.ListPaginatedFn(ctx, projectID, activeOnly, page, perPage)
	}
	return nil, 0, nil
}

func (m *MockKnownIssueStore) Update(ctx context.Context, id int64, projectID, ticketURL, description string, isActive bool) error {
	if m.UpdateFn != nil {
		return m.UpdateFn(ctx, id, projectID, ticketURL, description, isActive)
	}
	return nil
}

func (m *MockKnownIssueStore) Delete(ctx context.Context, id int64, projectID string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id, projectID)
	}
	return nil
}

func (m *MockKnownIssueStore) IsKnown(ctx context.Context, projectID, testName string) (bool, error) {
	if m.IsKnownFn != nil {
		return m.IsKnownFn(ctx, projectID, testName)
	}
	return false, nil
}

// ---------------------------------------------------------------------------
// MockBlacklistStore
// ---------------------------------------------------------------------------

// MockBlacklistStore is a test double for store.BlacklistStorer.
type MockBlacklistStore struct {
	AddToBlacklistFn func(ctx context.Context, jti string, expiresAt time.Time) error
	IsBlacklistedFn  func(ctx context.Context, jti string) (bool, error)
	PruneExpiredFn   func(ctx context.Context) (int64, error)
}

func (m *MockBlacklistStore) AddToBlacklist(ctx context.Context, jti string, expiresAt time.Time) error {
	if m.AddToBlacklistFn != nil {
		return m.AddToBlacklistFn(ctx, jti, expiresAt)
	}
	return nil
}

func (m *MockBlacklistStore) IsBlacklisted(ctx context.Context, jti string) (bool, error) {
	if m.IsBlacklistedFn != nil {
		return m.IsBlacklistedFn(ctx, jti)
	}
	return false, nil
}

func (m *MockBlacklistStore) PruneExpired(ctx context.Context) (int64, error) {
	if m.PruneExpiredFn != nil {
		return m.PruneExpiredFn(ctx)
	}
	return 0, nil
}

// ---------------------------------------------------------------------------
// MockBranchStore
// ---------------------------------------------------------------------------

// MockBranchStore is a test double for store.BranchStorer.
type MockBranchStore struct {
	GetOrCreateFn func(ctx context.Context, projectID, name string) (*store.Branch, bool, error)
	ListFn        func(ctx context.Context, projectID string) ([]store.Branch, error)
	GetDefaultFn  func(ctx context.Context, projectID string) (*store.Branch, error)
	SetDefaultFn  func(ctx context.Context, projectID string, branchID int64) error
	DeleteFn      func(ctx context.Context, projectID string, branchID int64) error
	GetByNameFn   func(ctx context.Context, projectID, name string) (*store.Branch, error)
}

func (m *MockBranchStore) GetOrCreate(ctx context.Context, projectID, name string) (*store.Branch, bool, error) {
	if m.GetOrCreateFn != nil {
		return m.GetOrCreateFn(ctx, projectID, name)
	}
	return nil, false, nil
}

func (m *MockBranchStore) List(ctx context.Context, projectID string) ([]store.Branch, error) {
	if m.ListFn != nil {
		return m.ListFn(ctx, projectID)
	}
	return nil, nil
}

func (m *MockBranchStore) GetDefault(ctx context.Context, projectID string) (*store.Branch, error) {
	if m.GetDefaultFn != nil {
		return m.GetDefaultFn(ctx, projectID)
	}
	return nil, nil
}

func (m *MockBranchStore) SetDefault(ctx context.Context, projectID string, branchID int64) error {
	if m.SetDefaultFn != nil {
		return m.SetDefaultFn(ctx, projectID, branchID)
	}
	return nil
}

func (m *MockBranchStore) Delete(ctx context.Context, projectID string, branchID int64) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, projectID, branchID)
	}
	return nil
}

func (m *MockBranchStore) GetByName(ctx context.Context, projectID, name string) (*store.Branch, error) {
	if m.GetByNameFn != nil {
		return m.GetByNameFn(ctx, projectID, name)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// MockSearchStore
// ---------------------------------------------------------------------------

// MockSearchStore is a test double for store.SearchStorer.
type MockSearchStore struct {
	SearchProjectsFn func(ctx context.Context, query string, limit int) ([]store.ProjectMatch, error)
	SearchTestsFn    func(ctx context.Context, query string, limit int) ([]store.TestMatch, error)
}

func (m *MockSearchStore) SearchProjects(ctx context.Context, query string, limit int) ([]store.ProjectMatch, error) {
	if m.SearchProjectsFn != nil {
		return m.SearchProjectsFn(ctx, query, limit)
	}
	return nil, nil
}

func (m *MockSearchStore) SearchTests(ctx context.Context, query string, limit int) ([]store.TestMatch, error) {
	if m.SearchTestsFn != nil {
		return m.SearchTestsFn(ctx, query, limit)
	}
	return nil, nil
}

// ---------------------------------------------------------------------------
// MockLocker
// ---------------------------------------------------------------------------

// MockLocker is a test double for store.Locker.
// When AcquireLockFn is nil, returns a no-op unlock function and nil error.
type MockLocker struct {
	AcquireLockFn func(ctx context.Context, key string) (func(), error)
}

func (m *MockLocker) AcquireLock(ctx context.Context, key string) (func(), error) {
	if m.AcquireLockFn != nil {
		return m.AcquireLockFn(ctx, key)
	}
	return func() {}, nil
}

// ---------------------------------------------------------------------------
// MockAPIKeyStore
// ---------------------------------------------------------------------------

// MockAPIKeyStore is a test double for store.APIKeyStorer.
// Set function fields to control behaviour; unset fields return zero values.
type MockAPIKeyStore struct {
	CreateFn          func(ctx context.Context, key *store.APIKey) (*store.APIKey, error)
	ListByUsernameFn  func(ctx context.Context, username string) ([]store.APIKey, error)
	GetByHashFn       func(ctx context.Context, keyHash string) (*store.APIKey, error)
	UpdateLastUsedFn  func(ctx context.Context, id int64) error
	DeleteFn          func(ctx context.Context, id int64, username string) error
	CountByUsernameFn func(ctx context.Context, username string) (int, error)
}

func (m *MockAPIKeyStore) Create(ctx context.Context, key *store.APIKey) (*store.APIKey, error) {
	if m.CreateFn != nil {
		return m.CreateFn(ctx, key)
	}
	return nil, nil
}

func (m *MockAPIKeyStore) ListByUsername(ctx context.Context, username string) ([]store.APIKey, error) {
	if m.ListByUsernameFn != nil {
		return m.ListByUsernameFn(ctx, username)
	}
	return nil, nil
}

func (m *MockAPIKeyStore) GetByHash(ctx context.Context, keyHash string) (*store.APIKey, error) {
	if m.GetByHashFn != nil {
		return m.GetByHashFn(ctx, keyHash)
	}
	return nil, nil
}

func (m *MockAPIKeyStore) UpdateLastUsed(ctx context.Context, id int64) error {
	if m.UpdateLastUsedFn != nil {
		return m.UpdateLastUsedFn(ctx, id)
	}
	return nil
}

func (m *MockAPIKeyStore) Delete(ctx context.Context, id int64, username string) error {
	if m.DeleteFn != nil {
		return m.DeleteFn(ctx, id, username)
	}
	return nil
}

func (m *MockAPIKeyStore) CountByUsername(ctx context.Context, username string) (int, error) {
	if m.CountByUsernameFn != nil {
		return m.CountByUsernameFn(ctx, username)
	}
	return 0, nil
}
