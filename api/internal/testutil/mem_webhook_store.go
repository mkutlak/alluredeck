package testutil

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// Compile-time interface check.
var _ store.WebhookStorer = (*MemWebhookStore)(nil)

// MemWebhookStore is a thread-safe in-memory WebhookStorer for tests.
type MemWebhookStore struct {
	mu         sync.RWMutex
	webhooks   []*store.Webhook
	deliveries []*store.WebhookDelivery
	nextSeq    int // used to generate unique IDs
}

// NewMemWebhookStore returns an initialised MemWebhookStore.
func NewMemWebhookStore() *MemWebhookStore {
	return &MemWebhookStore{nextSeq: 1}
}

func (m *MemWebhookStore) nextID() string {
	id := fmt.Sprintf("wh-%04d", m.nextSeq)
	m.nextSeq++
	return id
}

func (m *MemWebhookStore) Create(ctx context.Context, wh *store.Webhook) (*store.Webhook, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	cp := *wh
	cp.ID = m.nextID()
	cp.CreatedAt = now
	cp.UpdatedAt = now
	if cp.Events == nil {
		cp.Events = []string{}
	}
	m.webhooks = append(m.webhooks, &cp)
	result := cp
	return &result, nil
}

func (m *MemWebhookStore) GetByID(ctx context.Context, webhookID string) (*store.Webhook, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, wh := range m.webhooks {
		if wh.ID == webhookID {
			cp := *wh
			return &cp, nil
		}
	}
	return nil, fmt.Errorf("%w: id=%s", store.ErrWebhookNotFound, webhookID)
}

func (m *MemWebhookStore) List(ctx context.Context, projectID string) ([]store.Webhook, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []store.Webhook
	for _, wh := range m.webhooks {
		if wh.ProjectID == projectID {
			out = append(out, *wh)
		}
	}
	return out, nil
}

func (m *MemWebhookStore) Update(ctx context.Context, wh *store.Webhook) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, stored := range m.webhooks {
		if stored.ID == wh.ID {
			*stored = *wh
			stored.UpdatedAt = time.Now()
			return nil
		}
	}
	return fmt.Errorf("%w: id=%s", store.ErrWebhookNotFound, wh.ID)
}

func (m *MemWebhookStore) Delete(ctx context.Context, webhookID, projectID string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, wh := range m.webhooks {
		if wh.ID == webhookID && wh.ProjectID == projectID {
			m.webhooks = slices.Delete(m.webhooks, i, i+1)
			return nil
		}
	}
	return fmt.Errorf("%w: id=%s", store.ErrWebhookNotFound, webhookID)
}

func (m *MemWebhookStore) ListActiveForEvent(ctx context.Context, projectID, event string) ([]store.Webhook, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []store.Webhook
	for _, wh := range m.webhooks {
		if wh.ProjectID != projectID || !wh.IsActive {
			continue
		}
		if slices.Contains(wh.Events, event) {
			out = append(out, *wh)
		}
	}
	return out, nil
}

func (m *MemWebhookStore) InsertDelivery(ctx context.Context, d *store.WebhookDelivery) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *d
	if cp.ID == "" {
		cp.ID = fmt.Sprintf("dlv-%04d", m.nextSeq)
		m.nextSeq++
	}
	m.deliveries = append(m.deliveries, &cp)
	return nil
}

func (m *MemWebhookStore) ListDeliveries(ctx context.Context, webhookID string, page, perPage int) ([]store.WebhookDelivery, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	var filtered []store.WebhookDelivery
	for _, d := range m.deliveries {
		if d.WebhookID == webhookID {
			filtered = append(filtered, *d)
		}
	}
	total := len(filtered)
	start := (page - 1) * perPage
	if start >= total {
		return []store.WebhookDelivery{}, total, nil
	}
	end := min(start+perPage, total)
	return filtered[start:end], total, nil
}

func (m *MemWebhookStore) PruneDeliveries(ctx context.Context, olderThan time.Time) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var kept []*store.WebhookDelivery
	var pruned int64
	for _, d := range m.deliveries {
		if d.DeliveredAt.Before(olderThan) {
			pruned++
		} else {
			kept = append(kept, d)
		}
	}
	m.deliveries = kept
	return pruned, nil
}
