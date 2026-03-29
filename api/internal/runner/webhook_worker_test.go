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
		JobRow: &rivertype.JobRow{
			Attempt:   attempt,
			CreatedAt: time.Now(),
		},
		Args: args,
	}
}

// newTestWorker creates a SendWebhookWorker with the given store and HTTP client.
func newTestWorker(ws *testutil.MemWebhookStore, client *http.Client) *SendWebhookWorker {
	logger, _ := zap.NewDevelopment()
	return &SendWebhookWorker{
		webhookStore: ws,
		httpClient:   client,
		logger:       logger,
	}
}

func samplePayload(event string) store.WebhookPayload {
	return store.WebhookPayload{
		Event:      event,
		ProjectID:  "test-project",
		BuildOrder: 1,
		Stats: store.WebhookStats{
			Total:    10,
			Passed:   9,
			Failed:   1,
			PassRate: 90.0,
		},
		Timestamp: time.Now(),
	}
}

// TestSendWebhookWorker_Success verifies a 200 response is recorded and Work returns nil.
func TestSendWebhookWorker_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	ws := testutil.NewMemWebhookStore()
	wh := &store.Webhook{
		ProjectID:  "test-project",
		Name:       "test",
		TargetType: "generic",
		URL:        srv.URL,
		IsActive:   true,
		Events:     []string{"report_completed"},
	}
	created, err := ws.Create(context.Background(), wh)
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	worker := newTestWorker(ws, srv.Client())
	job := newTestJob(SendWebhookArgs{
		WebhookID: created.ID,
		Payload:   samplePayload("report_completed"),
	}, 1)

	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatalf("Work returned unexpected error: %v", err)
	}

	deliveries, total, err := ws.ListDeliveries(context.Background(), created.ID, 1, 10)
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 delivery, got %d", total)
	}
	if deliveries[0].StatusCode == nil {
		t.Fatal("delivery StatusCode is nil")
	}
	if *deliveries[0].StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", *deliveries[0].StatusCode)
	}
}

// TestSendWebhookWorker_NonSuccess verifies a 500 response causes Work to return an error
// (so River will retry) and the delivery is still recorded.
func TestSendWebhookWorker_NonSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	ws := testutil.NewMemWebhookStore()
	wh := &store.Webhook{
		ProjectID:  "test-project",
		Name:       "test",
		TargetType: "generic",
		URL:        srv.URL,
		IsActive:   true,
		Events:     []string{"report_completed"},
	}
	created, err := ws.Create(context.Background(), wh)
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	worker := newTestWorker(ws, srv.Client())
	job := newTestJob(SendWebhookArgs{
		WebhookID: created.ID,
		Payload:   samplePayload("report_completed"),
	}, 1)

	if err := worker.Work(context.Background(), job); err == nil {
		t.Fatal("Work should return an error for non-2xx response")
	}

	deliveries, total, err := ws.ListDeliveries(context.Background(), created.ID, 1, 10)
	if err != nil {
		t.Fatalf("list deliveries: %v", err)
	}
	if total != 1 {
		t.Fatalf("expected 1 delivery recorded, got %d", total)
	}
	if deliveries[0].StatusCode == nil {
		t.Fatal("delivery StatusCode is nil")
	}
	if *deliveries[0].StatusCode != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", *deliveries[0].StatusCode)
	}
}

// TestSendWebhookWorker_WebhookNotFound verifies Work returns nil (no retry) when the
// webhook does not exist in the store.
func TestSendWebhookWorker_WebhookNotFound(t *testing.T) {
	ws := testutil.NewMemWebhookStore()
	worker := newTestWorker(ws, &http.Client{})
	job := newTestJob(SendWebhookArgs{
		WebhookID: "nonexistent-id",
		Payload:   samplePayload("report_completed"),
	}, 1)

	if err := worker.Work(context.Background(), job); err != nil {
		t.Errorf("Work should return nil for missing webhook, got: %v", err)
	}
}

// TestSendWebhookWorker_HMACSignature verifies that when a secret is set the
// X-AllureDeck-Signature header is sent with the correct HMAC-SHA256 value.
func TestSendWebhookWorker_HMACSignature(t *testing.T) {
	const secret = "supersecret"
	var receivedSig string
	var receivedBody []byte

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSig = r.Header.Get("X-AllureDeck-Signature")
		var err error
		receivedBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ws := testutil.NewMemWebhookStore()
	sec := secret
	wh := &store.Webhook{
		ProjectID:  "test-project",
		Name:       "test",
		TargetType: "generic",
		URL:        srv.URL,
		Secret:     &sec,
		IsActive:   true,
		Events:     []string{"report_completed"},
	}
	created, err := ws.Create(context.Background(), wh)
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	worker := newTestWorker(ws, srv.Client())
	job := newTestJob(SendWebhookArgs{
		WebhookID: created.ID,
		Payload:   samplePayload("report_completed"),
	}, 1)

	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatalf("Work returned error: %v", err)
	}

	if receivedSig == "" {
		t.Fatal("X-AllureDeck-Signature header was not set")
	}

	// Compute expected signature over the body received by the server.
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(receivedBody)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	if receivedSig != expected {
		t.Errorf("signature mismatch\n  got:  %s\n  want: %s", receivedSig, expected)
	}
}

// TestSendWebhookWorker_ContentType verifies Content-Type and User-Agent headers are set.
func TestSendWebhookWorker_ContentType(t *testing.T) {
	var receivedContentType, receivedUserAgent string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedContentType = r.Header.Get("Content-Type")
		receivedUserAgent = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ws := testutil.NewMemWebhookStore()
	wh := &store.Webhook{
		ProjectID:  "test-project",
		Name:       "test",
		TargetType: "generic",
		URL:        srv.URL,
		IsActive:   true,
		Events:     []string{"report_completed"},
	}
	created, err := ws.Create(context.Background(), wh)
	if err != nil {
		t.Fatalf("create webhook: %v", err)
	}

	worker := newTestWorker(ws, srv.Client())
	job := newTestJob(SendWebhookArgs{
		WebhookID: created.ID,
		Payload:   samplePayload("report_completed"),
	}, 1)

	if err := worker.Work(context.Background(), job); err != nil {
		t.Fatalf("Work returned error: %v", err)
	}

	if !strings.HasPrefix(receivedContentType, "application/json") {
		t.Errorf("expected Content-Type application/json, got %q", receivedContentType)
	}
	if receivedUserAgent != "AllureDeck-Webhook/1.0" {
		t.Errorf("expected User-Agent AllureDeck-Webhook/1.0, got %q", receivedUserAgent)
	}
}
