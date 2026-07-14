package bootstrap

import (
	"context"
	"database/sql"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// PoolConfig holds pgx connection-pool tuning parameters.
// Use DefaultPoolConfig() for sensible defaults; override individual fields
// for workloads with different concurrency requirements (e.g. cmd/mcp uses
// a smaller MaxConns than cmd/api).
type PoolConfig struct {
	MaxConns int           // maps to DBMaxOpenConns (0 = use cfg default)
	MinConns int           // maps to DBMaxIdleConns  (0 = use cfg default)
	MaxIdle  time.Duration // maps to DBConnMaxLifetime (0 = use cfg default)
}

// DefaultPoolConfig returns sensible defaults for a production API workload.
func DefaultPoolConfig() PoolConfig {
	return PoolConfig{
		MaxConns: 25,
		MinConns: 2,
		MaxIdle:  5 * time.Minute,
	}
}

// Stores groups all database store instances. Fields use the store interface
// types from api/internal/store/interfaces.go so downstream code is testable
// without importing the pg/ implementation package.
type Stores struct {
	Project       store.ProjectStorer
	Build         store.BuildStorer
	Blacklist     store.BlacklistStorer
	TestResult    store.TestResultStorer
	Branch        store.BranchStorer
	KnownIssue    store.KnownIssueStorer
	Search        store.SearchStorer
	Analytics     store.AnalyticsStorer
	APIKey        store.APIKeyStorer
	Attachment    store.AttachmentStorer
	User          store.UserStorer
	Defect        store.DefectStorer
	Webhook       store.WebhookStorer
	Pipeline      store.PipelineStorer
	Preference    store.PreferenceStorer
	RefreshFamily store.RefreshTokenFamilyStorer
	Audit         store.AuditLogger

	DefectProposals     store.DefectProposalStorer
	KnownIssueProposals store.KnownIssueProposalStorer
	FlakyProposals      store.FlakyProposalStorer

	FailureSummary store.FailureSummaryStorer

	// ProjectPG and BuildPG expose the concrete pg store implementations so the
	// caller can invoke pg.SyncMetadata, which requires the concrete types
	// (*pg.ProjectStore, *pg.BuildStore) rather than the store interfaces above.
	// They hold the same instances as Project/Build, just typed concretely.
	ProjectPG *pg.ProjectStore
	BuildPG   *pg.BuildStore

	// DB exposes the *sql.DB handle for probes (e.g. SystemHandler).
	DB *sql.DB
	// Locker provides PostgreSQL advisory locks for multi-instance safety.
	Locker store.Locker
	// PGStore exposes the underlying *pg.PGStore so callers that need the raw
	// pool (e.g. River job manager) can obtain it via PGStore.Pool(). It is
	// also used by Close().
	PGStore *pg.PGStore
}

// Close gracefully tears down the database connection pool. It should be
// called in a deferred statement after the HTTP server has drained.
func (s *Stores) Close() error {
	return s.PGStore.Close()
}

// InitStores opens the PostgreSQL connection pool (applying poolCfg overrides),
// runs all pending migrations, and wires every store implementation. The
// returned *Stores.pgDB is retained for pool access (e.g. River job manager).
//
// encKey is required for the webhook store (AES-encrypted webhook secrets).
// Pass security.DeriveEncryptionKey(cfg.JWTSecret) from the caller.
//
// The one-time filesystem metadata sync (pg.SyncMetadata) is NOT run here; the
// caller invokes it separately using the concrete ProjectPG/BuildPG stores
// exposed on the returned *Stores. cmd/api runs it in a background goroutine
// after binding the HTTP listener so slow startup work never blocks readiness.
func InitStores(ctx context.Context, cfg *config.Config, poolCfg PoolConfig, encKey []byte, logger *zap.Logger) (*Stores, error) {
	// Apply PoolConfig overrides onto cfg fields before handing cfg to pg.Open.
	// We work on a shallow copy of the config so the caller's cfg is unchanged.
	cfgCopy := *cfg
	if poolCfg.MaxConns > 0 {
		cfgCopy.DBMaxOpenConns = poolCfg.MaxConns
	}
	if poolCfg.MinConns > 0 {
		cfgCopy.DBMaxIdleConns = poolCfg.MinConns
	}
	if poolCfg.MaxIdle > 0 {
		cfgCopy.DBConnMaxLifetime = poolCfg.MaxIdle
	}

	// pg.Open creates the pool, pings, and runs goose + River migrations.
	pgDB, err := pg.Open(ctx, &cfgCopy)
	if err != nil {
		return nil, err
	}

	pgProj := pg.NewProjectStore(pgDB, logger)
	pgBuild := pg.NewBuildStore(pgDB, logger)

	s := &Stores{
		Project:             pgProj,
		Build:               pgBuild,
		RefreshFamily:       pg.NewRefreshTokenFamilyStore(pgDB),
		Blacklist:           pg.NewBlacklistStore(pgDB),
		TestResult:          pg.NewTestResultStore(pgDB, logger),
		Branch:              pg.NewBranchStore(pgDB),
		KnownIssue:          pg.NewKnownIssueStore(pgDB),
		Search:              pg.NewSearchStore(pgDB, logger),
		Analytics:           pg.NewAnalyticsStore(pgDB),
		APIKey:              pg.NewAPIKeyStore(pgDB),
		Attachment:          pg.NewAttachmentStore(pgDB),
		User:                pg.NewUserStore(pgDB),
		Defect:              pg.NewDefectStore(pgDB),
		Webhook:             pg.NewWebhookStore(pgDB, encKey, logger),
		Pipeline:            pg.NewPipelineStore(pgDB),
		Preference:          pg.NewPreferenceStore(pgDB),
		Audit:               pg.NewAuditStore(pgDB),
		DefectProposals:     pg.NewDefectProposalStore(pgDB),
		KnownIssueProposals: pg.NewKnownIssueProposalStore(pgDB),
		FlakyProposals:      pg.NewFlakyProposalStore(pgDB),
		FailureSummary:      pg.NewFailureSummaryStore(pgDB),
		ProjectPG:           pgProj,
		BuildPG:             pgBuild,
		DB:                  pgDB.DB(),
		Locker:              pgDB,
		PGStore:             pgDB,
	}

	return s, nil
}
