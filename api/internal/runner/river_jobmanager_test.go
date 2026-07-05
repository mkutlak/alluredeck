package runner

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// statPtr is a small helper to build *int stat fields inline.
func statPtr(v int) *int { return &v }

// TestBuildWebhookPayload_RegressionDetected_FiresOnRegressions verifies that
// regression_detected is triggered when DefectReader.ListRegressionsForBuild
// returns a non-empty slice for the build, and that the regressions are
// mapped onto payload.Regressions.
func TestBuildWebhookPayload_RegressionDetected_FiresOnRegressions(t *testing.T) {
	buildStore := &testutil.MockBuildStore{
		GetLatestBuildFn: func(_ context.Context, projectID int64) (store.Build, error) {
			return store.Build{
				ID:          100,
				ProjectID:   projectID,
				BuildNumber: 5,
				StatTotal:   statPtr(10),
				StatPassed:  statPtr(10),
				StatFailed:  statPtr(0),
			}, nil
		},
	}
	defectStore := testutil.NewMemDefectStore()
	defectStore.SeedRegressionsForBuild(1, 100, []store.DefectRegression{
		{ID: "fp-1", NormalizedMessage: "boom", Category: "product_bug", OccurrenceCount: 4, BuildOrder: 5},
	})

	payload, triggered, err := buildWebhookPayload(context.Background(), 1, buildStore, defectStore, "", zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !triggered[store.WebhookEventRegressionDetected] {
		t.Error("expected regression_detected to be triggered when ListRegressionsForBuild returns regressions")
	}
	if len(payload.Regressions) != 1 {
		t.Fatalf("expected 1 regression in payload, got %d", len(payload.Regressions))
	}
	got := payload.Regressions[0]
	if got.FingerprintID != "fp-1" || got.Message != "boom" || got.Category != "product_bug" || got.OccurrenceCount != 4 {
		t.Errorf("unexpected regression mapping: %+v", got)
	}
}

// TestBuildWebhookPayload_RegressionDetected_NotFiredByDeltaAlone verifies the
// F3.4 semantics change: a build with new failures (Delta.NewFailures > 0)
// but NO rows from ListRegressionsForBuild must NOT trigger regression_detected.
// This locks in the fix for the old (incorrect) delta-based trigger.
func TestBuildWebhookPayload_RegressionDetected_NotFiredByDeltaAlone(t *testing.T) {
	buildStore := &testutil.MockBuildStore{
		GetLatestBuildFn: func(_ context.Context, projectID int64) (store.Build, error) {
			return store.Build{
				ID:          200,
				ProjectID:   projectID,
				BuildNumber: 6,
				StatTotal:   statPtr(10),
				StatPassed:  statPtr(5),
				StatFailed:  statPtr(5),
			}, nil
		},
		GetPreviousBuildFn: func(_ context.Context, projectID int64, _ int) (store.Build, error) {
			return store.Build{
				ID:          100,
				ProjectID:   projectID,
				BuildNumber: 5,
				StatTotal:   statPtr(10),
				StatPassed:  statPtr(10),
				StatFailed:  statPtr(0),
			}, nil
		},
	}
	// No regressions seeded — ListRegressionsForBuild returns empty.
	defectStore := testutil.NewMemDefectStore()

	payload, triggered, err := buildWebhookPayload(context.Background(), 1, buildStore, defectStore, "", zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if payload.Delta == nil || payload.Delta.NewFailures <= 0 {
		t.Fatalf("test setup invalid: expected positive Delta.NewFailures, got %+v", payload.Delta)
	}
	if triggered[store.WebhookEventRegressionDetected] {
		t.Error("regression_detected must not fire from Delta.NewFailures alone (old semantics) when no regressions were detected")
	}
	if len(payload.Regressions) != 0 {
		t.Errorf("expected no regressions in payload, got %d", len(payload.Regressions))
	}
	// report_failed must still fire since StatFailed > 0.
	if !triggered[store.WebhookEventReportFailed] {
		t.Error("expected report_failed to still be triggered")
	}
}

// TestBuildWebhookPayload_NilDefectReader verifies that a nil DefectReader
// (e.g. tests that don't wire regression detection) doesn't panic and simply
// never triggers regression_detected.
func TestBuildWebhookPayload_NilDefectReader(t *testing.T) {
	buildStore := &testutil.MockBuildStore{
		GetLatestBuildFn: func(_ context.Context, projectID int64) (store.Build, error) {
			return store.Build{ID: 1, ProjectID: projectID, BuildNumber: 1, StatTotal: statPtr(1), StatPassed: statPtr(1)}, nil
		},
	}
	payload, triggered, err := buildWebhookPayload(context.Background(), 1, buildStore, nil, "", zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if triggered[store.WebhookEventRegressionDetected] {
		t.Error("expected regression_detected not triggered with nil defect reader")
	}
	if payload.Regressions != nil {
		t.Errorf("expected nil Regressions with nil defect reader, got %+v", payload.Regressions)
	}
}

// TestBuildDigestDeliveries_RegressionsAndSubscribedWebhook verifies that a
// project with regressions in the window AND a webhook subscribed to
// "digest" produces exactly one SendWebhookArgs with the expected payload.
func TestBuildDigestDeliveries_RegressionsAndSubscribedWebhook(t *testing.T) {
	defectStore := testutil.NewMemDefectStore()
	defectStore.SeedRegressionsSince([]store.ProjectRegressions{
		{
			ProjectID: 1,
			Slug:      "my-project",
			Regressions: []store.DefectRegression{
				{ID: "fp-1", NormalizedMessage: "boom", Category: "product_bug", OccurrenceCount: 2},
			},
		},
	})

	webhookStore := testutil.NewMemWebhookStore()
	created, err := webhookStore.Create(context.Background(), &store.Webhook{
		ProjectID:  1,
		Name:       "digest-hook",
		TargetType: store.WebhookTargetSlack,
		URL:        "https://hooks.slack.com/services/x",
		Events:     []string{store.WebhookEventDigest},
		IsActive:   true,
	})
	if err != nil {
		t.Fatalf("failed to seed webhook: %v", err)
	}

	start := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)

	deliveries, err := buildDigestDeliveries(context.Background(), defectStore, webhookStore, start, end, zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	d := deliveries[0]
	if d.WebhookID != created.ID {
		t.Errorf("expected webhook ID %q, got %q", created.ID, d.WebhookID)
	}
	if d.Payload.Event != store.WebhookEventDigest {
		t.Errorf("expected event %q, got %q", store.WebhookEventDigest, d.Payload.Event)
	}
	if d.Payload.ProjectID != 1 || d.Payload.Slug != "my-project" {
		t.Errorf("unexpected payload project fields: %+v", d.Payload)
	}
	if d.Payload.Digest == nil {
		t.Fatal("expected Digest to be set")
	}
	if d.Payload.Digest.RegressionCount != 1 {
		t.Errorf("expected RegressionCount 1, got %d", d.Payload.Digest.RegressionCount)
	}
	if !d.Payload.Digest.PeriodStart.Equal(start) || !d.Payload.Digest.PeriodEnd.Equal(end) {
		t.Errorf("unexpected digest period: %+v", d.Payload.Digest)
	}
	if len(d.Payload.Digest.Regressions) != 1 || d.Payload.Digest.Regressions[0].Message != "boom" {
		t.Errorf("unexpected digest regressions: %+v", d.Payload.Digest.Regressions)
	}
}

// TestBuildDigestDeliveries_NoWebhooks verifies that a project with
// regressions but no webhook subscribed to "digest" produces no deliveries.
func TestBuildDigestDeliveries_NoWebhooks(t *testing.T) {
	defectStore := testutil.NewMemDefectStore()
	defectStore.SeedRegressionsSince([]store.ProjectRegressions{
		{ProjectID: 1, Slug: "my-project", Regressions: []store.DefectRegression{
			{ID: "fp-1", NormalizedMessage: "boom", Category: "product_bug", OccurrenceCount: 1},
		}},
	})
	webhookStore := testutil.NewMemWebhookStore() // no webhooks created

	deliveries, err := buildDigestDeliveries(context.Background(), defectStore, webhookStore, time.Now().Add(-24*time.Hour), time.Now(), zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deliveries) != 0 {
		t.Errorf("expected 0 deliveries with no subscribed webhooks, got %d", len(deliveries))
	}
}

// TestBuildDigestDeliveries_DedupesRegressionsByFingerprint verifies the fix
// for over-counting: ListRegressionsSince returns one row per
// (fingerprint, build), so a fingerprint that regressed in multiple builds
// within the window must collapse to a single Digest.Regressions entry and
// RegressionCount must reflect the deduped count, not the raw row count.
func TestBuildDigestDeliveries_DedupesRegressionsByFingerprint(t *testing.T) {
	defectStore := testutil.NewMemDefectStore()
	defectStore.SeedRegressionsSince([]store.ProjectRegressions{
		{
			ProjectID: 1,
			Slug:      "my-project",
			Regressions: []store.DefectRegression{
				{ID: "fp-1", NormalizedMessage: "boom", Category: "product_bug", OccurrenceCount: 2, BuildOrder: 5},
				{ID: "fp-1", NormalizedMessage: "boom", Category: "product_bug", OccurrenceCount: 4, BuildOrder: 6},
				{ID: "fp-1", NormalizedMessage: "boom", Category: "product_bug", OccurrenceCount: 3, BuildOrder: 7},
			},
		},
	})

	webhookStore := testutil.NewMemWebhookStore()
	if _, err := webhookStore.Create(context.Background(), &store.Webhook{
		ProjectID:  1,
		Name:       "digest-hook",
		TargetType: store.WebhookTargetSlack,
		URL:        "https://hooks.slack.com/services/x",
		Events:     []string{store.WebhookEventDigest},
		IsActive:   true,
	}); err != nil {
		t.Fatalf("failed to seed webhook: %v", err)
	}

	deliveries, err := buildDigestDeliveries(context.Background(), defectStore, webhookStore, time.Now().Add(-24*time.Hour), time.Now(), zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deliveries) != 1 {
		t.Fatalf("expected 1 delivery, got %d", len(deliveries))
	}
	digest := deliveries[0].Payload.Digest
	if digest == nil {
		t.Fatal("expected Digest to be set")
	}
	if len(digest.Regressions) != 1 {
		t.Fatalf("expected duplicate-fingerprint regressions to dedupe to 1 entry, got %d: %+v", len(digest.Regressions), digest.Regressions)
	}
	if digest.RegressionCount != 1 {
		t.Errorf("expected RegressionCount 1, got %d", digest.RegressionCount)
	}
	if digest.Regressions[0].OccurrenceCount != 4 {
		t.Errorf("expected deduped entry to keep the highest OccurrenceCount (4), got %d", digest.Regressions[0].OccurrenceCount)
	}
}

// TestBuildDigestDeliveries_NoRegressions verifies that a project with a
// subscribed webhook but no regressions in the window produces no deliveries.
func TestBuildDigestDeliveries_NoRegressions(t *testing.T) {
	defectStore := testutil.NewMemDefectStore()
	defectStore.SeedRegressionsSince([]store.ProjectRegressions{
		{ProjectID: 1, Slug: "my-project", Regressions: []store.DefectRegression{}},
	})

	webhookStore := testutil.NewMemWebhookStore()
	if _, err := webhookStore.Create(context.Background(), &store.Webhook{
		ProjectID:  1,
		Name:       "digest-hook",
		TargetType: store.WebhookTargetSlack,
		URL:        "https://hooks.slack.com/services/x",
		Events:     []string{store.WebhookEventDigest},
		IsActive:   true,
	}); err != nil {
		t.Fatalf("failed to seed webhook: %v", err)
	}

	deliveries, err := buildDigestDeliveries(context.Background(), defectStore, webhookStore, time.Now().Add(-24*time.Hour), time.Now(), zap.NewNop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(deliveries) != 0 {
		t.Errorf("expected 0 deliveries with no regressions, got %d", len(deliveries))
	}
}
