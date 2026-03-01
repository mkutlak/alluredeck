package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/runner"
	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

func newSearchHandler(t *testing.T) *AllureHandler {
	t.Helper()
	cfg := &config.Config{ProjectsDirectory: t.TempDir()}

	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	logger := zap.NewNop()
	buildStore := store.NewBuildStore(db, logger)
	lockManager := store.NewLockManager()
	st := storage.NewLocalStore(cfg)
	r := runner.NewAllure(cfg, st, buildStore, lockManager, nil, logger)
	searchStore := store.NewSearchStore(db, logger)

	return NewAllureHandler(cfg, r, nil, store.NewProjectStore(db, logger), buildStore, store.NewKnownIssueStore(db), nil, searchStore, st)
}

func makeSearchReq(t *testing.T, query string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "/api/v1/search"+query, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	return req
}

func TestSearch_MissingQuery(t *testing.T) {
	h := newSearchHandler(t)
	rr := httptest.NewRecorder()
	h.Search(rr, makeSearchReq(t, ""))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &body)
	meta, _ := body["metadata"].(map[string]any)
	msg, _ := meta["message"].(string)
	if msg == "" {
		t.Error("expected error message in metadata")
	}
}

func TestSearch_QueryTooShort(t *testing.T) {
	h := newSearchHandler(t)
	rr := httptest.NewRecorder()
	h.Search(rr, makeSearchReq(t, "?q=a"))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for too-short query, got %d", rr.Code)
	}
}

func TestSearch_QueryTooLong(t *testing.T) {
	h := newSearchHandler(t)
	rr := httptest.NewRecorder()
	longQ := ""
	for range 101 {
		longQ += "x"
	}
	h.Search(rr, makeSearchReq(t, "?q="+longQ))

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for too-long query, got %d", rr.Code)
	}
}

func TestSearch_ValidQuery_EmptyResults(t *testing.T) {
	h := newSearchHandler(t)
	rr := httptest.NewRecorder()
	h.Search(rr, makeSearchReq(t, "?q=nonexistent"))

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var body struct {
		Data struct {
			Projects []any `json:"projects"`
			Tests    []any `json:"tests"`
		} `json:"data"`
		Metadata struct {
			Message string `json:"message"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body.Data.Projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(body.Data.Projects))
	}
	if len(body.Data.Tests) != 0 {
		t.Errorf("expected 0 tests, got %d", len(body.Data.Tests))
	}
}

func TestSearch_LimitClampedToMax(t *testing.T) {
	h := newSearchHandler(t)
	rr := httptest.NewRecorder()
	// limit=999 should be clamped to 50 — handler should not error.
	h.Search(rr, makeSearchReq(t, "?q=test&limit=999"))

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestSearch_LimitDefault(t *testing.T) {
	h := newSearchHandler(t)
	rr := httptest.NewRecorder()
	// No limit param — should default to 10.
	h.Search(rr, makeSearchReq(t, "?q=test"))

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestSearch_InvalidLimit(t *testing.T) {
	h := newSearchHandler(t)
	rr := httptest.NewRecorder()
	// Non-numeric limit — should default to 10, not error.
	h.Search(rr, makeSearchReq(t, "?q=test&limit=abc"))

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 (invalid limit defaults), got %d", rr.Code)
	}
}
