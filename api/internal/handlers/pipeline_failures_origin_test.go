package handlers

import (
	"context"
	"net/http"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/store"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestGetRunFailures_OriginAndEnvironment: every failure carries the store's
// origin and environment tags verbatim next to the fields it had before, and
// Allure's new_failed is still reported unchanged (the two answer different
// questions: new_failed compares with the suite's previous build, origin with
// its default branch).
func TestGetRunFailures_OriginAndEnvironment(t *testing.T) {
	ctx := context.Background()
	projects := testutil.NewMemProjectStore()
	parent, _ := projects.CreateProject(ctx, "parent")
	child, _ := projects.CreateProjectWithParent(ctx, "ui-users", parent.ID)
	rows := []store.RunFailureRow{
		{ProjectID: child.ID, Slug: "ui-users", BuildID: 1, BuildNumber: 7, TestName: "regressed", Status: "failed",
			Origin: store.FailureOriginNew},
		{ProjectID: child.ID, Slug: "ui-users", BuildID: 1, BuildNumber: 7, TestName: "still red", Status: "failed", NewFailed: true,
			Origin: store.FailureOriginInherited},
		{ProjectID: child.ID, Slug: "ui-users", BuildID: 1, BuildNumber: 7, TestName: "staging down", Status: "broken",
			Origin: store.FailureOriginUnknown, Environment: true},
		{ProjectID: child.ID, Slug: "ui-users", BuildID: 1, BuildNumber: 7, TestName: "env and new", Status: "failed",
			Origin: store.FailureOriginNew, Environment: true},
	}
	ps := &testutil.MockPipelineStore{
		ListRunFailuresFn: func(context.Context, int64, string, int) ([]store.RunFailureRow, error) { return rows, nil },
	}
	h := NewPipelineHandler(ps, projects, testutil.NewMemKnownIssueStore(), t.TempDir(), zap.NewNop())

	code, body := serveJSON(t, h.GetRunFailures, http.MethodGet, "/api/v1/pipeline-runs/p/failures", "",
		"project_id", "1", "run_key", "p")
	if code != http.StatusOK {
		t.Fatalf("status = %d: %v", code, body)
	}
	wantJSON(t, body, map[string]any{
		"data#":              4,
		"data.0.origin":      "new",
		"data.0.environment": false,
		"data.1.origin":      "inherited",
		"data.1.new_failed":  true,
		"data.1.environment": false,
		"data.2.origin":      "unknown",
		"data.2.environment": true,
		"data.3.origin":      "new",
		"data.3.environment": true,
	})
}
