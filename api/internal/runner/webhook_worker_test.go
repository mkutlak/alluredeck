package runner

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// newTestJob constructs a minimal river.Job for unit testing without a live River client.
func newTestJob(args SendWebhookArgs, attempt int) *river.Job[SendWebhookArgs] {
	return &river.Job[SendWebhookArgs]{
		JobRow: &rivertype.JobRow{Attempt: attempt, CreatedAt: time.Now()},
		Args:   args,
	}
}

// newTestWorker creates a SendWebhookWorker with the given store and HTTP client.
func newTestWorker(ws *testutil.MemWebhookStore, client *http.Client) *SendWebhookWorker {
	return &SendWebhookWorker{webhookStore: ws, httpClient: client, logger: zap.NewNop()}
}

func samplePayload(event string) WebhookPayload {
	return WebhookPayload{
		Event: event, ProjectID: 1, BuildNumber: 1, Timestamp: time.Now(),
		Stats: WebhookStats{Total: 10, Passed: 9, Failed: 1, PassRate: 90.0},
	}
}

// TestSendWebhookWorker_Work verifies delivery: every response is recorded,
// a non-2xx makes Work fail so River retries, the body is posted as JSON and —
// when the webhook has a secret — signed with an HMAC-SHA256 of the exact body
// in X-AllureDeck-Signature. A webhook deleted before delivery is dropped
// without a retry.
func TestSendWebhookWorker_Work(t *testing.T) {
	secret := "supersecret"
	tests := []struct {
		name    string
		status  int
		secret  *string
		missing bool
		wantErr bool
	}{
		{name: "2xx is recorded and signed", status: http.StatusOK, secret: &secret},
		{name: "non-2xx is recorded and retried", status: http.StatusInternalServerError, wantErr: true},
		{name: "unknown webhook is dropped", missing: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var gotSig, gotContentType string
			var gotBody []byte
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotSig, gotContentType = r.Header.Get("X-AllureDeck-Signature"), r.Header.Get("Content-Type")
				gotBody, _ = io.ReadAll(r.Body)
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()

			ws := testutil.NewMemWebhookStore()
			webhookID := "nonexistent-id"
			if !tc.missing {
				created, err := ws.Create(context.Background(), &store.Webhook{
					ProjectID: 1, Name: "test", TargetType: "generic", URL: srv.URL, Secret: tc.secret,
					IsActive: true, Events: []string{"report_completed"},
				})
				if err != nil {
					t.Fatalf("create webhook: %v", err)
				}
				webhookID = created.ID
			}

			err := newTestWorker(ws, srv.Client()).Work(context.Background(),
				newTestJob(SendWebhookArgs{WebhookID: webhookID, Payload: samplePayload("report_completed")}, 1))
			if (err != nil) != tc.wantErr {
				t.Fatalf("Work error = %v, want error: %v", err, tc.wantErr)
			}
			if tc.missing {
				return
			}

			deliveries, total, err := ws.ListDeliveries(context.Background(), webhookID, 1, 10)
			if err != nil {
				t.Fatalf("list deliveries: %v", err)
			}
			if total != 1 || deliveries[0].StatusCode == nil || *deliveries[0].StatusCode != tc.status {
				t.Fatalf("want one delivery recording status %d, got %d: %+v", tc.status, total, deliveries)
			}
			if !strings.HasPrefix(gotContentType, "application/json") {
				t.Errorf("Content-Type = %q, want application/json", gotContentType)
			}
			if tc.secret != nil {
				mac := hmac.New(sha256.New, []byte(*tc.secret))
				mac.Write(gotBody)
				if want := "sha256=" + hex.EncodeToString(mac.Sum(nil)); gotSig != want {
					t.Errorf("signature = %q, want %q", gotSig, want)
				}
			}
		})
	}
}
