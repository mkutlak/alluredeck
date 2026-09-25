package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// TestCreateProject_DuplicateSlugs pins slug uniqueness against a real
// LocalStore, so creates exercise actual filesystem collisions: top-level slugs
// are globally unique (idx_projects_slug_standalone) and child slugs unique per
// parent only (idx_projects_slug_per_parent). Two parents may each own a
// "child-x", so storage must be keyed by storage_key, not the slug — keying by
// slug makes the second child collide on disk.
func TestCreateProject_DuplicateSlugs(t *testing.T) {
	projectsDir := t.TempDir()
	cfg := &config.Config{ProjectsPath: projectsDir}
	mocks := testutil.New()
	st := storage.NewLocalStore(cfg)
	r := runner.NewAllure(runner.AllureDeps{Config: cfg, Store: st, BuildStore: mocks.MemBuilds, Locker: mocks.Locker, Logger: zap.NewNop()})
	h := NewProjectHandler(mocks.Projects, r, st, cfg, zap.NewNop())

	// post creates slug under parentID (0 for top level) and returns the new
	// project's id and storage key.
	post := func(want int, slug string, parentID float64) (float64, string) {
		t.Helper()
		payload := fmt.Sprintf(`{"id":%q}`, slug)
		if parentID != 0 {
			payload = fmt.Sprintf(`{"id":%q,"parent_id":%v}`, slug, parentID)
		}
		code, body := serveJSON(t, h.CreateProject, http.MethodPost, "/api/v1/projects", payload)
		if code != want {
			t.Fatalf("create %s: status = %d, want %d: %v", slug, code, want, body)
		}
		id, _ := jsonAt(body, "data.project_id").(float64)
		key, _ := jsonAt(body, "data.storage_key").(string)
		return id, key
	}

	parentA, _ := post(http.StatusCreated, "parent-a", 0)
	parentB, _ := post(http.StatusCreated, "parent-b", 0)
	childA, keyA := post(http.StatusCreated, "child-x", parentA)
	childB, keyB := post(http.StatusCreated, "child-x", parentB)
	if childA == childB || keyA == keyB {
		t.Fatalf("same-slug children share project_id or storage_key: %v/%q, %v/%q", childA, keyA, childB, keyB)
	}
	for _, key := range []string{keyA, keyB} {
		if _, err := os.Stat(filepath.Join(projectsDir, key)); err != nil {
			t.Errorf("storage dir for key %q: %v", key, err)
		}
	}
	post(http.StatusConflict, "child-x", parentA)
	post(http.StatusConflict, "parent-a", 0)
}
