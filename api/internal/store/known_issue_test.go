package store

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
)

func openKnownIssueTestStore(t *testing.T) (*SQLiteStore, *KnownIssueStore, *ProjectStore) {
	t.Helper()
	db, err := Open(filepath.Join(t.TempDir(), "ki_test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db, NewKnownIssueStore(db), NewProjectStore(db, zap.NewNop())
}

func mustCreateProject(t *testing.T, ctx context.Context, ps *ProjectStore, id string) {
	t.Helper()
	if err := ps.CreateProject(ctx, id); err != nil {
		t.Fatalf("create project %s: %v", id, err)
	}
}

func TestKnownIssueStore_CreateAndGet(t *testing.T) {
	ctx := context.Background()
	_, kis, ps := openKnownIssueTestStore(t)
	mustCreateProject(t, ctx, ps, "proj1")

	issue, err := kis.Create(ctx, "proj1", "Login should succeed", "TC-1234", "http://ticket.example/1234", "Flaky login test")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if issue.ID == 0 {
		t.Fatal("expected non-zero ID")
	}
	if issue.TestName != "Login should succeed" {
		t.Errorf("test_name = %q, want %q", issue.TestName, "Login should succeed")
	}
	if issue.TicketURL != "http://ticket.example/1234" {
		t.Errorf("ticket_url = %q", issue.TicketURL)
	}
	if !issue.IsActive {
		t.Error("expected is_active = true")
	}

	got, err := kis.Get(ctx, issue.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.TestName != issue.TestName {
		t.Errorf("got test_name %q, want %q", got.TestName, issue.TestName)
	}
}

func TestKnownIssueStore_UniqueConstraint(t *testing.T) {
	ctx := context.Background()
	_, kis, ps := openKnownIssueTestStore(t)
	mustCreateProject(t, ctx, ps, "proj2")

	if _, err := kis.Create(ctx, "proj2", "Duplicate test", "", "", ""); err != nil {
		t.Fatalf("first create: %v", err)
	}
	if _, err := kis.Create(ctx, "proj2", "Duplicate test", "", "", ""); err == nil {
		t.Fatal("expected error on duplicate (project_id, test_name), got nil")
	}
}

func TestKnownIssueStore_ListActiveOnly(t *testing.T) {
	ctx := context.Background()
	_, kis, ps := openKnownIssueTestStore(t)
	mustCreateProject(t, ctx, ps, "proj3")

	issue1, err := kis.Create(ctx, "proj3", "Active test", "", "", "")
	if err != nil {
		t.Fatalf("create active: %v", err)
	}
	issue2, err := kis.Create(ctx, "proj3", "Inactive test", "", "", "")
	if err != nil {
		t.Fatalf("create inactive: %v", err)
	}

	// Deactivate issue2
	if err := kis.Update(ctx, issue2.ID, "proj3", "", "", false); err != nil {
		t.Fatalf("update: %v", err)
	}

	active, err := kis.List(ctx, "proj3", true)
	if err != nil {
		t.Fatalf("list active: %v", err)
	}
	if len(active) != 1 {
		t.Fatalf("expected 1 active, got %d", len(active))
	}
	if active[0].ID != issue1.ID {
		t.Errorf("wrong active issue: got %d, want %d", active[0].ID, issue1.ID)
	}
}

func TestKnownIssueStore_ListAll(t *testing.T) {
	ctx := context.Background()
	_, kis, ps := openKnownIssueTestStore(t)
	mustCreateProject(t, ctx, ps, "proj4")

	for _, name := range []string{"T1", "T2", "T3"} {
		if _, err := kis.Create(ctx, "proj4", name, "", "", ""); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}
	// Deactivate T1
	all, _ := kis.List(ctx, "proj4", false)
	for _, i := range all {
		if i.TestName == "T1" {
			_ = kis.Update(ctx, i.ID, "proj4", "", "", false)
		}
	}

	all2, err := kis.List(ctx, "proj4", false)
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all2) != 3 {
		t.Fatalf("expected 3 total, got %d", len(all2))
	}
}

func TestKnownIssueStore_Update(t *testing.T) {
	ctx := context.Background()
	_, kis, ps := openKnownIssueTestStore(t)
	mustCreateProject(t, ctx, ps, "proj5")

	issue, err := kis.Create(ctx, "proj5", "Update me", "", "", "old desc")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := kis.Update(ctx, issue.ID, "proj5", "http://new-ticket", "new desc", true); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := kis.Get(ctx, issue.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.TicketURL != "http://new-ticket" {
		t.Errorf("ticket_url = %q, want %q", got.TicketURL, "http://new-ticket")
	}
	if got.Description != "new desc" {
		t.Errorf("description = %q, want %q", got.Description, "new desc")
	}
}

func TestKnownIssueStore_Delete(t *testing.T) {
	ctx := context.Background()
	_, kis, ps := openKnownIssueTestStore(t)
	mustCreateProject(t, ctx, ps, "proj6")

	issue, err := kis.Create(ctx, "proj6", "Delete me", "", "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := kis.Delete(ctx, issue.ID, "proj6"); err != nil {
		t.Fatalf("delete: %v", err)
	}

	if _, err := kis.Get(ctx, issue.ID); err == nil {
		t.Fatal("expected error after delete, got nil")
	}
}

func TestKnownIssueStore_CascadeOnProjectDelete(t *testing.T) {
	ctx := context.Background()
	_, kis, ps := openKnownIssueTestStore(t)
	mustCreateProject(t, ctx, ps, "proj7")

	if _, err := kis.Create(ctx, "proj7", "Cascade test", "", "", ""); err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := ps.DeleteProject(ctx, "proj7"); err != nil {
		t.Fatalf("delete project: %v", err)
	}

	list, err := kis.List(ctx, "proj7", false)
	if err != nil {
		t.Fatalf("list after cascade: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 after cascade, got %d", len(list))
	}
}

func TestKnownIssueStore_Pagination(t *testing.T) {
	ctx := context.Background()
	_, kis, ps := openKnownIssueTestStore(t)
	mustCreateProject(t, ctx, ps, "proj8")

	for i := range 5 {
		name := "Test " + string(rune('A'+i))
		if _, err := kis.Create(ctx, "proj8", name, "", "", ""); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
	}

	page1, total, err := kis.ListPaginated(ctx, "proj8", false, 1, 3)
	if err != nil {
		t.Fatalf("list paginated: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(page1) != 3 {
		t.Errorf("page1 len = %d, want 3", len(page1))
	}

	page2, total2, err := kis.ListPaginated(ctx, "proj8", false, 2, 3)
	if err != nil {
		t.Fatalf("list page 2: %v", err)
	}
	if total2 != 5 {
		t.Errorf("total2 = %d, want 5", total2)
	}
	if len(page2) != 2 {
		t.Errorf("page2 len = %d, want 2", len(page2))
	}
}

func TestKnownIssueStore_UpdateRejectsWrongProject(t *testing.T) {
	ctx := context.Background()
	_, kis, ps := openKnownIssueTestStore(t)
	mustCreateProject(t, ctx, ps, "projA")
	mustCreateProject(t, ctx, ps, "projB")

	issue, err := kis.Create(ctx, "projA", "Test in projA", "", "http://ticket/1", "desc")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Attempt to update via projB should fail.
	err = kis.Update(ctx, issue.ID, "projB", "http://evil", "hacked", true)
	if err == nil {
		t.Fatal("expected error when updating issue from wrong project, got nil")
	}
	if !errors.Is(err, ErrKnownIssueNotFound) {
		t.Errorf("expected ErrKnownIssueNotFound, got %v", err)
	}

	// Verify issue is unchanged.
	got, err := kis.Get(ctx, issue.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.TicketURL != "http://ticket/1" {
		t.Errorf("ticket_url should be unchanged, got %q", got.TicketURL)
	}
}

func TestKnownIssueStore_DeleteRejectsWrongProject(t *testing.T) {
	ctx := context.Background()
	_, kis, ps := openKnownIssueTestStore(t)
	mustCreateProject(t, ctx, ps, "projA")
	mustCreateProject(t, ctx, ps, "projB")

	issue, err := kis.Create(ctx, "projA", "Test in projA", "", "", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	// Attempt to delete via projB should fail.
	err = kis.Delete(ctx, issue.ID, "projB")
	if err == nil {
		t.Fatal("expected error when deleting issue from wrong project, got nil")
	}
	if !errors.Is(err, ErrKnownIssueNotFound) {
		t.Errorf("expected ErrKnownIssueNotFound, got %v", err)
	}

	// Verify issue still exists.
	if _, err := kis.Get(ctx, issue.ID); err != nil {
		t.Fatalf("issue should still exist: %v", err)
	}
}

func TestKnownIssueStore_IsKnown(t *testing.T) {
	ctx := context.Background()
	_, kis, ps := openKnownIssueTestStore(t)
	mustCreateProject(t, ctx, ps, "proj9")

	if _, err := kis.Create(ctx, "proj9", "Known test", "", "", ""); err != nil {
		t.Fatalf("create: %v", err)
	}

	known, err := kis.IsKnown(ctx, "proj9", "Known test")
	if err != nil {
		t.Fatalf("is known: %v", err)
	}
	if !known {
		t.Error("expected known=true for existing test")
	}

	unknown, err := kis.IsKnown(ctx, "proj9", "Not a known test")
	if err != nil {
		t.Fatalf("is known 2: %v", err)
	}
	if unknown {
		t.Error("expected known=false for unknown test")
	}
}
