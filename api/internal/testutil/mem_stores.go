package testutil

import (
	"context"
	"slices"
	"sort"
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
	projects map[string]*store.Project // keyed by project ID
}

var _ store.ProjectStorer = (*MemProjectStore)(nil)

// NewMemProjectStore returns an initialised MemProjectStore.
func NewMemProjectStore() *MemProjectStore {
	return &MemProjectStore{projects: make(map[string]*store.Project)}
}

func (m *MemProjectStore) CreateProject(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.projects[id]; ok {
		return store.ErrProjectExists
	}
	m.projects[id] = &store.Project{ID: id, CreatedAt: time.Now()}
	return nil
}

func (m *MemProjectStore) GetProject(ctx context.Context, id string) (*store.Project, error) {
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

func (m *MemProjectStore) ListProjectsPaginated(ctx context.Context, page, perPage int, tag string) ([]store.Project, int, error) {
	all, err := m.ListProjects(ctx)
	if err != nil {
		return nil, 0, err
	}
	if tag != "" {
		var filtered []store.Project
		for _, p := range all {
			if slices.Contains(p.Tags, tag) {
				filtered = append(filtered, p)
			}
		}
		all = filtered
	}
	total := len(all)
	start := (page - 1) * perPage
	if start >= total {
		return []store.Project{}, total, nil
	}
	return all[start:min(start+perPage, total)], total, nil
}

func (m *MemProjectStore) ListAllTags(ctx context.Context) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	seen := make(map[string]struct{})
	for _, p := range m.projects {
		for _, t := range p.Tags {
			seen[t] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out, nil
}

func (m *MemProjectStore) SetTags(ctx context.Context, projectID string, tags []string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.projects[projectID]
	if !ok {
		return store.ErrProjectNotFound
	}
	p.Tags = slices.Clone(tags)
	return nil
}

func (m *MemProjectStore) DeleteProject(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.projects[id]; !ok {
		return store.ErrProjectNotFound
	}
	delete(m.projects, id)
	return nil
}

func (m *MemProjectStore) ProjectExists(ctx context.Context, id string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.projects[id]
	return ok, nil
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

func (m *MemKnownIssueStore) Create(ctx context.Context, projectID, testName, pattern, ticketURL, description string) (*store.KnownIssue, error) {
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

func (m *MemKnownIssueStore) List(ctx context.Context, projectID string, activeOnly bool) ([]store.KnownIssue, error) {
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

func (m *MemKnownIssueStore) ListPaginated(ctx context.Context, projectID string, activeOnly bool, page, perPage int) ([]store.KnownIssue, int, error) {
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

func (m *MemKnownIssueStore) Update(ctx context.Context, id int64, projectID, ticketURL, description string, isActive bool) error {
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

func (m *MemKnownIssueStore) Delete(ctx context.Context, id int64, projectID string) error {
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

func (m *MemKnownIssueStore) IsKnown(ctx context.Context, projectID, testName string) (bool, error) {
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

func (m *MemBuildStore) InsertBuild(ctx context.Context, projectID string, buildOrder int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.builds = append(m.builds, &store.Build{
		ID:         m.nextID,
		ProjectID:  projectID,
		BuildOrder: buildOrder,
		CreatedAt:  time.Now(),
	})
	m.nextID++
	return nil
}

func (m *MemBuildStore) UpdateBuildStats(ctx context.Context, projectID string, buildOrder int, stats store.BuildStats) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, b := range m.builds {
		if b.ProjectID == projectID && b.BuildOrder == buildOrder {
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

func (m *MemBuildStore) UpdateBuildCIMetadata(ctx context.Context, projectID string, buildOrder int, ciMeta store.CIMetadata) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, b := range m.builds {
		if b.ProjectID == projectID && b.BuildOrder == buildOrder {
			b.CIProvider = &ciMeta.Provider
			b.CIBuildURL = &ciMeta.BuildURL
			b.CIBranch = &ciMeta.Branch
			b.CICommitSHA = &ciMeta.CommitSHA
			return nil
		}
	}
	return store.ErrBuildNotFound
}

func (m *MemBuildStore) ListBuildsPaginatedBranch(ctx context.Context, projectID string, page, perPage int, _ *int64) ([]store.Build, int, error) {
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
		return filtered[i].BuildOrder > filtered[j].BuildOrder
	})
	total := len(filtered)
	start := (page - 1) * perPage
	if start >= total {
		return []store.Build{}, total, nil
	}
	end := min(start+perPage, total)
	return filtered[start:end], total, nil
}

func (m *MemBuildStore) NextBuildOrder(ctx context.Context, projectID string) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	max := 0
	for _, b := range m.builds {
		if b.ProjectID == projectID && b.BuildOrder > max {
			max = b.BuildOrder
		}
	}
	return max + 1, nil
}

func (m *MemBuildStore) GetBuildByOrder(ctx context.Context, projectID string, buildOrder int) (store.Build, error) {
	if err := ctx.Err(); err != nil {
		return store.Build{}, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, b := range m.builds {
		if b.ProjectID == projectID && b.BuildOrder == buildOrder {
			return *b, nil
		}
	}
	return store.Build{}, store.ErrBuildNotFound
}

func (m *MemBuildStore) GetPreviousBuild(_ context.Context, _ string, _ int) (store.Build, error) {
	return store.Build{}, nil
}

func (m *MemBuildStore) GetLatestBuild(_ context.Context, _ string) (store.Build, error) {
	return store.Build{}, nil
}

func (m *MemBuildStore) ListBuilds(_ context.Context, _ string) ([]store.Build, error) {
	return nil, nil
}

func (m *MemBuildStore) ListBuildsPaginated(_ context.Context, _ string, _, _ int) ([]store.Build, int, error) {
	return nil, 0, nil
}

func (m *MemBuildStore) PruneBuilds(_ context.Context, _ string, _ int) ([]int, error) {
	return nil, nil
}

func (m *MemBuildStore) SetLatest(_ context.Context, _ string, _ int) error {
	return nil
}

func (m *MemBuildStore) DeleteAllBuilds(_ context.Context, _ string) error {
	return nil
}

func (m *MemBuildStore) GetDashboardData(_ context.Context, _ int, _ string) ([]store.DashboardProject, error) {
	return nil, nil
}

func (m *MemBuildStore) DeleteBuild(_ context.Context, _ string, _ int) error {
	return nil
}

func (m *MemBuildStore) UpdateBuildBranchID(_ context.Context, _ string, _ int, _ int64) error {
	return nil
}

func (m *MemBuildStore) SetLatestBranch(_ context.Context, _ string, _ int, _ *int64) error {
	return nil
}

func (m *MemBuildStore) PruneBuildsBranch(_ context.Context, _ string, _ int, _ *int64) ([]int, error) {
	return nil, nil
}

func (m *MemBuildStore) PruneBuildsByAge(_ context.Context, _ string, _ time.Time) ([]int, error) {
	return nil, nil
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
	for _, u := range m.users {
		if u.Email == email {
			cp := *u
			return &cp, nil
		}
	}
	return nil, store.ErrUserNotFound
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
