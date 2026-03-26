package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/storage"
	"github.com/mkutlak/alluredeck/api/internal/store"
)

// ---------------------------------------------------------------------------
// mockAttachmentStore
// ---------------------------------------------------------------------------

type mockAttachmentStore struct {
	attachments []store.TestAttachment
	total       int
	errToReturn error
	source      *store.TestAttachment
}

func (m *mockAttachmentStore) ListByBuild(_ context.Context, _ string, _ int64, _ string, _, _ int) ([]store.TestAttachment, int, error) {
	return m.attachments, m.total, m.errToReturn
}

func (m *mockAttachmentStore) GetBySource(_ context.Context, _ int64, _ string) (*store.TestAttachment, error) {
	if m.errToReturn != nil {
		return nil, m.errToReturn
	}
	return m.source, nil
}

// ---------------------------------------------------------------------------
// mockBuildStore (minimal — only the two methods used by AttachmentHandler)
// ---------------------------------------------------------------------------

type mockAttachmentBuildStore struct {
	build       store.Build
	errToReturn error
}

func (m *mockAttachmentBuildStore) NextBuildOrder(_ context.Context, _ string) (int, error) {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) InsertBuild(_ context.Context, _ string, _ int) error {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) UpdateBuildStats(_ context.Context, _ string, _ int, _ store.BuildStats) error {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) UpdateBuildCIMetadata(_ context.Context, _ string, _ int, _ store.CIMetadata) error {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) GetBuildByOrder(_ context.Context, _ string, _ int) (store.Build, error) {
	return m.build, m.errToReturn
}
func (m *mockAttachmentBuildStore) GetPreviousBuild(_ context.Context, _ string, _ int) (store.Build, error) {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) GetLatestBuild(_ context.Context, _ string) (store.Build, error) {
	return m.build, m.errToReturn
}
func (m *mockAttachmentBuildStore) ListBuilds(_ context.Context, _ string) ([]store.Build, error) {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) ListBuildsPaginated(_ context.Context, _ string, _, _ int) ([]store.Build, int, error) {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) PruneBuilds(_ context.Context, _ string, _ int) ([]int, error) {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) SetLatest(_ context.Context, _ string, _ int) error {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) DeleteAllBuilds(_ context.Context, _ string) error {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) GetDashboardData(_ context.Context, _ int, _ string) ([]store.DashboardProject, error) {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) DeleteBuild(_ context.Context, _ string, _ int) error {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) UpdateBuildBranchID(_ context.Context, _ string, _ int, _ int64) error {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) SetLatestBranch(_ context.Context, _ string, _ int, _ *int64) error {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) PruneBuildsBranch(_ context.Context, _ string, _ int, _ *int64) ([]int, error) {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) PruneBuildsByAge(_ context.Context, _ string, _ time.Time) ([]int, error) {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) ListBuildsPaginatedBranch(_ context.Context, _ string, _, _ int, _ *int64) ([]store.Build, int, error) {
	panic("not implemented")
}
func (m *mockAttachmentBuildStore) ListBuildsInRange(_ context.Context, _ string, _ *int64, _, _ time.Time, _ int) ([]store.Build, int, error) {
	panic("not implemented")
}

// ---------------------------------------------------------------------------
// mockDataStore (minimal — only OpenReportFile used by AttachmentHandler)
// ---------------------------------------------------------------------------

type mockDataStore struct {
	content     string
	mimeType    string
	errToReturn error
}

func (m *mockDataStore) OpenReportFile(_ context.Context, _, _, _ string) (io.ReadCloser, string, error) {
	if m.errToReturn != nil {
		return nil, "", m.errToReturn
	}
	return io.NopCloser(strings.NewReader(m.content)), m.mimeType, nil
}

func (m *mockDataStore) CreateProject(_ context.Context, _ string) error { panic("not implemented") }
func (m *mockDataStore) DeleteProject(_ context.Context, _ string) error { panic("not implemented") }
func (m *mockDataStore) ProjectExists(_ context.Context, _ string) (bool, error) {
	panic("not implemented")
}
func (m *mockDataStore) ListProjects(_ context.Context) ([]string, error) { panic("not implemented") }
func (m *mockDataStore) WriteResultFile(_ context.Context, _, _ string, _ io.Reader) error {
	panic("not implemented")
}
func (m *mockDataStore) ListResultFiles(_ context.Context, _ string) ([]string, error) {
	panic("not implemented")
}
func (m *mockDataStore) CleanResults(_ context.Context, _ string) error { panic("not implemented") }
func (m *mockDataStore) PrepareLocal(_ context.Context, _ string) (string, error) {
	panic("not implemented")
}
func (m *mockDataStore) CleanupLocal(_ string) error { panic("not implemented") }
func (m *mockDataStore) PublishReport(_ context.Context, _ string, _ int, _ string) error {
	panic("not implemented")
}
func (m *mockDataStore) DeleteReport(_ context.Context, _, _ string) error { panic("not implemented") }
func (m *mockDataStore) PruneReportDirs(_ context.Context, _ string, _ []int) error {
	panic("not implemented")
}
func (m *mockDataStore) KeepHistory(_ context.Context, _ string) error  { panic("not implemented") }
func (m *mockDataStore) CleanHistory(_ context.Context, _ string) error { panic("not implemented") }
func (m *mockDataStore) ReadBuildStats(_ context.Context, _ string, _ int) (storage.BuildStats, error) {
	panic("not implemented")
}
func (m *mockDataStore) ReadFile(_ context.Context, _, _ string) ([]byte, error) {
	panic("not implemented")
}
func (m *mockDataStore) ReadDir(_ context.Context, _, _ string) ([]storage.DirEntry, error) {
	panic("not implemented")
}
func (m *mockDataStore) ListReportBuilds(_ context.Context, _ string) ([]int, error) {
	panic("not implemented")
}
func (m *mockDataStore) LatestReportExists(_ context.Context, _ string) (bool, error) {
	panic("not implemented")
}
func (m *mockDataStore) ResultsDirHash(_ context.Context, _ string) (string, error) {
	panic("not implemented")
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func newAttachmentHandler(as store.AttachmentStorer, bs store.BuildStorer, ds storage.Store) *AttachmentHandler {
	return NewAttachmentHandler(as, bs, ds, zap.NewNop())
}

// ---------------------------------------------------------------------------
// ListAttachments tests
// ---------------------------------------------------------------------------

func TestListAttachments_InvalidReportID(t *testing.T) {
	h := newAttachmentHandler(&mockAttachmentStore{}, &mockAttachmentBuildStore{}, &mockDataStore{})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/myproj/reports/abc!!/attachments", nil)
	req.SetPathValue("project_id", "myproj")
	req.SetPathValue("report_id", "abc!!")
	rr := httptest.NewRecorder()
	h.ListAttachments(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestListAttachments_BuildNotFound(t *testing.T) {
	bs := &mockAttachmentBuildStore{errToReturn: store.ErrBuildNotFound}
	h := newAttachmentHandler(&mockAttachmentStore{}, bs, &mockDataStore{})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/myproj/reports/5/attachments", nil)
	req.SetPathValue("project_id", "myproj")
	req.SetPathValue("report_id", "5")
	rr := httptest.NewRecorder()
	h.ListAttachments(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestListAttachments_Empty(t *testing.T) {
	bs := &mockAttachmentBuildStore{build: store.Build{ID: 1, BuildOrder: 5, ProjectID: "myproj"}}
	as := &mockAttachmentStore{attachments: []store.TestAttachment{}, total: 0}
	h := newAttachmentHandler(as, bs, &mockDataStore{})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/myproj/reports/5/attachments", nil)
	req.SetPathValue("project_id", "myproj")
	req.SetPathValue("report_id", "5")
	rr := httptest.NewRecorder()
	h.ListAttachments(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", resp["data"])
	}
	atts, ok := data["attachments"].([]any)
	if !ok {
		t.Fatalf("expected attachments array, got %T", data["attachments"])
	}
	if len(atts) != 0 {
		t.Errorf("expected 0 attachments, got %d", len(atts))
	}
	total, _ := data["total"].(float64)
	if total != 0 {
		t.Errorf("expected total=0, got %v", total)
	}
}

func TestListAttachments_WithResults(t *testing.T) {
	bs := &mockAttachmentBuildStore{build: store.Build{ID: 10, BuildOrder: 3, ProjectID: "proj1"}}
	as := &mockAttachmentStore{
		attachments: []store.TestAttachment{
			{ID: 1, Name: "screenshot.png", Source: "abc123-result.png", MimeType: "image/png", SizeBytes: 1024},
		},
		total: 1,
	}
	h := newAttachmentHandler(as, bs, &mockDataStore{})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj1/reports/3/attachments", nil)
	req.SetPathValue("project_id", "proj1")
	req.SetPathValue("report_id", "3")
	rr := httptest.NewRecorder()
	h.ListAttachments(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	data, ok := resp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", resp["data"])
	}
	atts, ok := data["attachments"].([]any)
	if !ok {
		t.Fatalf("expected attachments array, got %T", data["attachments"])
	}
	if len(atts) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(atts))
	}
	att, ok := atts[0].(map[string]any)
	if !ok {
		t.Fatalf("expected attachment object, got %T", atts[0])
	}
	// Verify url field is set correctly.
	wantURL := "/api/v1/projects/proj1/reports/3/attachments/abc123-result.png"
	if att["url"] != wantURL {
		t.Errorf("url = %v, want %v", att["url"], wantURL)
	}
	if att["name"] != "screenshot.png" {
		t.Errorf("name = %v, want screenshot.png", att["name"])
	}
	if att["mime_type"] != "image/png" {
		t.Errorf("mime_type = %v, want image/png", att["mime_type"])
	}
}

func TestListAttachments_MimeFilter(t *testing.T) {
	var capturedMime string
	bs := &mockAttachmentBuildStore{build: store.Build{ID: 1, BuildOrder: 1, ProjectID: "p"}}
	as := &mockAttachmentStore{}
	as.attachments = []store.TestAttachment{}
	// Override ListByBuild to capture the mimeFilter arg.
	captureStore := &captureMimeStore{inner: as, capturedMime: &capturedMime}
	h := newAttachmentHandler(captureStore, bs, &mockDataStore{})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/p/reports/1/attachments?mime_type=image", nil)
	req.SetPathValue("project_id", "p")
	req.SetPathValue("report_id", "1")
	rr := httptest.NewRecorder()
	h.ListAttachments(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if capturedMime != "image" {
		t.Errorf("mimeFilter passed to store = %q, want %q", capturedMime, "image")
	}
}

// captureMimeStore wraps mockAttachmentStore to capture the mimeFilter argument.
type captureMimeStore struct {
	inner        *mockAttachmentStore
	capturedMime *string
}

func (c *captureMimeStore) ListByBuild(_ context.Context, _ string, _ int64, mimeFilter string, _, _ int) ([]store.TestAttachment, int, error) {
	*c.capturedMime = mimeFilter
	return c.inner.attachments, c.inner.total, c.inner.errToReturn
}

func (c *captureMimeStore) GetBySource(_ context.Context, _ int64, _ string) (*store.TestAttachment, error) {
	return c.inner.source, c.inner.errToReturn
}

// ---------------------------------------------------------------------------
// ServeAttachment tests
// ---------------------------------------------------------------------------

func TestServeAttachment_PathTraversal(t *testing.T) {
	// These values should be rejected by the path traversal defense.
	// We set them directly via SetPathValue so the handler reads them from
	// r.PathValue("source") — bypassing URL parsing which would reject slashes.
	cases := []string{"dotdot-secret", "a-dotdot-b", "a-slash-b", "a-backslash-b", "a-null-b"}
	rawSources := []string{"../secret", "a/../b", "a/b", "a\\b", "a\x00b"}

	bs := &mockAttachmentBuildStore{build: store.Build{ID: 1, BuildOrder: 1, ProjectID: "p"}}
	h := newAttachmentHandler(&mockAttachmentStore{}, bs, &mockDataStore{})

	for i, src := range rawSources {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
			"/api/v1/projects/p/reports/1/attachments/"+cases[i], nil)
		req.SetPathValue("project_id", "p")
		req.SetPathValue("report_id", "1")
		req.SetPathValue("source", src)
		rr := httptest.NewRecorder()
		h.ServeAttachment(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Errorf("source=%q: want 400, got %d", src, rr.Code)
		}
	}
}

func TestServeAttachment_FileNotFound(t *testing.T) {
	bs := &mockAttachmentBuildStore{build: store.Build{ID: 1, BuildOrder: 1, ProjectID: "p"}}
	ds := &mockDataStore{errToReturn: errors.New("not found")}
	h := newAttachmentHandler(&mockAttachmentStore{}, bs, ds)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/p/reports/1/attachments/abc.png", nil)
	req.SetPathValue("project_id", "p")
	req.SetPathValue("report_id", "1")
	req.SetPathValue("source", "abc.png")
	rr := httptest.NewRecorder()
	h.ServeAttachment(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestServeAttachment_Success(t *testing.T) {
	bs := &mockAttachmentBuildStore{build: store.Build{ID: 1, BuildOrder: 2, ProjectID: "proj"}}
	ds := &mockDataStore{content: "PNG_DATA", mimeType: "image/png"}
	h := newAttachmentHandler(&mockAttachmentStore{}, bs, ds)
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet,
		"/api/v1/projects/proj/reports/2/attachments/screenshot.png", nil)
	req.SetPathValue("project_id", "proj")
	req.SetPathValue("report_id", "2")
	req.SetPathValue("source", "screenshot.png")
	rr := httptest.NewRecorder()
	h.ServeAttachment(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if ct := rr.Header().Get("Content-Type"); ct != "image/png" {
		t.Errorf("Content-Type = %q, want %q", ct, "image/png")
	}
	if rr.Body.String() != "PNG_DATA" {
		t.Errorf("body = %q, want %q", rr.Body.String(), "PNG_DATA")
	}
}
