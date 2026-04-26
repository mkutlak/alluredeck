package pg

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// AuditStore provides audit_log persistence backed by PostgreSQL. It implements
// store.AuditLogger as a thin INSERT/SELECT wrapper — no caching, no batching:
// an audit event is one row.
type AuditStore struct {
	pool *pgxpool.Pool
}

// NewAuditStore creates an AuditStore backed by the given PGStore.
func NewAuditStore(s *PGStore) *AuditStore {
	return &AuditStore{pool: s.pool}
}

var _ store.AuditLogger = (*AuditStore)(nil)

// Record inserts a single audit event. occurred_at is omitted from the INSERT
// list when the caller passed the zero value so PostgreSQL's column DEFAULT
// NOW() fills it in — this matches the docstring contract.
func (a *AuditStore) Record(ctx context.Context, evt store.AuditEvent) error {
	if evt.Action == "" {
		return fmt.Errorf("audit record: action is required")
	}
	if evt.Outcome == "" {
		return fmt.Errorf("audit record: outcome is required")
	}

	if evt.OccurredAt.IsZero() {
		_, err := a.pool.Exec(ctx, `
			INSERT INTO audit_log (
				actor_id, actor_label, target_type, target_id,
				action, outcome, ip, user_agent, request_id, metadata
			) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			evt.ActorID,
			evt.ActorLabel,
			evt.TargetType,
			evt.TargetID,
			evt.Action,
			evt.Outcome,
			evt.IP,
			evt.UserAgent,
			evt.RequestID,
			evt.Metadata,
		)
		if err != nil {
			return fmt.Errorf("audit record: %w", err)
		}
		return nil
	}

	_, err := a.pool.Exec(ctx, `
		INSERT INTO audit_log (
			occurred_at, actor_id, actor_label, target_type, target_id,
			action, outcome, ip, user_agent, request_id, metadata
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		evt.OccurredAt.UTC(),
		evt.ActorID,
		evt.ActorLabel,
		evt.TargetType,
		evt.TargetID,
		evt.Action,
		evt.Outcome,
		evt.IP,
		evt.UserAgent,
		evt.RequestID,
		evt.Metadata,
	)
	if err != nil {
		return fmt.Errorf("audit record: %w", err)
	}
	return nil
}

// ListRecent returns the newest `limit` audit events ordered by occurred_at
// DESC. A non-positive limit is normalised to 50 to avoid an unbounded scan.
func (a *AuditStore) ListRecent(ctx context.Context, limit int) ([]store.AuditEvent, error) {
	if limit <= 0 {
		limit = 50
	}

	rows, err := a.pool.Query(ctx, `
		SELECT id, occurred_at, actor_id, actor_label, target_type, target_id,
		       action, outcome, ip, user_agent, request_id, metadata
		FROM audit_log
		ORDER BY occurred_at DESC, id DESC
		LIMIT $1`, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("audit list recent: %w", err)
	}
	defer rows.Close()

	out := make([]store.AuditEvent, 0, limit)
	for rows.Next() {
		var e store.AuditEvent
		if scanErr := rows.Scan(
			&e.ID,
			&e.OccurredAt,
			&e.ActorID,
			&e.ActorLabel,
			&e.TargetType,
			&e.TargetID,
			&e.Action,
			&e.Outcome,
			&e.IP,
			&e.UserAgent,
			&e.RequestID,
			&e.Metadata,
		); scanErr != nil {
			return nil, fmt.Errorf("audit list recent: scan: %w", scanErr)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("audit list recent: rows: %w", err)
	}
	return out, nil
}
