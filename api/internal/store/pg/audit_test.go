//go:build integration

package pg_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

func TestAuditLogger_RecordRoundtrip(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	a := pg.NewAuditStore(s)

	actorID := int64(42)
	meta, err := json.Marshal(map[string]any{"reason": "smoke"})
	if err != nil {
		t.Fatalf("marshal metadata: %v", err)
	}
	evt := store.AuditEvent{
		ActorID:    &actorID,
		ActorLabel: "alice@test.local",
		TargetType: store.AuditTargetUser,
		TargetID:   "42",
		Action:     store.AuditActionLoginSuccess,
		Outcome:    store.AuditOutcomeSuccess,
		IP:         "10.0.0.1",
		UserAgent:  "test-agent/1.0",
		RequestID:  "req-123",
		Metadata:   meta,
	}

	if err := a.Record(ctx, evt); err != nil {
		t.Fatalf("Record: %v", err)
	}

	rows, err := a.ListRecent(ctx, 10)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(rows) == 0 {
		t.Fatal("expected at least one row after Record")
	}

	// Locate the row we just inserted by request_id (unique per test run).
	var got *store.AuditEvent
	for i := range rows {
		if rows[i].RequestID == "req-123" {
			got = &rows[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("inserted row not found; got %d rows", len(rows))
	}

	if got.ID == 0 {
		t.Errorf("ID = 0, want non-zero IDENTITY value")
	}
	if got.OccurredAt.IsZero() {
		t.Errorf("OccurredAt is zero, want server-populated NOW()")
	}
	if got.ActorID == nil || *got.ActorID != actorID {
		t.Errorf("ActorID = %v, want %d", got.ActorID, actorID)
	}
	if got.ActorLabel != evt.ActorLabel {
		t.Errorf("ActorLabel = %q, want %q", got.ActorLabel, evt.ActorLabel)
	}
	if got.TargetType != evt.TargetType {
		t.Errorf("TargetType = %q, want %q", got.TargetType, evt.TargetType)
	}
	if got.TargetID != evt.TargetID {
		t.Errorf("TargetID = %q, want %q", got.TargetID, evt.TargetID)
	}
	if got.Action != evt.Action {
		t.Errorf("Action = %q, want %q", got.Action, evt.Action)
	}
	if got.Outcome != evt.Outcome {
		t.Errorf("Outcome = %q, want %q", got.Outcome, evt.Outcome)
	}
	if got.IP != evt.IP {
		t.Errorf("IP = %q, want %q", got.IP, evt.IP)
	}
	if got.UserAgent != evt.UserAgent {
		t.Errorf("UserAgent = %q, want %q", got.UserAgent, evt.UserAgent)
	}
	if got.RequestID != evt.RequestID {
		t.Errorf("RequestID = %q, want %q", got.RequestID, evt.RequestID)
	}
	if len(got.Metadata) == 0 {
		t.Errorf("Metadata empty, want JSON object")
	}
}

func TestAuditLogger_RecordFailureKeepsActorIDNil(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	a := pg.NewAuditStore(s)

	evt := store.AuditEvent{
		ActorID:    nil, // unauthenticated failed login
		ActorLabel: "ghost@test.local",
		TargetType: store.AuditTargetUser,
		TargetID:   "",
		Action:     store.AuditActionLoginFailure,
		Outcome:    store.AuditOutcomeFailure,
		IP:         "10.0.0.2",
		UserAgent:  "curl/8",
		RequestID:  "req-failure-456",
	}

	if err := a.Record(ctx, evt); err != nil {
		t.Fatalf("Record failure event: %v", err)
	}

	rows, err := a.ListRecent(ctx, 50)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	var got *store.AuditEvent
	for i := range rows {
		if rows[i].RequestID == "req-failure-456" {
			got = &rows[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("failure row not found in %d recent rows", len(rows))
	}
	if got.ActorID != nil {
		t.Errorf("ActorID = %v, want nil for unauthenticated failure", got.ActorID)
	}
	if got.Outcome != store.AuditOutcomeFailure {
		t.Errorf("Outcome = %q, want %q", got.Outcome, store.AuditOutcomeFailure)
	}
}

func TestAuditLogger_RecordRejectsMissingAction(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	a := pg.NewAuditStore(s)
	if err := a.Record(ctx, store.AuditEvent{Outcome: store.AuditOutcomeSuccess}); err == nil {
		t.Fatal("expected error for empty action, got nil")
	}
}

func TestAuditLogger_RecordRejectsMissingOutcome(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	a := pg.NewAuditStore(s)
	if err := a.Record(ctx, store.AuditEvent{Action: store.AuditActionLoginSuccess}); err == nil {
		t.Fatal("expected error for empty outcome, got nil")
	}
}

func TestAuditLogger_RecordRespectsExplicitOccurredAt(t *testing.T) {
	s := openLockTestStore(t)
	ctx := context.Background()

	a := pg.NewAuditStore(s)
	when := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	if err := a.Record(ctx, store.AuditEvent{
		OccurredAt: when,
		Action:     store.AuditActionLogout,
		Outcome:    store.AuditOutcomeSuccess,
		RequestID:  "req-explicit-time",
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}

	rows, err := a.ListRecent(ctx, 200)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	var got *store.AuditEvent
	for i := range rows {
		if rows[i].RequestID == "req-explicit-time" {
			got = &rows[i]
			break
		}
	}
	if got == nil {
		t.Fatal("row with explicit occurred_at not found")
	}
	if got.OccurredAt.Sub(when).Abs() > time.Second {
		t.Errorf("OccurredAt = %v, want close to %v", got.OccurredAt, when)
	}
}
