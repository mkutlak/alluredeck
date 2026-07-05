package pg

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// Compile-time interface compliance check.
var _ store.FailureSummaryStorer = (*FailureSummaryStore)(nil)

// FailureSummaryStore caches LLM-generated failure summaries in the
// failure_summaries table.
type FailureSummaryStore struct {
	pool *pgxpool.Pool
}

// NewFailureSummaryStore creates a FailureSummaryStore backed by the given PGStore.
func NewFailureSummaryStore(s *PGStore) *FailureSummaryStore {
	return &FailureSummaryStore{pool: s.pool}
}

// Get returns the cached summary for (buildID, historyID), or (nil, nil) when
// no row exists.
func (fs *FailureSummaryStore) Get(ctx context.Context, buildID int64, historyID string) (*store.FailureSummary, error) {
	const q = `
		SELECT build_id, history_id, project_id, input_hash, hypothesis,
		       category, confidence, evidence, model, prompt_version, created_at
		FROM failure_summaries
		WHERE build_id = $1 AND history_id = $2`

	var (
		s           store.FailureSummary
		evidenceRaw []byte
	)
	err := fs.pool.QueryRow(ctx, q, buildID, historyID).Scan(
		&s.BuildID, &s.HistoryID, &s.ProjectID, &s.InputHash, &s.Hypothesis,
		&s.Category, &s.Confidence, &evidenceRaw, &s.Model, &s.PromptVersion, &s.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get failure summary: %w", err)
	}
	if len(evidenceRaw) > 0 {
		if err := json.Unmarshal(evidenceRaw, &s.Evidence); err != nil {
			return nil, fmt.Errorf("get failure summary: decode evidence: %w", err)
		}
	}
	return &s, nil
}

// Upsert writes or replaces the summary for (build_id, history_id).
func (fs *FailureSummaryStore) Upsert(ctx context.Context, s store.FailureSummary) error {
	evidence := s.Evidence
	if evidence == nil {
		evidence = []string{}
	}
	evidenceJSON, err := json.Marshal(evidence)
	if err != nil {
		return fmt.Errorf("upsert failure summary: encode evidence: %w", err)
	}

	promptVersion := s.PromptVersion
	if promptVersion == 0 {
		promptVersion = 1
	}

	const q = `
		INSERT INTO failure_summaries (
			build_id, history_id, project_id, input_hash, hypothesis,
			category, confidence, evidence, model, prompt_version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		ON CONFLICT (build_id, history_id) DO UPDATE SET
			project_id     = EXCLUDED.project_id,
			input_hash     = EXCLUDED.input_hash,
			hypothesis     = EXCLUDED.hypothesis,
			category       = EXCLUDED.category,
			confidence     = EXCLUDED.confidence,
			evidence       = EXCLUDED.evidence,
			model          = EXCLUDED.model,
			prompt_version = EXCLUDED.prompt_version,
			created_at     = now()`

	if _, err := fs.pool.Exec(ctx, q,
		s.BuildID, s.HistoryID, s.ProjectID, s.InputHash, s.Hypothesis,
		s.Category, s.Confidence, evidenceJSON, s.Model, promptVersion,
	); err != nil {
		return fmt.Errorf("upsert failure summary: %w", err)
	}
	return nil
}
