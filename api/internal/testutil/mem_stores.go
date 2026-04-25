package testutil

import (
	"context"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// ---------------------------------------------------------------------------
// MemAPIKeyStore
// ---------------------------------------------------------------------------

// MemAPIKeyStore is a thread-safe in-memory APIKeyStorer for tests.
type MemAPIKeyStore struct {
	mu     sync.RWMutex
	keys   []*store.APIKey
	nextID int64
}

var _ store.APIKeyStorer = (*MemAPIKeyStore)(nil)

// NewMemAPIKeyStore returns an initialised MemAPIKeyStore.
func NewMemAPIKeyStore() *MemAPIKeyStore {
	return &MemAPIKeyStore{nextID: 1}
}

func (m *MemAPIKeyStore) Create(ctx context.Context, key *store.APIKey) (*store.APIKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	cp := *key
	cp.ID = m.nextID
	cp.CreatedAt = now
	m.nextID++
	m.keys = append(m.keys, &cp)
	result := cp
	return &result, nil
}

func (m *MemAPIKeyStore) ListByUsername(ctx context.Context, username string) ([]store.APIKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []store.APIKey
	for _, k := range m.keys {
		if k.Username == username {
			out = append(out, *k)
		}
	}
	// Sort newest first (descending created_at).
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out, nil
}

func (m *MemAPIKeyStore) GetByHash(ctx context.Context, keyHash string) (*store.APIKey, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, k := range m.keys {
		if k.KeyHash == keyHash {
			cp := *k
			return &cp, nil
		}
	}
	return nil, store.ErrAPIKeyNotFound
}

func (m *MemAPIKeyStore) UpdateLastUsed(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, k := range m.keys {
		if k.ID == id {
			now := time.Now()
			k.LastUsed = &now
			return nil
		}
	}
	return nil
}

func (m *MemAPIKeyStore) Delete(ctx context.Context, id int64, username string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, k := range m.keys {
		if k.ID == id && k.Username == username {
			m.keys = slices.Delete(m.keys, i, i+1)
			return nil
		}
	}
	return store.ErrAPIKeyNotFound
}

func (m *MemAPIKeyStore) CountByUsername(ctx context.Context, username string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	count := 0
	for _, k := range m.keys {
		if k.Username == username {
			count++
		}
	}
	return count, nil
}

// ---------------------------------------------------------------------------
// MemProjectStore
// ---------------------------------------------------------------------------

// MemProjectStore is a thread-safe in-memory ProjectStorer for tests.
type MemProjectStore struct {
	mu       sync.RWMutex
	projects map[int64]*store.Project // keyed by project ID
	slugMap  map[string]int64         // slug -> id for standalone lookups
	nextID   int64
}

var _ store.ProjectStorer = (*MemProjectStore)(nil)

// NewMemProjectStore returns an initialised MemProjectStore.
func NewMemProjectStore() *MemProjectStore {
	return &MemProjectStore{
		projects: make(map[int64]*store.Project),
		slugMap:  make(map[string]int64),
		nextID:   1,
	}
}

func (m *MemProjectStore) CreateProject(ctx context.Context, slug string) (*store.Project, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.slugMap[slug]; ok {
		return nil, store.ErrProjectExists
	}
	id := m.nextID
	m.nextID++
	p := &store.Project{ID: id, Slug: slug, StorageKey: slug, DisplayName: slug, CreatedAt: time.Now()}
	m.projects[id] = p
	m.slugMap[slug] = id
	cp := *p
	return &cp, nil
}

func (m *MemProjectStore) CreateProjectWithParent(ctx context.Context, slug string, parentID int64) (*store.Project, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.slugMap[slug]; ok {
		return nil, store.ErrProjectExists
	}
	id := m.nextID
	m.nextID++
	pid := parentID
	p := &store.Project{ID: id, Slug: slug, StorageKey: strconv.FormatInt(id, 10), ParentID: &pid, DisplayName: slug, CreatedAt: time.Now()}
	m.projects[id] = p
	m.slugMap[slug] = id
	cp := *p
	return &cp, nil
}

func (m *MemProjectStore) GetProject(ctx context.Context, id int64) (*store.Project, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	p, ok := m.projects[id]
	if !ok {
		return nil, store.ErrProjectNotFound
	}
	cp := *p
	return &cp, nil
}

func (m *MemProjectStore) GetProjectBySlug(ctx context.Context, slug string) (*store.Project, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.slugMap[slug]
	if !ok {
		return nil, store.ErrProjectNotFound
	}
	p := m.projects[id]
	// Only return top-level projects (parent_id IS NULL).
	if p.ParentID != nil {
		return nil, store.ErrProjectNotFound
	}
	cp := *p
	return &cp, nil
}

func (m *MemProjectStore) GetProjectBySlugAny(ctx context.Context, slug string) (*store.Project, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.slugMap[slug]
	if !ok {
		return nil, store.ErrProjectNotFound
	}
	p := m.projects[id]
	cp := *p
	return &cp, nil
}

func (m *MemProjectStore) ListProjects(ctx context.Context) ([]store.Project, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]store.Project, 0, len(m.projects))
	for _, p := range m.projects {
		out = append(out, *p)
	}
	return out, nil
}

func (m *MemProjectStore) ListProjectsPaginated(ctx context.Context, page, perPage int) ([]store.Project, int, error) {
	all, err := m.ListProjects(ctx)
	if err != nil {
		return nil, 0, err
	}
	total := len(all)
	start := (page - 1) * perPage
	if start >= total {
		return []store.Project{}, total, nil
	}
	return all[start:min(start+perPage, total)], total, nil
}

func (m *MemProjectStore) DeleteProject(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[id]
	if !ok {
		return store.ErrProjectNotFound
	}
	delete(m.slugMap, p.Slug)
	delete(m.projects, id)
	return nil
}

func (m *MemProjectStore) RenameProject(ctx context.Context, id int64, newSlug string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[id]
	if !ok {
		return store.ErrProjectNotFound
	}
	if existingID, exists := m.slugMap[newSlug]; exists && existingID != id {
		return store.ErrProjectExists
	}
	delete(m.slugMap, p.Slug)
	p.Slug = newSlug
	p.DisplayName = newSlug
	m.slugMap[newSlug] = id
	return nil
}

func (m *MemProjectStore) ProjectExists(ctx context.Context, id int64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.projects[id]
	return ok, nil
}

func (m *MemProjectStore) SetReportType(_ context.Context, id int64, reportType string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[id]
	if !ok {
		return store.ErrProjectNotFound
	}
	p.ReportType = reportType
	return nil
}

func (m *MemProjectStore) ListProjectsPaginatedTopLevel(ctx context.Context, page, perPage int) ([]store.Project, int, error) {
	all, err := m.ListProjects(ctx)
	if err != nil {
		return nil, 0, err
	}
	var topLevel []store.Project
	for _, p := range all {
		if p.ParentID == nil {
			topLevel = append(topLevel, p)
		}
	}
	total := len(topLevel)
	start := (page - 1) * perPage
	if start >= total {
		return []store.Project{}, total, nil
	}
	return topLevel[start:min(start+perPage, total)], total, nil
}

func (m *MemProjectStore) ListChildren(ctx context.Context, parentID int64) ([]store.Project, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []store.Project
	for _, p := range m.projects {
		if p.ParentID != nil && *p.ParentID == parentID {
			out = append(out, *p)
		}
	}
	return out, nil
}

func (m *MemProjectStore) ListChildIDs(ctx context.Context, parentID int64) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []string
	for _, p := range m.projects {
		if p.ParentID != nil && *p.ParentID == parentID {
			out = append(out, p.Slug)
		}
	}
	return out, nil
}

func (m *MemProjectStore) HasChildren(ctx context.Context, projectID int64) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, p := range m.projects {
		if p.ParentID != nil && *p.ParentID == projectID {
			return true, nil
		}
	}
	return false, nil
}

func (m *MemProjectStore) SetParent(ctx context.Context, projectID, parentID int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[projectID]
	if !ok {
		return store.ErrProjectNotFound
	}
	pid := parentID
	p.ParentID = &pid
	return nil
}

func (m *MemProjectStore) ClearParent(ctx context.Context, projectID int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[projectID]
	if !ok {
		return store.ErrProjectNotFound
	}
	p.ParentID = nil
	return nil
}

// MemKnownIssueStore is a thread-safe in-memory KnownIssueStorer for tests.
// Unlike MockKnownIssueStore, it maintains real state across method calls,
// making it suitable for handler tests that create and then list/get issues.
type MemKnownIssueStore struct {
	mu     sync.RWMutex
	issues []*store.KnownIssue
	nextID int64
}

var _ store.KnownIssueStorer = (*MemKnownIssueStore)(nil)

// NewMemKnownIssueStore returns an initialised MemKnownIssueStore.
func NewMemKnownIssueStore() *MemKnownIssueStore {
	return &MemKnownIssueStore{nextID: 1}
}

func (m *MemKnownIssueStore) Create(ctx context.Context, projectID int64, testName, pattern, ticketURL, description string) (*store.KnownIssue, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ki := range m.issues {
		if ki.ProjectID == projectID && ki.TestName == testName {
			return nil, store.ErrDuplicateEntry
		}
	}
	now := time.Now()
	ki := &store.KnownIssue{
		ID:          m.nextID,
		ProjectID:   projectID,
		TestName:    testName,
		Pattern:     pattern,
		TicketURL:   ticketURL,
		Description: description,
		IsActive:    true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	m.nextID++
	m.issues = append(m.issues, ki)
	result := *ki
	return &result, nil
}

func (m *MemKnownIssueStore) Get(ctx context.Context, id int64) (*store.KnownIssue, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ki := range m.issues {
		if ki.ID == id {
			result := *ki
			return &result, nil
		}
	}
	return nil, store.ErrKnownIssueNotFound
}

func (m *MemKnownIssueStore) List(ctx context.Context, projectID int64, activeOnly bool) ([]store.KnownIssue, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []store.KnownIssue
	for _, ki := range m.issues {
		if ki.ProjectID != projectID {
			continue
		}
		if activeOnly && !ki.IsActive {
			continue
		}
		out = append(out, *ki)
	}
	return out, nil
}

func (m *MemKnownIssueStore) ListPaginated(ctx context.Context, projectID int64, activeOnly bool, page, perPage int) ([]store.KnownIssue, int, error) {
	all, err := m.List(ctx, projectID, activeOnly)
	if err != nil {
		return nil, 0, err
	}
	total := len(all)
	start := (page - 1) * perPage
	if start >= total {
		return []store.KnownIssue{}, total, nil
	}
	end := min(start+perPage, total)
	return all[start:end], total, nil
}

func (m *MemKnownIssueStore) Update(ctx context.Context, id int64, projectID int64, ticketURL, description string, isActive bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, ki := range m.issues {
		if ki.ID == id && ki.ProjectID == projectID {
			ki.TicketURL = ticketURL
			ki.Description = description
			ki.IsActive = isActive
			ki.UpdatedAt = time.Now()
			return nil
		}
	}
	return store.ErrKnownIssueNotFound
}

func (m *MemKnownIssueStore) Delete(ctx context.Context, id int64, projectID int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, ki := range m.issues {
		if ki.ID == id && ki.ProjectID == projectID {
			m.issues = slices.Delete(m.issues, i, i+1)
			return nil
		}
	}
	return store.ErrKnownIssueNotFound
}

func (m *MemKnownIssueStore) IsKnown(ctx context.Context, projectID int64, testName string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, ki := range m.issues {
		if ki.ProjectID == projectID && ki.TestName == testName && ki.IsActive {
			return true, nil
		}
	}
	return false, nil
}

// ---------------------------------------------------------------------------
// MemBuildStore
// ---------------------------------------------------------------------------

// MemBuildStore is a thread-safe in-memory BuildStorer for tests.
// InsertBuild, UpdateBuildStats, UpdateBuildCIMetadata, and
// ListBuildsPaginatedBranch are fully stateful; remaining methods return
// zero values (matching MockBuildStore defaults) as they are not needed
// by the handler tests that use this store.
type MemBuildStore struct {
	mu     sync.RWMutex
	builds []*store.Build
	nextID int64
}

var _ store.BuildStorer = (*MemBuildStore)(nil)

// NewMemBuildStore returns an initialised MemBuildStore.
func NewMemBuildStore() *MemBuildStore {
	return &MemBuildStore{nextID: 1}
}

func (m *MemBuildStore) InsertBuild(ctx context.Context, projectID int64, buildNumber int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.builds = append(m.builds, &store.Build{
		ID:          m.nextID,
		ProjectID:   projectID,
		BuildNumber: buildNumber,
		CreatedAt:   time.Now(),
	})
	m.nextID++
	return nil
}

func (m *MemBuildStore) UpdateBuildStats(ctx context.Context, projectID int64, buildNumber int, stats store.BuildStats) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, b := range m.builds {
		if b.ProjectID == projectID && b.BuildNumber == buildNumber {
			b.StatPassed = &stats.Passed
			b.StatFailed = &stats.Failed
			b.StatBroken = &stats.Broken
			b.StatSkipped = &stats.Skipped
			b.StatUnknown = &stats.Unknown
			b.StatTotal = &stats.Total
			b.DurationMs = &stats.DurationMs
			b.FlakyCount = &stats.FlakyCount
			b.RetriedCount = &stats.RetriedCount
			b.NewFailedCount = &stats.NewFailedCount
			b.NewPassedCount = &stats.NewPassedCount
			return nil
		}
	}
	return store.ErrBuildNotFound
}

func (m *MemBuildStore) UpdateBuildCIMetadata(ctx context.Context, projectID int64, buildNumber int, ciMeta store.CIMetadata) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, b := range m.builds {
		if b.ProjectID == projectID && b.BuildNumber == buildNumber {
			b.CIProvider = &ciMeta.Provider
			b.CIBuildURL = &ciMeta.BuildURL
			b.CIBranch = &ciMeta.Branch
			b.CICommitSHA = &ciMeta.CommitSHA
			return nil
		}
	}
	return store.ErrBuildNotFound
}

func (m *MemBuildStore) ListBuildsPaginatedBranch(ctx context.Context, projectID int64, page, perPage int, _ *int64) ([]store.Build, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var filtered []store.Build
	for _, b := range m.builds {
		if b.ProjectID == projectID {
			filtered = append(filtered, *b)
		}
	}
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].BuildNumber > filtered[j].BuildNumber
	})
	total := len(filtered)
	start := (page - 1) * perPage
	if start >= total {
		return []store.Build{}, total, nil
	}
	end := min(start+perPage, total)
	return filtered[start:end], total, nil
}

func (m *MemBuildStore) NextBuildNumber(ctx context.Context, projectID int64) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	max := 0
	for _, b := range m.builds {
		if b.ProjectID == projectID && b.BuildNumber > max {
			max = b.BuildNumber
		}
	}
	return max + 1, nil
}

func (m *MemBuildStore) GetBuildByNumber(ctx context.Context, projectID int64, buildNumber int) (store.Build, error) {
	if err := ctx.Err(); err != nil {
		return store.Build{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, b := range m.builds {
		if b.ProjectID == projectID && b.BuildNumber == buildNumber {
			return *b, nil
		}
	}
	return store.Build{}, store.ErrBuildNotFound
}

func (m *MemBuildStore) GetPreviousBuild(_ context.Context, _ int64, _ int) (store.Build, error) {
	return store.Build{}, nil
}

func (m *MemBuildStore) GetLatestBuild(_ context.Context, _ int64) (store.Build, error) {
	return store.Build{}, nil
}

func (m *MemBuildStore) ListBuilds(_ context.Context, _ int64) ([]store.Build, error) {
	return nil, nil
}

func (m *MemBuildStore) ListBuildsPaginated(_ context.Context, _ int64, _, _ int) ([]store.Build, int, error) {
	return nil, 0, nil
}

func (m *MemBuildStore) PruneBuilds(_ context.Context, _ int64, _ int) ([]int, error) {
	return nil, nil
}

func (m *MemBuildStore) SetLatest(_ context.Context, _ int64, _ int) error {
	return nil
}

func (m *MemBuildStore) DeleteAllBuilds(_ context.Context, _ int64) error {
	return nil
}

func (m *MemBuildStore) GetDashboardData(_ context.Context, _ int) ([]store.DashboardProject, error) {
	return nil, nil
}

func (m *MemBuildStore) DeleteBuild(_ context.Context, _ int64, _ int) error {
	return nil
}

func (m *MemBuildStore) UpdateBuildBranchID(_ context.Context, _ int64, _ int, _ int64) error {
	return nil
}

func (m *MemBuildStore) SetLatestBranch(_ context.Context, _ int64, _ int, _ *int64) error {
	return nil
}

func (m *MemBuildStore) PruneBuildsBranch(_ context.Context, _ int64, _ int, _ *int64) ([]int, error) {
	return nil, nil
}

func (m *MemBuildStore) PruneBuildsByAge(_ context.Context, _ int64, _ time.Time) ([]int, error) {
	return nil, nil
}

func (m *MemBuildStore) ListBuildsInRange(_ context.Context, _ int64, _ *int64, _, _ time.Time, _ int) ([]store.Build, int, error) {
	return nil, 0, nil
}

func (m *MemBuildStore) SetHasPlaywrightReport(_ context.Context, _ int64, _ int, _ bool) error {
	return nil
}

// ---------------------------------------------------------------------------
// MemUserStore
// ---------------------------------------------------------------------------

// MemUserStore is a thread-safe in-memory UserStorer for tests.
type MemUserStore struct {
	mu     sync.RWMutex
	users  []*store.User
	nextID int64
}

var _ store.UserStorer = (*MemUserStore)(nil)

// NewMemUserStore returns an initialised MemUserStore.
func NewMemUserStore() *MemUserStore {
	return &MemUserStore{nextID: 1}
}

func (m *MemUserStore) UpsertByOIDC(ctx context.Context, provider, sub, email, name, role string) (*store.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	// Update existing record if provider+sub match.
	for _, u := range m.users {
		if u.Provider == provider && u.ProviderSub == sub && sub != "" {
			u.Email = email
			u.Name = name
			u.Role = role
			u.LastLogin = &now
			u.UpdatedAt = now
			cp := *u
			return &cp, nil
		}
	}
	// Insert new record.
	u := &store.User{
		ID:          m.nextID,
		Email:       email,
		Name:        name,
		Provider:    provider,
		ProviderSub: sub,
		Role:        role,
		IsActive:    true,
		LastLogin:   &now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	m.nextID++
	m.users = append(m.users, u)
	cp := *u
	return &cp, nil
}

func (m *MemUserStore) GetByID(ctx context.Context, id int64) (*store.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if u.ID == id {
			cp := *u
			return &cp, nil
		}
	}
	return nil, store.ErrUserNotFound
}

func (m *MemUserStore) GetByEmail(ctx context.Context, email string) (*store.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	// Prefer active matches first, then any match.
	var fallback *store.User
	for _, u := range m.users {
		if strings.EqualFold(u.Email, email) {
			if u.IsActive {
				cp := *u
				return &cp, nil
			}
			if fallback == nil {
				cp := *u
				fallback = &cp
			}
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, store.ErrUserNotFound
}

// CreateLocal inserts a new local user. Returns store.ErrDuplicateEntry when
// an active row already holds the same email (case-insensitive).
func (m *MemUserStore) CreateLocal(ctx context.Context, email, name, passwordHash, role string) (*store.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if strings.EqualFold(u.Email, email) && u.IsActive {
			return nil, store.ErrDuplicateEntry
		}
	}
	now := time.Now()
	u := &store.User{
		ID:           m.nextID,
		Email:        email,
		Name:         name,
		Provider:     "local",
		PasswordHash: passwordHash,
		Role:         role,
		IsActive:     true,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	m.nextID++
	m.users = append(m.users, u)
	cp := *u
	return &cp, nil
}

func (m *MemUserStore) List(ctx context.Context) ([]store.User, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]store.User, 0, len(m.users))
	for _, u := range m.users {
		out = append(out, *u)
	}
	return out, nil
}

// ListPaginated applies search + role + active filters, orders by email
// ascending, and paginates via limit/offset.
func (m *MemUserStore) ListPaginated(ctx context.Context, params store.ListUsersParams) ([]store.User, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()

	search := strings.ToLower(strings.TrimSpace(params.Search))
	role := strings.TrimSpace(params.Role)

	var filtered []store.User
	for _, u := range m.users {
		if search != "" &&
			!strings.Contains(strings.ToLower(u.Email), search) &&
			!strings.Contains(strings.ToLower(u.Name), search) {
			continue
		}
		if role != "" && u.Role != role {
			continue
		}
		if params.Active != nil && u.IsActive != *params.Active {
			continue
		}
		filtered = append(filtered, *u)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Email < filtered[j].Email })

	total := len(filtered)
	limit := params.Limit
	if limit <= 0 {
		limit = 20
	}
	offset := max(params.Offset, 0)
	if offset >= total {
		return []store.User{}, total, nil
	}
	end := min(offset+limit, total)
	return filtered[offset:end], total, nil
}

// UpdateRole changes the user's role.
func (m *MemUserStore) UpdateRole(ctx context.Context, id int64, role string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			u.Role = role
			u.UpdatedAt = time.Now()
			return nil
		}
	}
	return store.ErrUserNotFound
}

// UpdateActive toggles the user's is_active flag.
func (m *MemUserStore) UpdateActive(ctx context.Context, id int64, active bool) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			u.IsActive = active
			u.UpdatedAt = time.Now()
			return nil
		}
	}
	return store.ErrUserNotFound
}

// UpdateProfile updates the user's display name.
func (m *MemUserStore) UpdateProfile(ctx context.Context, id int64, name string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			u.Name = name
			u.UpdatedAt = time.Now()
			return nil
		}
	}
	return store.ErrUserNotFound
}

func (m *MemUserStore) Deactivate(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			u.IsActive = false
			u.UpdatedAt = time.Now()
			return nil
		}
	}
	return store.ErrUserNotFound
}

// UpdatePasswordHash replaces the user's password_hash and bumps updated_at.
func (m *MemUserStore) UpdatePasswordHash(ctx context.Context, id int64, passwordHash string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			u.PasswordHash = passwordHash
			u.UpdatedAt = time.Now()
			return nil
		}
	}
	return store.ErrUserNotFound
}

// UpdateLastLogin sets last_login and updated_at to the current time.
func (m *MemUserStore) UpdateLastLogin(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			now := time.Now()
			u.LastLogin = &now
			u.UpdatedAt = now
			return nil
		}
	}
	return store.ErrUserNotFound
}

// ClearLastLogin zeroes out the user's LastLogin pointer. Test-only helper used
// to set up fixtures for verifying Login-path last_login population.
func (m *MemUserStore) ClearLastLogin(ctx context.Context, id int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			u.LastLogin = nil
			return nil
		}
	}
	return store.ErrUserNotFound
}
