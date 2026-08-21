package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// newCreateProjectHandler wires a ProjectHandler over a real LocalStore rooted
// at a temp dir, so CreateProject exercises actual filesystem behaviour rather
// than a mock that cannot collide.
func newCreateProjectHandler(t *testing.T) (*ProjectHandler, string) {
	t.Helper()
	projectsDir := t.TempDir()
	cfg := &config.Config{ProjectsPath: projectsDir}
	mocks := testutil.New()
	st := storage.NewLocalStore(cfg)
	logger := zap.NewNop()
	r := runner.NewAllure(runner.AllureDeps{
		Config:     cfg,
		Store:      st,
		BuildStore: mocks.MemBuilds,
		Locker:     mocks.Locker,
		Logger:     logger,
	})
	return NewProjectHandler(mocks.Projects, r, st, cfg, logger), projectsDir
}

// postProject issues POST /projects and returns the status and decoded entry.
func postProject(t *testing.T, h *ProjectHandler, slug string, parentID *int64) (int, ProjectEntry) {
	t.Helper()
	payload := fmt.Sprintf(`{"id":%q}`, slug)
	if parentID != nil {
		payload = fmt.Sprintf(`{"id":%q,"parent_id":%d}`, slug, *parentID)
	}
	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost,
		"/api/v1/projects", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.CreateProject(rr, req)

	var body struct {
		Data ProjectEntry `json:"data"`
	}
	if rr.Code == http.StatusCreated {
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode response %s: %v", rr.Body.String(), err)
		}
	}
	return rr.Code, body.Data
}

// TestCreateProject_DuplicateChildSlugAcrossParents pins the behaviour the
// schema already allows: idx_projects_slug_per_parent makes child slugs unique
// per parent, so two parents may each own a child called "child-x". Storage
// must therefore be keyed by storage_key (the numeric id for children), not by
// the slug — keying by slug makes the second create collide on disk.
func TestCreateProject_DuplicateChildSlugAcrossParents(t *testing.T) {
	h, projectsDir := newCreateProjectHandler(t)

	code, parentA := postProject(t, h, "parent-a", nil)
	if code != http.StatusCreated {
		t.Fatalf("create parent-a: want 201, got %d", code)
	}
	code, parentB := postProject(t, h, "parent-b", nil)
	if code != http.StatusCreated {
		t.Fatalf("create parent-b: want 201, got %d", code)
	}

	code, childA := postProject(t, h, "child-x", &parentA.ProjectID)
	if code != http.StatusCreated {
		t.Fatalf("create child-x under parent-a: want 201, got %d", code)
	}
	code, childB := postProject(t, h, "child-x", &parentB.ProjectID)
	if code != http.StatusCreated {
		t.Fatalf("create child-x under parent-b: want 201, got %d", code)
	}

	if childA.ProjectID == childB.ProjectID {
		t.Fatalf("same-slug children under different parents share project_id %d", childA.ProjectID)
	}
	if childA.StorageKey == childB.StorageKey {
		t.Fatalf("same-slug children share storage_key %q", childA.StorageKey)
	}

	// Each child must own a distinct directory, named by its storage key.
	for _, child := range []ProjectEntry{childA, childB} {
		dir := filepath.Join(projectsDir, child.StorageKey)
		if _, err := os.Stat(dir); err != nil {
			t.Errorf("storage dir for child %d (key %q): %v", child.ProjectID, child.StorageKey, err)
		}
	}
}

// TestCreateProject_DuplicateSlugSameParent verifies the conflict that IS real:
// reusing a slug under the same parent violates idx_projects_slug_per_parent.
func TestCreateProject_DuplicateSlugSameParent(t *testing.T) {
	h, _ := newCreateProjectHandler(t)

	code, parent := postProject(t, h, "parent-a", nil)
	if code != http.StatusCreated {
		t.Fatalf("create parent-a: want 201, got %d", code)
	}
	if code, _ = postProject(t, h, "child-x", &parent.ProjectID); code != http.StatusCreated {
		t.Fatalf("first child-x: want 201, got %d", code)
	}
	if code, _ = postProject(t, h, "child-x", &parent.ProjectID); code != http.StatusConflict {
		t.Fatalf("duplicate child-x under the same parent: want 409, got %d", code)
	}
}

// TestCreateProject_DuplicateTopLevelSlug verifies top-level slugs stay globally
// unique, per idx_projects_slug_standalone.
func TestCreateProject_DuplicateTopLevelSlug(t *testing.T) {
	h, _ := newCreateProjectHandler(t)

	if code, _ := postProject(t, h, "proj1", nil); code != http.StatusCreated {
		t.Fatalf("first proj1: want 201, got %d", code)
	}
	if code, _ := postProject(t, h, "proj1", nil); code != http.StatusConflict {
		t.Fatalf("duplicate top-level proj1: want 409, got %d", code)
	}
}
