package pg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// WebhookStore implements store.WebhookStorer using PostgreSQL.
type WebhookStore struct {
	pool   *pgxpool.Pool
	encKey []byte
	logger *zap.Logger
}

// NewWebhookStore creates a new WebhookStore.
func NewWebhookStore(s *PGStore, encKey []byte, logger *zap.Logger) *WebhookStore {
	return &WebhookStore{pool: s.pool, encKey: encKey, logger: logger}
}

var _ store.WebhookStorer = (*WebhookStore)(nil)

// Create inserts a new webhook, encrypting URL and Secret before storage.
// Returns the created webhook with generated ID and timestamps.
func (ws *WebhookStore) Create(ctx context.Context, wh *store.Webhook) (*store.Webhook, error) {
	encURL, err := security.Encrypt(wh.URL, ws.encKey)
	if err != nil {
		return nil, fmt.Errorf("encrypt webhook url: %w", err)
	}

	var encSecret *string
	if wh.Secret != nil {
		s, err := security.Encrypt(*wh.Secret, ws.encKey)
		if err != nil {
			return nil, fmt.Errorf("encrypt webhook secret: %w", err)
		}
		encSecret = &s
	}

	err = ws.pool.QueryRow(ctx, `
		INSERT INTO webhooks (project_id, name, target_type, url, secret, template, events, is_active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at, updated_at`,
		wh.ProjectID, wh.Name, wh.TargetType, encURL, encSecret,
		wh.Template, wh.Events, wh.IsActive,
	).Scan(&wh.ID, &wh.CreatedAt, &wh.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("create webhook: %w", err)
	}

	// Return the webhook with the plaintext URL (not encrypted).
	return wh, nil
}

// GetByID retrieves a single webhook by its UUID, decrypting URL and Secret.
// Returns store.ErrWebhookNotFound when no matching row exists.
func (ws *WebhookStore) GetByID(ctx context.Context, webhookID string) (*store.Webhook, error) {
	var wh store.Webhook
	var encURL string
	var encSecret *string

	err := ws.pool.QueryRow(ctx, `
		SELECT id, project_id, name, target_type, url, secret, template, events, is_active, created_at, updated_at
		FROM webhooks
		WHERE id = $1`, webhookID,
	).Scan(&wh.ID, &wh.ProjectID, &wh.Name, &wh.TargetType, &encURL, &encSecret,
		&wh.Template, &wh.Events, &wh.IsActive, &wh.CreatedAt, &wh.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("%w: id=%s", store.ErrWebhookNotFound, webhookID)
	}
	if err != nil {
		return nil, fmt.Errorf("get webhook by id: %w", err)
	}

	wh.URL, err = security.Decrypt(encURL, ws.encKey)
	if err != nil {
		return nil, fmt.Errorf("decrypt webhook url: %w", err)
	}

	if encSecret != nil {
		plain, err := security.Decrypt(*encSecret, ws.encKey)
		if err != nil {
			return nil, fmt.Errorf("decrypt webhook secret: %w", err)
		}
		wh.Secret = &plain
	}

	return &wh, nil
}

// List returns all webhooks for the given project, ordered by created_at DESC.
// URLs are decrypted; Secrets are not returned in list responses.
func (ws *WebhookStore) List(ctx context.Context, projectID string) ([]store.Webhook, error) {
	rows, err := ws.pool.Query(ctx, `
		SELECT id, project_id, name, target_type, url, template, events, is_active, created_at, updated_at
		FROM webhooks
		WHERE project_id = $1
		ORDER BY created_at DESC`, projectID,
	)
	if err != nil {
		return nil, fmt.Errorf("list webhooks: %w", err)
	}
	defer rows.Close()

	var webhooks []store.Webhook
	for rows.Next() {
		var wh store.Webhook
		var encURL string

		if err := rows.Scan(&wh.ID, &wh.ProjectID, &wh.Name, &wh.TargetType, &encURL,
			&wh.Template, &wh.Events, &wh.IsActive, &wh.CreatedAt, &wh.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan webhook row: %w", err)
		}

		wh.URL, err = security.Decrypt(encURL, ws.encKey)
		if err != nil {
			ws.logger.Warn("failed to decrypt webhook url", zap.String("webhook_id", wh.ID), zap.Error(err))
			continue
		}

		webhooks = append(webhooks, wh)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate webhook rows: %w", err)
	}
	if webhooks == nil {
		webhooks = []store.Webhook{}
	}
	return webhooks, nil
}

// Update updates a webhook's mutable fields. The URL and Secret are re-encrypted.
// Returns store.ErrWebhookNotFound if no rows were affected.
func (ws *WebhookStore) Update(ctx context.Context, wh *store.Webhook) error {
	encURL, err := security.Encrypt(wh.URL, ws.encKey)
	if err != nil {
		return fmt.Errorf("encrypt webhook url: %w", err)
	}

	var encSecret *string
	if wh.Secret != nil {
		s, err := security.Encrypt(*wh.Secret, ws.encKey)
		if err != nil {
			return fmt.Errorf("encrypt webhook secret: %w", err)
		}
		encSecret = &s
	}

	tag, err := ws.pool.Exec(ctx, `
		UPDATE webhooks
		SET name        = $1,
		    target_type = $2,
		    url         = $3,
		    secret      = $4,
		    template    = $5,
		    events      = $6,
		    is_active   = $7,
		    updated_at  = NOW()
		WHERE id = $8`,
		wh.Name, wh.TargetType, encURL, encSecret,
		wh.Template, wh.Events, wh.IsActive, wh.ID,
	)
	if err != nil {
		return fmt.Errorf("update webhook: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%s", store.ErrWebhookNotFound, wh.ID)
	}
	return nil
}

// Delete removes a webhook scoped to a project (IDOR prevention).
// Returns store.ErrWebhookNotFound if no rows were affected.
func (ws *WebhookStore) Delete(ctx context.Context, webhookID, projectID string) error {
	tag, err := ws.pool.Exec(ctx,
		"DELETE FROM webhooks WHERE id = $1 AND project_id = $2", webhookID, projectID)
	if err != nil {
		return fmt.Errorf("delete webhook: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w: id=%s", store.ErrWebhookNotFound, webhookID)
	}
	return nil
}

// ListActiveForEvent returns all active webhooks for a project that subscribe
// to the given event. This is the hot-path query used by the delivery engine.
func (ws *WebhookStore) ListActiveForEvent(ctx context.Context, projectID, event string) ([]store.Webhook, error) {
	rows, err := ws.pool.Query(ctx, `
		SELECT id, project_id, name, target_type, url, secret, template, events, is_active, created_at, updated_at
		FROM webhooks
		WHERE project_id = $1
		  AND is_active = true
		  AND $2 = ANY(events)`, projectID, event,
	)
	if err != nil {
		return nil, fmt.Errorf("list active webhooks for event: %w", err)
	}
	defer rows.Close()

	var webhooks []store.Webhook
	for rows.Next() {
		var wh store.Webhook
		var encURL string
		var encSecret *string

		if err := rows.Scan(&wh.ID, &wh.ProjectID, &wh.Name, &wh.TargetType, &encURL, &encSecret,
			&wh.Template, &wh.Events, &wh.IsActive, &wh.CreatedAt, &wh.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan active webhook row: %w", err)
		}

		wh.URL, err = security.Decrypt(encURL, ws.encKey)
		if err != nil {
			ws.logger.Warn("failed to decrypt webhook url", zap.String("webhook_id", wh.ID), zap.Error(err))
			continue
		}

		if encSecret != nil {
			plain, err := security.Decrypt(*encSecret, ws.encKey)
			if err != nil {
				ws.logger.Warn("failed to decrypt webhook secret", zap.String("webhook_id", wh.ID), zap.Error(err))
				continue
			}
			wh.Secret = &plain
		}

		webhooks = append(webhooks, wh)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate active webhook rows: %w", err)
	}
	if webhooks == nil {
		webhooks = []store.Webhook{}
	}
	return webhooks, nil
}

// InsertDelivery records a single webhook delivery attempt.
func (ws *WebhookStore) InsertDelivery(ctx context.Context, d *store.WebhookDelivery) error {
	err := ws.pool.QueryRow(ctx, `
		INSERT INTO webhook_deliveries
		    (webhook_id, build_id, event, payload, status_code, response_body, error, attempt, duration_ms, delivered_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id`,
		d.WebhookID, d.BuildID, d.Event, d.Payload,
		d.StatusCode, d.ResponseBody, d.Error,
		d.Attempt, d.DurationMs, d.DeliveredAt,
	).Scan(&d.ID)
	if err != nil {
		return fmt.Errorf("insert webhook delivery: %w", err)
	}
	return nil
}

// ListDeliveries returns a paginated list of delivery attempts for a webhook,
// ordered by delivered_at DESC. Total count is returned alongside the page.
func (ws *WebhookStore) ListDeliveries(ctx context.Context, webhookID string, page, perPage int) ([]store.WebhookDelivery, int, error) {
	if page < 1 {
		page = 1
	}
	if perPage < 1 {
		perPage = 20
	}
	offset := (page - 1) * perPage

	rows, err := ws.pool.Query(ctx, `
		SELECT id, webhook_id, build_id, event, payload, status_code, response_body, error,
		       attempt, duration_ms, delivered_at,
		       COUNT(*) OVER() AS total
		FROM webhook_deliveries
		WHERE webhook_id = $1
		ORDER BY delivered_at DESC
		LIMIT $2 OFFSET $3`,
		webhookID, perPage, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list webhook deliveries: %w", err)
	}
	defer rows.Close()

	var deliveries []store.WebhookDelivery
	var total int
	for rows.Next() {
		var d store.WebhookDelivery
		if err := rows.Scan(
			&d.ID, &d.WebhookID, &d.BuildID, &d.Event, &d.Payload,
			&d.StatusCode, &d.ResponseBody, &d.Error,
			&d.Attempt, &d.DurationMs, &d.DeliveredAt,
			&total,
		); err != nil {
			return nil, 0, fmt.Errorf("scan webhook delivery row: %w", err)
		}
		deliveries = append(deliveries, d)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate webhook delivery rows: %w", err)
	}
	if deliveries == nil {
		deliveries = []store.WebhookDelivery{}
	}
	return deliveries, total, nil
}

// PruneDeliveries deletes delivery records older than the given cutoff time.
// Returns the number of rows deleted.
func (ws *WebhookStore) PruneDeliveries(ctx context.Context, olderThan time.Time) (int64, error) {
	tag, err := ws.pool.Exec(ctx,
		"DELETE FROM webhook_deliveries WHERE delivered_at < $1", olderThan)
	if err != nil {
		return 0, fmt.Errorf("prune webhook deliveries: %w", err)
	}
	return tag.RowsAffected(), nil
}
