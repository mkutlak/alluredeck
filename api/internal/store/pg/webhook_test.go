//go:build integration

package pg_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/security"
	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

func newWebhookStore(f *fixture) *pg.WebhookStore {
	return pg.NewWebhookStore(f.s, [][]byte{security.DeriveEncryptionKey("test-encryption-secret")}, zap.NewNop())
}

// TestPGWebhookStore_KeyChange: a webhook saved while the JWT secret was the
// development default stays readable (List, GetByID, ListActiveForEvent, with
// its secret) after the secret changes, and is encrypted under the new key
// once it is updated. A store that knows neither key skips the row.
func TestPGWebhookStore_KeyChange(t *testing.T) {
	f := newFixture(t)
	keysFor := func(jwtSecret string) *pg.WebhookStore {
		return pg.NewWebhookStore(f.s, security.EncryptionKeys(&config.Config{JWTSecret: jwtSecret}), zap.NewNop())
	}
	oldStore, newStore := keysFor(config.DefaultJWTSecret), keysFor("rotated-secret")

	secret := "s3cr3t"
	hook, err := oldStore.Create(f.ctx, &store.Webhook{ProjectID: f.id, Name: "hook", TargetType: "generic",
		URL: "https://example.com/hook", Secret: &secret, Events: []string{"report_completed"}, IsActive: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if got, err := newStore.GetByID(f.ctx, hook.ID); err != nil || got.URL != hook.URL || got.Secret == nil || *got.Secret != secret {
		t.Errorf("GetByID under the new key = %+v, %v; want the URL and secret", got, err)
	}
	if got, err := newStore.List(f.ctx, f.id); err != nil || len(got) != 1 || got[0].URL != hook.URL {
		t.Errorf("List under the new key = %+v, %v; want the webhook", got, err)
	}
	if got, err := newStore.ListActiveForEvent(f.ctx, f.id, "report_completed"); err != nil ||
		len(got) != 1 || got[0].URL != hook.URL || got[0].Secret == nil || *got[0].Secret != secret {
		t.Errorf("ListActiveForEvent under the new key = %+v, %v; want the webhook with its secret", got, err)
	}

	if err := newStore.Update(f.ctx, hook); err != nil {
		t.Fatalf("Update: %v", err)
	}
	// Primary key only: proves the update re-encrypted under it, not under the default.
	primaryOnly := pg.NewWebhookStore(f.s, security.EncryptionKeys(&config.Config{JWTSecret: "rotated-secret"})[:1], zap.NewNop())
	if got, err := primaryOnly.ListActiveForEvent(f.ctx, f.id, "report_completed"); err != nil || len(got) != 1 {
		t.Errorf("ListActiveForEvent with the primary key only = %+v, %v; want the webhook", got, err)
	}
	if got, err := keysFor("unrelated").ListActiveForEvent(f.ctx, f.id, "report_completed"); err != nil || len(got) != 0 {
		t.Errorf("ListActiveForEvent without the key = %+v, %v; want the row skipped", got, err)
	}
}

// TestPGWebhookStore walks one project's webhooks: Create stamps the id and
// timestamps; GetByID decrypts URL and secret (nil when none was set); List
// never returns secrets; ListActiveForEvent keeps only active subscribers of
// the event; Update re-encrypts; Delete is scoped to the owning project (IDOR
// prevention). Unknown ids are ErrWebhookNotFound.
func TestPGWebhookStore(t *testing.T) {
	f := newFixture(t)
	ws := newWebhookStore(f)
	list := func() []store.Webhook {
		t.Helper()
		hooks, err := ws.List(f.ctx, f.id)
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		return hooks
	}
	if hooks := list(); len(hooks) != 0 {
		t.Errorf("List without webhooks = %+v, want none", hooks)
	}

	secret := "s3cr3t"
	created := map[string]*store.Webhook{}
	var active *store.Webhook
	for _, wh := range []store.Webhook{
		{Name: "active", TargetType: "generic", URL: "https://example.com/hook", Secret: &secret, Events: []string{"report_completed"}, IsActive: true},
		{Name: "inactive", TargetType: "generic", URL: "https://example.com/inactive", Events: []string{"report_completed"}},
		{Name: "other event", TargetType: "slack", URL: "https://hooks.slack.com/test", Events: []string{"build_started"}, IsActive: true},
	} {
		wh.ProjectID = f.id
		c, err := ws.Create(f.ctx, &wh)
		if err != nil {
			t.Fatalf("Create %s: %v", wh.Name, err)
		}
		if c.ID == "" || c.CreatedAt.IsZero() || c.UpdatedAt.IsZero() {
			t.Errorf("Create %s = %+v, want the id and timestamps set", wh.Name, c)
		}
		if got, err := ws.GetByID(f.ctx, c.ID); err != nil || !reflect.DeepEqual(got, c) {
			t.Errorf("GetByID(%s) = %+v, %v; want %+v", wh.Name, got, err, c)
		}
		created[c.ID] = c
		if wh.Name == "active" {
			active = c
		}
	}

	hooks := list()
	if len(hooks) != len(created) {
		t.Errorf("List = %d webhooks, want %d", len(hooks), len(created))
	}
	for _, wh := range hooks {
		if c := created[wh.ID]; c == nil || wh.URL != c.URL || wh.Secret != nil {
			t.Errorf("List row %+v: want a created webhook with its URL and no secret", wh)
		}
	}
	if got, err := ws.ListActiveForEvent(f.ctx, f.id, "report_completed"); err != nil ||
		len(got) != 1 || got[0].ID != active.ID || got[0].URL != active.URL {
		t.Errorf("ListActiveForEvent = %+v, %v; want only %s", got, err, active.ID)
	}

	upd := *active
	newSecret := "new-secret"
	upd.Name, upd.URL, upd.Secret, upd.IsActive = "updated-name", "https://updated.example.com/hook", &newSecret, false
	if err := ws.Update(f.ctx, &upd); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := ws.GetByID(f.ctx, upd.ID)
	if err != nil || got.Name != upd.Name || got.URL != upd.URL || got.Secret == nil || *got.Secret != newSecret || got.IsActive {
		t.Errorf("GetByID after Update = %+v, %v; want %+v", got, err, upd)
	}

	const unknown = "00000000-0000-0000-0000-000000000000"
	if _, err := ws.GetByID(f.ctx, unknown); !errors.Is(err, store.ErrWebhookNotFound) {
		t.Errorf("GetByID(unknown) err = %v, want ErrWebhookNotFound", err)
	}
	ghost := upd
	ghost.ID = unknown
	if err := ws.Update(f.ctx, &ghost); !errors.Is(err, store.ErrWebhookNotFound) {
		t.Errorf("Update(unknown) err = %v, want ErrWebhookNotFound", err)
	}

	other := f.newProject(0)
	for _, step := range []struct {
		projectID int64
		want      error
	}{{other.ID, store.ErrWebhookNotFound}, {f.id, nil}, {f.id, store.ErrWebhookNotFound}} {
		if err := ws.Delete(f.ctx, upd.ID, step.projectID); !errors.Is(err, step.want) {
			t.Errorf("Delete(project %d) err = %v, want %v", step.projectID, err, step.want)
		}
	}
	if _, err := ws.GetByID(f.ctx, upd.ID); !errors.Is(err, store.ErrWebhookNotFound) {
		t.Errorf("GetByID after Delete err = %v, want ErrWebhookNotFound", err)
	}
}

// TestPGWebhookStore_Deliveries: ListDeliveries pages a webhook's deliveries
// with the total alongside (empty before the first), and PruneDeliveries
// deletes only deliveries older than the cutoff.
func TestPGWebhookStore_Deliveries(t *testing.T) {
	f := newFixture(t)
	ws := newWebhookStore(f)
	hook, err := ws.Create(f.ctx, &store.Webhook{ProjectID: f.id, Name: "hook", TargetType: "generic",
		URL: "https://example.com/hook", Events: []string{"report_completed"}, IsActive: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	page := func(perPage, wantLen, wantTotal int) {
		t.Helper()
		ds, total, err := ws.ListDeliveries(f.ctx, hook.ID, 1, perPage)
		if err != nil || len(ds) != wantLen || total != wantTotal {
			t.Errorf("ListDeliveries(perPage %d) = %d rows, total %d, %v; want %d, %d", perPage, len(ds), total, err, wantLen, wantTotal)
		}
	}
	page(10, 0, 0)

	status, body, duration := 200, `{"ok":true}`, 42
	for _, d := range []*store.WebhookDelivery{
		{Attempt: 1, StatusCode: &status, ResponseBody: &body, DurationMs: &duration},
		{Attempt: 2},
	} {
		d.WebhookID, d.Event, d.Payload, d.DeliveredAt = hook.ID, "report_completed", `{"event":"report_completed"}`, time.Now().UTC()
		if err := ws.InsertDelivery(f.ctx, d); err != nil || d.ID == "" {
			t.Fatalf("InsertDelivery attempt %d: id %q, %v", d.Attempt, d.ID, err)
		}
	}
	page(10, 2, 2)
	page(1, 1, 2)

	for _, step := range []struct {
		cutoff time.Time
		want   int64
	}{{time.Now().Add(-24 * time.Hour), 0}, {time.Now().Add(time.Hour), 2}} {
		if n, err := ws.PruneDeliveries(f.ctx, step.cutoff); err != nil || n != step.want {
			t.Errorf("PruneDeliveries(%v) = %d, %v; want %d", step.cutoff, n, err, step.want)
		}
	}
	page(10, 0, 0)
}
