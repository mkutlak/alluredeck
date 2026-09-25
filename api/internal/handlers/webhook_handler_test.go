package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// webhookFixture seeds project 1 with a Slack webhook "{a}" (a secret in its
// URL path and five deliveries) and a generic webhook "{b}", and project 2 with
// generic webhook "{c}".
type webhookFixture struct {
	h     *WebhookHandler
	store *testutil.MemWebhookStore
	ids   *strings.Replacer // "{a}", "{b}", "{c}" → seeded webhook IDs
}

func newWebhookFixture(t *testing.T) *webhookFixture {
	t.Helper()
	ctx := context.Background()
	whs := testutil.NewMemWebhookStore()
	seed := func(projectID int64, name string, target store.WebhookTargetType, url string) string {
		wh, err := whs.Create(ctx, &store.Webhook{ProjectID: projectID, Name: name, TargetType: target, URL: url, Events: []string{"report_completed"}, IsActive: true})
		if err != nil {
			t.Fatal(err)
		}
		return wh.ID
	}
	a := seed(1, "slack-notify", "slack", "https://hooks.slack.com/services/T00/B00/secret")
	b := seed(1, "generic-hook", "generic", "https://example.com/hook")
	c := seed(2, "other-project", "generic", "https://example.com/hook")
	for i := range 5 {
		if err := whs.InsertDelivery(ctx, &store.WebhookDelivery{ID: fmt.Sprintf("dlv-%d", i), WebhookID: a, Event: "report_completed", Payload: "{}", Attempt: 1, DeliveredAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	return &webhookFixture{
		h:     NewWebhookHandler(whs, testutil.NewMemProjectStore(), zap.NewNop()),
		store: whs,
		ids:   strings.NewReplacer("{a}", a, "{b}", b, "{c}", c),
	}
}

// TestWebhookHandler drives every webhook endpoint. Webhooks of another
// project are indistinguishable from missing ones (IDOR).
func TestWebhookHandler(t *testing.T) {
	list := (*WebhookHandler).List
	create := (*WebhookHandler).Create
	get := (*WebhookHandler).Get
	update := (*WebhookHandler).Update
	del := (*WebhookHandler).Delete
	test := (*WebhookHandler).Test
	deliveries := (*WebhookHandler).ListDeliveries
	createBody := func(target, url, extra string) string {
		return `{"name":"wh","target_type":"` + target + `","url":"` + url + `"` + extra + `}`
	}
	tests := []struct {
		name     string
		serve    func(*WebhookHandler, http.ResponseWriter, *http.Request)
		project  string
		webhook  string // webhook_id path value; "{a}".."{c}" are the seeded ones
		query    string
		body     string
		setup    func(t *testing.T, f *webhookFixture)
		want     int
		wantJSON map[string]any
	}{
		// URLs are masked to scheme+host and secrets never leave the server.
		{name: "list masks urls", serve: list, project: "1", want: http.StatusOK, wantJSON: map[string]any{
			"data#": 2, "data.0.url": "https://hooks.slack.com/****", "data.0.secret": nil,
		}},
		{name: "list empty project", serve: list, project: "3", want: http.StatusOK, wantJSON: map[string]any{"data#": 0}},
		{name: "create applies defaults", serve: create, project: "3", body: `{"name":"my-webhook","target_type":"slack","url":"https://hooks.slack.com/services/T00/B00/abc"}`,
			want: http.StatusCreated, wantJSON: map[string]any{
				"data.name": "my-webhook", "data.target_type": "slack", "data.events#": 1, "data.events.0": "report_completed", "data.is_active": true,
			}},
		{name: "create missing name", serve: create, project: "3", body: `{"target_type":"slack","url":"https://example.com/hook"}`, want: http.StatusBadRequest},
		{name: "create invalid target type", serve: create, project: "3", body: createBody("unknown", "https://example.com/hook", ""), want: http.StatusBadRequest},
		{name: "create invalid event", serve: create, project: "3", body: createBody("slack", "https://example.com/hook", `,"events":["report_completed","bogus_event"]`), want: http.StatusBadRequest},
		{name: "create non-http url", serve: create, project: "3", body: createBody("generic", "ftp://example.com/hook", ""), want: http.StatusBadRequest},
		// SSRF: loopback targets are refused, including a bracketed IPv6
		// literal, which Go 1.26's stricter url.Parse still accepts.
		{name: "create loopback url", serve: create, project: "3", body: createBody("generic", "http://127.0.0.1/hook", ""), want: http.StatusBadRequest},
		{name: "create ipv6 loopback url", serve: create, project: "3", body: createBody("generic", "http://[::1]/hook", ""), want: http.StatusBadRequest,
			wantJSON: map[string]any{"metadata.message": "URL must not point to private or loopback addresses"}},
		{name: "create over the limit", serve: create, project: "1", body: createBody("generic", "https://example.com/hook", ""), want: http.StatusConflict,
			setup: func(t *testing.T, f *webhookFixture) {
				for i := 2; i < maxWebhooksPerProject; i++ {
					if _, err := f.store.Create(context.Background(), &store.Webhook{ProjectID: 1, Name: fmt.Sprintf("wh-%d", i), TargetType: "generic", URL: "https://example.com/hook"}); err != nil {
						t.Fatal(err)
					}
				}
			}},
		{name: "get", serve: get, project: "1", webhook: "{b}", want: http.StatusOK, wantJSON: map[string]any{"data.id": "{b}", "data.name": "generic-hook"}},
		{name: "get missing", serve: get, project: "1", webhook: "nonexistent", want: http.StatusNotFound},
		{name: "get other project's", serve: get, project: "2", webhook: "{b}", want: http.StatusNotFound},
		{name: "update is partial", serve: update, project: "1", webhook: "{b}", body: `{"name":"updated-name"}`, want: http.StatusOK,
			wantJSON: map[string]any{"data.name": "updated-name", "data.target_type": "generic"}},
		{name: "update loopback url", serve: update, project: "1", webhook: "{b}", body: `{"url":"http://localhost/evil"}`, want: http.StatusBadRequest},
		{name: "update missing", serve: update, project: "1", webhook: "nonexistent", body: `{"name":"new"}`, want: http.StatusNotFound},
		{name: "delete", serve: del, project: "1", webhook: "{b}", want: http.StatusOK},
		{name: "delete missing", serve: del, project: "1", webhook: "nonexistent", want: http.StatusNotFound},
		{name: "delete other project's", serve: del, project: "2", webhook: "{b}", want: http.StatusNotFound},
		{name: "test delivery", serve: test, project: "1", webhook: "{b}", want: http.StatusOK, wantJSON: map[string]any{"data.message": "test notification queued"}},
		{name: "test missing", serve: test, project: "1", webhook: "nonexistent", want: http.StatusNotFound},
		{name: "deliveries empty", serve: deliveries, project: "1", webhook: "{b}", want: http.StatusOK, wantJSON: map[string]any{"data#": 0}},
		{name: "deliveries paginated", serve: deliveries, project: "1", webhook: "{a}", query: "page=1&per_page=3", want: http.StatusOK,
			wantJSON: map[string]any{"data#": 3, "pagination.total": 5, "pagination.page": 1}},
		{name: "deliveries missing webhook", serve: deliveries, project: "1", webhook: "nonexistent", want: http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newWebhookFixture(t)
			if tc.setup != nil {
				tc.setup(t, f)
			}
			// Handler methods are called directly, so the HTTP method is moot.
			fn := func(w http.ResponseWriter, r *http.Request) { tc.serve(f.h, w, r) }
			code, body := serveJSON(t, fn, http.MethodPost, "/api/v1/projects/webhooks?"+tc.query, tc.body,
				"project_id", tc.project, "webhook_id", f.ids.Replace(tc.webhook))
			if code != tc.want {
				t.Fatalf("status = %d, want %d: %v", code, tc.want, body)
			}
			want := make(map[string]any, len(tc.wantJSON))
			for path, v := range tc.wantJSON {
				if s, ok := v.(string); ok {
					v = f.ids.Replace(s)
				}
				want[path] = v
			}
			wantJSON(t, body, want)
		})
	}
}
