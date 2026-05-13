package bootstrap

import (
	"context"
	"database/sql"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
	"github.com/mkutlak/alluredeck/api/internal/storage"
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
// dataStore is used for the one-time SyncMetadata call that imports any
// filesystem projects/builds not yet recorded in PostgreSQL. Pass nil to skip
// the sync (e.g. in cmd/mcp which is read-only).
func InitStores(ctx context.Context, cfg *config.Config, poolCfg PoolConfig, encKey []byte, dataStore storage.Store, logger *zap.Logger) (*Stores, error) {
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
		Project:       pgProj,
		Build:         pgBuild,
		RefreshFamily: pg.NewRefreshTokenFamilyStore(pgDB),
		Blacklist:     pg.NewBlacklistStore(pgDB),
		TestResult:    pg.NewTestResultStore(pgDB, logger),
		Branch:        pg.NewBranchStore(pgDB),
		KnownIssue:    pg.NewKnownIssueStore(pgDB),
		Search:        pg.NewSearchStore(pgDB, logger),
		Analytics:     pg.NewAnalyticsStore(pgDB),
		APIKey:        pg.NewAPIKeyStore(pgDB),
		Attachment:    pg.NewAttachmentStore(pgDB),
		User:          pg.NewUserStore(pgDB),
		Defect:        pg.NewDefectStore(pgDB),
		Webhook:             pg.NewWebhookStore(pgDB, encKey, logger),
		Pipeline:            pg.NewPipelineStore(pgDB),
		Preference:          pg.NewPreferenceStore(pgDB),
		Audit:               pg.NewAuditStore(pgDB),
		DefectProposals:     pg.NewDefectProposalStore(pgDB),
		KnownIssueProposals: pg.NewKnownIssueProposalStore(pgDB),
		FlakyProposals:      pg.NewFlakyProposalStore(pgDB),
		DB:      pgDB.DB(),
		Locker:  pgDB,
		PGStore: pgDB,
	}

	if dataStore != nil {
		if err := pg.SyncMetadata(ctx, dataStore, pgProj, pgBuild, logger); err != nil {
			logger.Warn("metadata sync failed (non-fatal)", zap.Error(err))
		}
	}

	return s, nil
}
