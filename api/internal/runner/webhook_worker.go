package runner

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/riverqueue/river"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
)

// SendWebhookArgs holds the River job arguments for async webhook delivery.
type SendWebhookArgs struct {
	WebhookID string               `json:"webhook_id"`
	Payload   store.WebhookPayload `json:"payload"`
}

// Kind returns the River job kind identifier.
func (SendWebhookArgs) Kind() string { return "send_webhook" }

// SendWebhookWorker is a River worker that delivers webhook notifications via HTTP POST.
type SendWebhookWorker struct {
	river.WorkerDefaults[SendWebhookArgs]
	webhookStore store.WebhookStorer
	httpClient   *http.Client
	encKey       []byte
	logger       *zap.Logger
}

// Work implements river.Worker. It loads the webhook configuration, renders the
// payload body, delivers it via HTTP POST, and records the delivery attempt.
func (w *SendWebhookWorker) Work(ctx context.Context, job *river.Job[SendWebhookArgs]) error {
	a := job.Args
	start := time.Now()

	// 1. Load webhook from DB.
	wh, err := w.webhookStore.GetByID(ctx, a.WebhookID)
	if err != nil {
		w.logger.Error("webhook worker: webhook not found",
			zap.String("webhook_id", a.WebhookID), zap.Error(err))
		// Don't retry if webhook was deleted.
		return nil
	}

	// 2. Render the message body using the template engine.
	body, contentType, err := RenderWebhookPayload(wh.TargetType, wh.Template, a.Payload)
	if err != nil {
		w.logger.Error("webhook worker: template render failed",
			zap.String("webhook_id", a.WebhookID), zap.Error(err))
		return err // retry
	}

	// 3. Build HTTP request.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, wh.URL, bytes.NewReader(body))
	if err != nil {
		w.logger.Error("webhook worker: create request failed",
			zap.String("webhook_id", a.WebhookID), zap.Error(err))
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("User-Agent", "AllureDeck-Webhook/1.0")

	// 4. HMAC signature for webhooks with a secret.
	if wh.Secret != nil && *wh.Secret != "" {
		mac := hmac.New(sha256.New, []byte(*wh.Secret))
		mac.Write(body)
		sig := hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-AllureDeck-Signature", "sha256="+sig)
	}

	// 5. Execute HTTP POST.
	resp, err := w.httpClient.Do(req)

	// 6. Record delivery.
	delivery := &store.WebhookDelivery{
		WebhookID:   a.WebhookID,
		Event:       a.Payload.Event,
		Attempt:     job.Attempt,
		DeliveredAt: time.Now(),
	}

	payloadJSON, _ := json.Marshal(a.Payload)
	delivery.Payload = string(payloadJSON)

	durationMs := int(time.Since(start).Milliseconds())
	delivery.DurationMs = &durationMs

	if err != nil {
		errMsg := err.Error()
		delivery.Error = &errMsg
		_ = w.webhookStore.InsertDelivery(ctx, delivery)
		w.logger.Warn("webhook worker: delivery failed",
			zap.String("webhook_id", a.WebhookID),
			zap.Int("attempt", job.Attempt),
			zap.Error(err))
		return err // retry
	}
	defer func() { _ = resp.Body.Close() }()

	delivery.StatusCode = &resp.StatusCode

	// Read first 1024 bytes of response body for debugging.
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if len(respBody) > 0 {
		respStr := string(respBody)
		delivery.ResponseBody = &respStr
	}

	_ = w.webhookStore.InsertDelivery(ctx, delivery)

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		w.logger.Info("webhook worker: delivered successfully",
			zap.String("webhook_id", a.WebhookID),
			zap.Int("status_code", resp.StatusCode))
		return nil
	}

	// Non-2xx: retry.
	w.logger.Warn("webhook worker: non-2xx response",
		zap.String("webhook_id", a.WebhookID),
		zap.Int("status_code", resp.StatusCode),
		zap.Int("attempt", job.Attempt))
	return fmt.Errorf("webhook returned status %d", resp.StatusCode)
}
