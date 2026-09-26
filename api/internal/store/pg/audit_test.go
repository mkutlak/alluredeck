//go:build integration

package pg_test

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/store/pg"
)

// TestAuditStore_Record round-trips events through ListRecent: every column
// survives, the IDENTITY id is set, occurred_at defaults to NOW() unless the
// caller supplies one, and an unauthenticated failure keeps a NULL actor. An
// event without an action or an outcome is rejected.
func TestAuditStore_Record(t *testing.T) {
	a := pg.NewAuditStore(openTestStore(t))
	ctx := context.Background()
	actorID := int64(42)
	explicit := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
	tests := []struct {
		name    string
		evt     store.AuditEvent
		wantErr bool
	}{
		{name: "authenticated success", evt: store.AuditEvent{
			ActorID: &actorID, ActorLabel: "alice@test.local", TargetType: store.AuditTargetUser, TargetID: "42",
			Action: store.AuditActionLoginSuccess, Outcome: store.AuditOutcomeSuccess, IP: "10.0.0.1",
			UserAgent: "test-agent/1.0", Metadata: json.RawMessage(`{"reason": "smoke"}`),
		}},
		{name: "unauthenticated failure keeps a nil actor", evt: store.AuditEvent{
			ActorLabel: "ghost@test.local", TargetType: store.AuditTargetUser,
			Action: store.AuditActionLoginFailure, Outcome: store.AuditOutcomeFailure, IP: "10.0.0.2", UserAgent: "curl/8",
		}},
		{name: "explicit occurred_at is kept", evt: store.AuditEvent{
			OccurredAt: explicit, Action: store.AuditActionLogout, Outcome: store.AuditOutcomeSuccess,
		}},
		{name: "missing action", evt: store.AuditEvent{Outcome: store.AuditOutcomeSuccess}, wantErr: true},
		{name: "missing outcome", evt: store.AuditEvent{Action: store.AuditActionLoginSuccess}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.evt
			want.RequestID = unique("req")
			if err := a.Record(ctx, want); (err != nil) != tt.wantErr {
				t.Fatalf("Record err = %v, want error %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			rows, err := a.ListRecent(ctx, 200)
			if err != nil {
				t.Fatalf("ListRecent: %v", err)
			}
			i := slices.IndexFunc(rows, func(e store.AuditEvent) bool { return e.RequestID == want.RequestID })
			if i < 0 {
				t.Fatalf("recorded event not among %d recent rows", len(rows))
			}
			got := rows[i]
			if got.ID == 0 || got.OccurredAt.IsZero() {
				t.Errorf("ID %d, OccurredAt %v: want the IDENTITY id and a server timestamp", got.ID, got.OccurredAt)
			}
			if !want.OccurredAt.IsZero() && got.OccurredAt.Sub(want.OccurredAt).Abs() > time.Second {
				t.Errorf("OccurredAt = %v, want ~%v", got.OccurredAt, want.OccurredAt)
			}
			got.ID, got.OccurredAt = 0, want.OccurredAt
			if !reflect.DeepEqual(got, want) {
				t.Errorf("round-trip = %+v, want %+v", got, want)
			}
		})
	}
}
