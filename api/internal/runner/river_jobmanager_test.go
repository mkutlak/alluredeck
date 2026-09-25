package runner

import (
	"context"
	"reflect"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestBuildWebhookPayload_RegressionDetected verifies regression_detected fires
// only when DefectReader.ListRegressionsForBuild returns rows for the build, and
// that those rows are mapped onto payload.Regressions. Every row has a build
// with new failures versus a clean previous build: the old delta-based trigger
// (F3.4) fired on Delta.NewFailures alone, and must not come back.
func TestBuildWebhookPayload_RegressionDetected(t *testing.T) {
	buildStore := &testutil.MockBuildStore{
		GetLatestBuildFn: func(_ context.Context, projectID int64) (store.Build, error) {
			return store.Build{ID: 200, ProjectID: projectID, BuildNumber: 6, StatTotal: new(10), StatPassed: new(5), StatFailed: new(5)}, nil
		},
		GetPreviousBuildFn: func(_ context.Context, projectID int64, _ int) (store.Build, error) {
			return store.Build{ID: 100, ProjectID: projectID, BuildNumber: 5, StatTotal: new(10), StatPassed: new(10), StatFailed: new(0)}, nil
		},
	}
	tests := []struct {
		name      string
		regs      []store.DefectRegression
		nilReader bool // callers that don't wire regression detection
		want      []WebhookRegression
	}{
		{
			name: "regressions for the build fire and are mapped",
			regs: []store.DefectRegression{{ID: "fp-1", NormalizedMessage: "boom", Category: "product_bug", OccurrenceCount: 4, BuildOrder: 6}},
			want: []WebhookRegression{{FingerprintID: "fp-1", Message: "boom", Category: "product_bug", OccurrenceCount: 4}},
		},
		{name: "new failures in the delta alone do not fire"},
		{name: "nil defect reader never fires", nilReader: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var reader store.DefectReader
			if !tc.nilReader {
				defects := testutil.NewMemDefectStore()
				defects.SeedRegressionsForBuild(1, 200, tc.regs)
				reader = defects
			}
			payload, triggered, err := buildWebhookPayload(context.Background(), 1, buildStore, reader, "", zap.NewNop())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if payload.Delta == nil || payload.Delta.NewFailures <= 0 {
				t.Fatalf("test setup invalid: expected positive Delta.NewFailures, got %+v", payload.Delta)
			}
			if !triggered[store.WebhookEventReportFailed] {
				t.Error("report_failed must fire for a build with failures")
			}
			if got := triggered[store.WebhookEventRegressionDetected]; got != (tc.want != nil) {
				t.Errorf("regression_detected triggered = %v, want %v", got, tc.want != nil)
			}
			if !reflect.DeepEqual(payload.Regressions, tc.want) {
				t.Errorf("payload.Regressions = %+v, want %+v", payload.Regressions, tc.want)
			}
		})
	}
}

// TestBuildDigestDeliveries verifies the daily digest: one delivery per
// project with regressions in the window and a webhook subscribed to "digest".
// ListRegressionsSince returns one row per (fingerprint, build), so a
// fingerprint that regressed in several builds must collapse to one entry
// (keeping the highest occurrence count) instead of being over-counted.
func TestBuildDigestDeliveries(t *testing.T) {
	start := time.Date(2026, 7, 3, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 7, 4, 0, 0, 0, 0, time.UTC)
	reg := func(occurrences, buildOrder int) store.DefectRegression {
		return store.DefectRegression{ID: "fp-1", NormalizedMessage: "boom", Category: "product_bug", OccurrenceCount: occurrences, BuildOrder: buildOrder}
	}
	tests := []struct {
		name      string
		regs      []store.DefectRegression
		subscribe bool
		wantOcc   int // 0 = no delivery
	}{
		{name: "regressions and a subscribed webhook", regs: []store.DefectRegression{reg(2, 5)}, subscribe: true, wantOcc: 2},
		{name: "no subscribed webhook", regs: []store.DefectRegression{reg(1, 5)}},
		{name: "no regressions in the window", regs: []store.DefectRegression{}, subscribe: true},
		{name: "duplicate fingerprints dedupe", regs: []store.DefectRegression{reg(2, 5), reg(4, 6), reg(3, 7)}, subscribe: true, wantOcc: 4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			defects := testutil.NewMemDefectStore()
			defects.SeedRegressionsSince([]store.ProjectRegressions{{ProjectID: 1, Slug: "my-project", Regressions: tc.regs}})
			webhooks := testutil.NewMemWebhookStore()
			var hook *store.Webhook
			if tc.subscribe {
				var err error
				hook, err = webhooks.Create(context.Background(), &store.Webhook{
					ProjectID: 1, Name: "digest-hook", TargetType: store.WebhookTargetSlack, URL: "https://hooks.slack.com/services/x",
					Events: []string{store.WebhookEventDigest}, IsActive: true,
				})
				if err != nil {
					t.Fatalf("seed webhook: %v", err)
				}
			}

			deliveries, err := buildDigestDeliveries(context.Background(), defects, webhooks, start, end, zap.NewNop())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantOcc == 0 {
				if len(deliveries) != 0 {
					t.Errorf("expected no deliveries, got %d", len(deliveries))
				}
				return
			}
			if len(deliveries) != 1 {
				t.Fatalf("expected 1 delivery, got %d", len(deliveries))
			}
			d := deliveries[0]
			if d.WebhookID != hook.ID || d.Payload.Event != store.WebhookEventDigest || d.Payload.ProjectID != 1 || d.Payload.Slug != "my-project" {
				t.Errorf("unexpected delivery: webhook %q, payload %+v", d.WebhookID, d.Payload)
			}
			want := &WebhookDigest{PeriodStart: start, PeriodEnd: end, RegressionCount: 1, Regressions: []WebhookRegression{
				{FingerprintID: "fp-1", Message: "boom", Category: "product_bug", OccurrenceCount: tc.wantOcc},
			}}
			if !reflect.DeepEqual(d.Payload.Digest, want) {
				t.Errorf("digest = %+v, want %+v", d.Payload.Digest, want)
			}
		})
	}
}
