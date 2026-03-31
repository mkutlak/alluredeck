//go:build integration

package pg_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// testEncKey is a fixed 32-byte key for integration tests.
var testEncKey = security.DeriveEncryptionKey("test-encryption-secret")

// webhookProjectID returns a unique project ID string for each test run.
func webhookProjectID() string {
	return fmt.Sprintf("wh-test-%d", time.Now().UnixNano())
}

// ensureProject inserts a project row so FK constraints are satisfied.
func ensureProject(t *testing.T, s *pg.Store, projectID string) {
	t.Helper()
	ctx := context.Background()
	_, err := s.Pool().Exec(ctx,
		"INSERT INTO projects (id) VALUES ($1) ON CONFLICT DO NOTHING", projectID)
	if err != nil {
		t.Fatalf("ensureProject %s: %v", projectID, err)
	}
}

func newTestWebhook(projectID string) *store.Webhook {
	secret := "s3cr3t"
	return &store.Webhook{
		ProjectID:  projectID,
		Name:       fmt.Sprintf("hook-%d", time.Now().UnixNano()),
		TargetType: "generic",
		URL:        "https://example.com/hook",
		Secret:     &secret,
		Events:     []string{"report_completed"},
		IsActive:   true,
	}
}

func isWebhookNotFound(err error) bool {
	return errors.Is(err, store.ErrWebhookNotFound)
}

// ---------------------------------------------------------------------------
// Create / GetByID
// ---------------------------------------------------------------------------

func TestPGWebhookStore_CreateAndGetByID(t *testing.T) {
	s := openLockTestStore(t)
	ws := pg.NewWebhookStore(s, testEncKey, zap.NewNop())
	ctx := context.Background()

	projectID := webhookProjectID()
	ensureProject(t, s, projectID)

	wh := newTestWebhook(projectID)
	created, err := ws.Create(ctx, wh)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Error("expected non-empty ID after Create")
	}
	if created.CreatedAt.IsZero() {
		t.Error("expected non-zero CreatedAt after Create")
	}
	if created.UpdatedAt.IsZero() {
		t.Error("expected non-zero UpdatedAt after Create")
	}

	got, err := ws.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ID != created.ID {
		t.Errorf("ID = %s, want %s", got.ID, created.ID)
	}
	if got.URL != "https://example.com/hook" {
		t.Errorf("URL = %s, want https://example.com/hook", got.URL)
	}
	if got.Secret == nil || *got.Secret != "s3cr3t" {
		t.Error("expected decrypted secret to match")
	}
	if len(got.Events) != 1 || got.Events[0] != "report_completed" {
		t.Errorf("Events = %v, want [report_completed]", got.Events)
	}
}

func TestPGWebhookStore_GetByID_NotFound(t *testing.T) {
	s := openLockTestStore(t)
	ws := pg.NewWebhookStore(s, testEncKey, zap.NewNop())
	ctx := context.Background()

	_, err := ws.GetByID(ctx, "00000000-0000-0000-0000-000000000000")
	if !isWebhookNotFound(err) {
		t.Errorf("expected ErrWebhookNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Create without Secret
// ---------------------------------------------------------------------------

func TestPGWebhookStore_Create_NoSecret(t *testing.T) {
	s := openLockTestStore(t)
	ws := pg.NewWebhookStore(s, testEncKey, zap.NewNop())
	ctx := context.Background()

	projectID := webhookProjectID()
	ensureProject(t, s, projectID)

	wh := &store.Webhook{
		ProjectID:  projectID,
		Name:       "no-secret-hook",
		TargetType: "slack",
		URL:        "https://hooks.slack.com/test",
		Secret:     nil,
		Events:     []string{"report_completed"},
		IsActive:   true,
	}
	created, err := ws.Create(ctx, wh)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := ws.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Secret != nil {
		t.Error("expected nil Secret for webhook created without secret")
	}
}

// ---------------------------------------------------------------------------
// List
// ---------------------------------------------------------------------------

func TestPGWebhookStore_List(t *testing.T) {
	s := openLockTestStore(t)
	ws := pg.NewWebhookStore(s, testEncKey, zap.NewNop())
	ctx := context.Background()

	projectID := webhookProjectID()
	ensureProject(t, s, projectID)

	for i := range 3 {
		wh := newTestWebhook(projectID)
		wh.Name = fmt.Sprintf("hook-%d", i)
		if _, err := ws.Create(ctx, wh); err != nil {
			t.Fatalf("Create hook %d: %v", i, err)
		}
	}

	list, err := ws.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 3 {
		t.Errorf("expected 3 webhooks, got %d", len(list))
	}
	// Secrets are not returned by List.
	for _, wh := range list {
		if wh.Secret != nil {
			t.Errorf("expected nil Secret in list response for webhook %s", wh.ID)
		}
		if wh.URL == "" {
			t.Errorf("expected non-empty URL in list response for webhook %s", wh.ID)
		}
	}
}

func TestPGWebhookStore_List_Empty(t *testing.T) {
	s := openLockTestStore(t)
	ws := pg.NewWebhookStore(s, testEncKey, zap.NewNop())
	ctx := context.Background()

	projectID := webhookProjectID()
	ensureProject(t, s, projectID)

	list, err := ws.List(ctx, projectID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %d items", len(list))
	}
}

// ---------------------------------------------------------------------------
// Update
// ---------------------------------------------------------------------------

func TestPGWebhookStore_Update(t *testing.T) {
	s := openLockTestStore(t)
	ws := pg.NewWebhookStore(s, testEncKey, zap.NewNop())
	ctx := context.Background()

	projectID := webhookProjectID()
	ensureProject(t, s, projectID)

	wh := newTestWebhook(projectID)
	created, err := ws.Create(ctx, wh)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	newSecret := "new-secret"
	created.Name = "updated-name"
	created.URL = "https://updated.example.com/hook"
	created.Secret = &newSecret
	created.IsActive = false

	if err := ws.Update(ctx, created); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := ws.GetByID(ctx, created.ID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if got.Name != "updated-name" {
		t.Errorf("Name = %s, want updated-name", got.Name)
	}
	if got.URL != "https://updated.example.com/hook" {
		t.Errorf("URL = %s, want updated URL", got.URL)
	}
	if got.Secret == nil || *got.Secret != "new-secret" {
		t.Error("expected updated secret")
	}
	if got.IsActive {
		t.Error("expected IsActive = false after update")
	}
}

func TestPGWebhookStore_Update_NotFound(t *testing.T) {
	s := openLockTestStore(t)
	ws := pg.NewWebhookStore(s, testEncKey, zap.NewNop())
	ctx := context.Background()

	wh := &store.Webhook{
		ID:         "00000000-0000-0000-0000-000000000000",
		ProjectID:  "nonexistent",
		Name:       "ghost",
		TargetType: "generic",
		URL:        "https://example.com",
		Events:     []string{"report_completed"},
		IsActive:   true,
	}
	if err := ws.Update(ctx, wh); !isWebhookNotFound(err) {
		t.Errorf("expected ErrWebhookNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Delete
// ---------------------------------------------------------------------------

func TestPGWebhookStore_Delete(t *testing.T) {
	s := openLockTestStore(t)
	ws := pg.NewWebhookStore(s, testEncKey, zap.NewNop())
	ctx := context.Background()

	projectID := webhookProjectID()
	ensureProject(t, s, projectID)

	wh := newTestWebhook(projectID)
	created, err := ws.Create(ctx, wh)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Wrong project → not found.
	if err := ws.Delete(ctx, created.ID, "other-project"); !isWebhookNotFound(err) {
		t.Errorf("expected ErrWebhookNotFound for wrong project, got %v", err)
	}

	// Correct project → success.
	if err := ws.Delete(ctx, created.ID, projectID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	// Second delete → not found.
	if err := ws.Delete(ctx, created.ID, projectID); !isWebhookNotFound(err) {
		t.Errorf("expected ErrWebhookNotFound after deletion, got %v", err)
	}

	// GetByID → not found.
	if _, err := ws.GetByID(ctx, created.ID); !isWebhookNotFound(err) {
		t.Errorf("expected ErrWebhookNotFound after deletion, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// ListActiveForEvent
// ---------------------------------------------------------------------------

func TestPGWebhookStore_ListActiveForEvent(t *testing.T) {
	s := openLockTestStore(t)
	ws := pg.NewWebhookStore(s, testEncKey, zap.NewNop())
	ctx := context.Background()

	projectID := webhookProjectID()
	ensureProject(t, s, projectID)

	// Active webhook for report_completed.
	active := newTestWebhook(projectID)
	active.Events = []string{"report_completed"}
	active.IsActive = true
	if _, err := ws.Create(ctx, active); err != nil {
		t.Fatalf("Create active: %v", err)
	}

	// Inactive webhook.
	inactive := newTestWebhook(projectID)
	inactive.Events = []string{"report_completed"}
	inactive.IsActive = false
	if _, err := ws.Create(ctx, inactive); err != nil {
		t.Fatalf("Create inactive: %v", err)
	}

	// Active but different event.
	other := newTestWebhook(projectID)
	other.Events = []string{"build_started"}
	other.IsActive = true
	if _, err := ws.Create(ctx, other); err != nil {
		t.Fatalf("Create other-event: %v", err)
	}

	list, err := ws.ListActiveForEvent(ctx, projectID, "report_completed")
	if err != nil {
		t.Fatalf("ListActiveForEvent: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("expected 1 active webhook for report_completed, got %d", len(list))
	}
	if list[0].URL == "" {
		t.Error("expected non-empty URL in ListActiveForEvent result")
	}
}

// ---------------------------------------------------------------------------
// InsertDelivery / ListDeliveries / PruneDeliveries
// ---------------------------------------------------------------------------

func TestPGWebhookStore_DeliveryLifecycle(t *testing.T) {
	s := openLockTestStore(t)
	ws := pg.NewWebhookStore(s, testEncKey, zap.NewNop())
	ctx := context.Background()

	projectID := webhookProjectID()
	ensureProject(t, s, projectID)

	wh := newTestWebhook(projectID)
	created, err := ws.Create(ctx, wh)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	statusCode := 200
	respBody := `{"ok":true}`
	durationMs := 42

	delivery := &store.WebhookDelivery{
		WebhookID:    created.ID,
		Event:        "report_completed",
		Payload:      `{"event":"report_completed"}`,
		StatusCode:   &statusCode,
		ResponseBody: &respBody,
		Attempt:      1,
		DurationMs:   &durationMs,
		DeliveredAt:  time.Now().UTC(),
	}
	if err := ws.InsertDelivery(ctx, delivery); err != nil {
		t.Fatalf("InsertDelivery: %v", err)
	}
	if delivery.ID == "" {
		t.Error("expected non-empty ID after InsertDelivery")
	}

	// Insert a second delivery.
	delivery2 := &store.WebhookDelivery{
		WebhookID:   created.ID,
		Event:       "report_completed",
		Payload:     `{"event":"report_completed"}`,
		Attempt:     2,
		DeliveredAt: time.Now().UTC(),
	}
	if err := ws.InsertDelivery(ctx, delivery2); err != nil {
		t.Fatalf("InsertDelivery 2: %v", err)
	}

	// ListDeliveries — page 1, 10 per page.
	deliveries, total, err := ws.ListDeliveries(ctx, created.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListDeliveries: %v", err)
	}
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if len(deliveries) != 2 {
		t.Errorf("len(deliveries) = %d, want 2", len(deliveries))
	}

	// ListDeliveries — pagination: page 1, 1 per page.
	page1, total1, err := ws.ListDeliveries(ctx, created.ID, 1, 1)
	if err != nil {
		t.Fatalf("ListDeliveries page1: %v", err)
	}
	if total1 != 2 {
		t.Errorf("total1 = %d, want 2", total1)
	}
	if len(page1) != 1 {
		t.Errorf("len(page1) = %d, want 1", len(page1))
	}

	// PruneDeliveries — prune nothing (cutoff in the past).
	n, err := ws.PruneDeliveries(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("PruneDeliveries (noop): %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 pruned, got %d", n)
	}

	// PruneDeliveries — prune all (cutoff in the future).
	n, err = ws.PruneDeliveries(ctx, time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("PruneDeliveries (all): %v", err)
	}
	if n != 2 {
		t.Errorf("expected 2 pruned, got %d", n)
	}

	// Verify empty after prune.
	deliveries, total, err = ws.ListDeliveries(ctx, created.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListDeliveries after prune: %v", err)
	}
	if total != 0 || len(deliveries) != 0 {
		t.Errorf("expected empty deliveries after prune, got total=%d len=%d", total, len(deliveries))
	}
}

func TestPGWebhookStore_ListDeliveries_Empty(t *testing.T) {
	s := openLockTestStore(t)
	ws := pg.NewWebhookStore(s, testEncKey, zap.NewNop())
	ctx := context.Background()

	projectID := webhookProjectID()
	ensureProject(t, s, projectID)

	wh := newTestWebhook(projectID)
	created, err := ws.Create(ctx, wh)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	deliveries, total, err := ws.ListDeliveries(ctx, created.ID, 1, 10)
	if err != nil {
		t.Fatalf("ListDeliveries: %v", err)
	}
	if total != 0 {
		t.Errorf("expected total=0, got %d", total)
	}
	if len(deliveries) != 0 {
		t.Errorf("expected empty slice, got %d items", len(deliveries))
	}
}
