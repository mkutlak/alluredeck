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
	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// ---------------------------------------------------------------------------
// mockAttachmentStore
// ---------------------------------------------------------------------------

type mockAttachmentStore struct {
	attachments []store.TestAttachment
	total       int
	errToReturn error
	source      *store.TestAttachment
	location    *store.AttachmentLocation
}

// Compile-time interface check.
var _ store.AttachmentStorer = (*mockAttachmentStore)(nil)

func (m *mockAttachmentStore) ListByBuild(_ context.Context, _ int64, _ int64, _, _ string, _, _ int) ([]store.TestAttachment, int, error) {
	return m.attachments, m.total, m.errToReturn
}

func (m *mockAttachmentStore) ListByTestResult(_ context.Context, _ int64, _ int64, _ string, _ int) ([]store.TestAttachment, error) {
	return m.attachments, m.errToReturn
}

func (m *mockAttachmentStore) GetBySource(_ context.Context, _ int64, _ string) (*store.TestAttachment, error) {
	if m.errToReturn != nil {
		return nil, m.errToReturn
	}
	return m.source, nil
}

func (m *mockAttachmentStore) InsertBuildAttachments(_ context.Context, _ int64, _ int64, _ []store.TestAttachment) error {
	return nil
}

func (m *mockAttachmentStore) GetByID(_ context.Context, _ int64) (*store.TestAttachment, error) {
	if m.errToReturn != nil {
		return nil, m.errToReturn
	}
	return m.source, nil
}

func (m *mockAttachmentStore) GetLocation(_ context.Context, _ int64) (*store.AttachmentLocation, error) {
	if m.errToReturn != nil {
		return nil, m.errToReturn
	}
	if m.location == nil {
		return nil, store.ErrAttachmentNotFound
	}
	return m.location, nil
}

// ---------------------------------------------------------------------------
// mockDataStore (minimal — only OpenReportFile used by AttachmentHandler)
// ---------------------------------------------------------------------------

type mockDataStore struct {
	content     string
	mimeType    string
	errToReturn error
	openCalls   int // number of times OpenReportFile was invoked
}

func (m *mockDataStore) OpenReportFile(_ context.Context, _, _, _ string) (io.ReadCloser, string, error) {
	m.openCalls++
	if m.errToReturn != nil {
		return nil, "", m.errToReturn
	}
	return io.NopCloser(strings.NewReader(m.content)), m.mimeType, nil
}

func (m *mockDataStore) HealthCheck(_ context.Context) error             { return nil }
func (m *mockDataStore) CreateProject(_ context.Context, _ string) error { panic("not implemented") }
func (m *mockDataStore) DeleteProject(_ context.Context, _ string) error { panic("not implemented") }
func (m *mockDataStore) RenameProject(_ context.Context, _, _ string) error {
	panic("not implemented")
}
func (m *mockDataStore) ProjectExists(_ context.Context, _ string) (bool, error) {
	panic("not implemented")
}
func (m *mockDataStore) ListProjects(_ context.Context) ([]string, error) { panic("not implemented") }
func (m *mockDataStore) WriteResultFile(_ context.Context, _, _, _ string, _ io.Reader) error {
	panic("not implemented")
}
func (m *mockDataStore) ListResultFiles(_ context.Context, _, _ string) ([]string, error) {
	panic("not implemented")
}
func (m *mockDataStore) CleanBatch(_ context.Context, _, _ string) error { panic("not implemented") }
func (m *mockDataStore) CleanResults(_ context.Context, _ string) error  { panic("not implemented") }
func (m *mockDataStore) ListResultBatches(_ context.Context, _ string) ([]string, error) {
	panic("not implemented")
}
func (m *mockDataStore) PrepareLocal(_ context.Context, _ string, _ storage.ProgressFn) (string, error) {
	panic("not implemented")
}
func (m *mockDataStore) CleanupLocal(_ string) error { panic("not implemented") }
func (m *mockDataStore) PublishReport(_ context.Context, _ string, _ int, _ string, _ storage.ProgressFn) error {
	panic("not implemented")
}
func (m *mockDataStore) DeleteReport(_ context.Context, _, _ string) error { panic("not implemented") }
func (m *mockDataStore) PruneReportDirs(_ context.Context, _ string, _ []int) error {
	panic("not implemented")
}
func (m *mockDataStore) KeepHistory(_ context.Context, _, _ string) error { panic("not implemented") }
func (m *mockDataStore) CleanHistory(_ context.Context, _ string) error   { panic("not implemented") }
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
func (m *mockDataStore) WritePlaywrightFile(_ context.Context, _, _ string, _ io.Reader) error {
	panic("not implemented")
}
func (m *mockDataStore) PlaywrightReportExists(_ context.Context, _ string, _ int) (bool, error) {
	panic("not implemented")
}
func (m *mockDataStore) CopyPlaywrightLatestToBuild(_ context.Context, _ string, _ int) error {
	panic("not implemented")
}
func (m *mockDataStore) CleanPlaywrightLatest(_ context.Context, _ string) error {
	panic("not implemented")
}
func (m *mockDataStore) ListPlaywrightDataFiles(_ context.Context, _ string, _ int) ([]string, error) {
	panic("not implemented")
}
func (m *mockDataStore) ReadPlaywrightFile(_ context.Context, _, _ string) (io.ReadCloser, string, error) {
	panic("not implemented")
}
func (m *mockDataStore) WriteRawBlob(_ context.Context, _ string, _ io.Reader) error {
	panic("not implemented")
}
func (m *mockDataStore) OpenBlob(_ context.Context, _ string) (io.ReadCloser, error) {
	panic("not implemented")
}
func (m *mockDataStore) DeleteBlob(_ context.Context, _ string) error {
	panic("not implemented")
}
func (m *mockDataStore) ListStagingBlobs(_ context.Context, _ time.Duration) ([]string, error) {
	panic("not implemented")
}

// newAttachmentHandler serves project 1 over the given stores; build lookups
// answer buildErr or a build with ID 10 for any number or "latest".
func newAttachmentHandler(t *testing.T, as store.AttachmentStorer, buildErr error, ds storage.Store) *AttachmentHandler {
	t.Helper()
	ps := testutil.NewMemProjectStore()
	if _, err := ps.CreateProject(context.Background(), "test-proj"); err != nil {
		t.Fatal(err)
	}
	bs := &testutil.MockBuildStore{GetBuildByNumberFn: func(_ context.Context, _ int64, n int) (store.Build, error) {
		return store.Build{ID: 10, BuildNumber: n, ProjectID: 1}, buildErr
	}}
	return NewAttachmentHandler(as, bs, ps, ds, zap.NewNop())
}

func TestAttachmentHandler_ListAttachments(t *testing.T) {
	t.Parallel()
	atts := []store.TestAttachment{
		{ID: 1, TestResultID: 100, Name: "screenshot.png", Source: "abc123-result.png", MimeType: "image/png", SizeBytes: 1024, TestName: "shouldRegister", TestStatus: "failed"},
		{ID: 2, TestResultID: 100, Name: "stdout.txt", Source: "def456.txt", MimeType: "text/plain", SizeBytes: 512, TestName: "shouldRegister", TestStatus: "failed"},
		{ID: 3, TestResultID: 200, Name: "stderr.txt", Source: "ghi789.txt", MimeType: "text/plain", SizeBytes: 256, TestName: "shouldLogin", TestStatus: "passed"},
	}
	type row struct {
		name, reportID, query string
		buildErr              error
		atts                  []store.TestAttachment
		want                  int
		wantMime, wantStatus  string // filters the store must receive
		wantGroups            string // "test/status:attachment,..." per group
	}
	rows := []row{
		{name: "invalid report id", reportID: "abc!!", want: http.StatusBadRequest},
		{name: "unknown build", reportID: "5", buildErr: store.ErrBuildNotFound, want: http.StatusNotFound},
		{name: "no attachments", reportID: "5", want: http.StatusOK},
		{name: "grouped by test result", reportID: "3", atts: atts, want: http.StatusOK,
			wantGroups: "shouldRegister/failed:screenshot.png,stdout.txt shouldLogin/passed:stderr.txt"},
		{name: "mime filter", reportID: "1", query: "mime_type=image", want: http.StatusOK, wantMime: "image"},
		{name: "invalid test_status", reportID: "1", query: "test_status=bogus", want: http.StatusBadRequest},
	}
	for _, s := range []string{"passed", "failed", "broken", "skipped", "unknown"} {
		rows = append(rows, row{name: "test_status " + s, reportID: "1", query: "test_status=" + s, want: http.StatusOK, wantStatus: s})
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var gotMime, gotStatus string
			as := &testutil.MockAttachmentStore{ListByBuildFn: func(_ context.Context, _, _ int64, mime, status string, _, _ int) ([]store.TestAttachment, int, error) {
				gotMime, gotStatus = mime, status
				return tc.atts, len(tc.atts), nil
			}}
			req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/1/reports/"+tc.reportID+"/attachments?"+tc.query, nil)
			req.SetPathValue("project_id", "1")
			req.SetPathValue("report_id", tc.reportID)
			rr := httptest.NewRecorder()
			newAttachmentHandler(t, as, tc.buildErr, &mockDataStore{}).ListAttachments(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.want != http.StatusOK {
				return
			}
			if gotMime != tc.wantMime || gotStatus != tc.wantStatus {
				t.Errorf("store filters = (%q, %q), want (%q, %q)", gotMime, gotStatus, tc.wantMime, tc.wantStatus)
			}
			var resp struct {
				Data struct {
					Total  int `json:"total"`
					Groups []struct {
						TestName    string `json:"test_name"`
						TestStatus  string `json:"test_status"`
						Attachments []struct {
							Name string `json:"name"`
						} `json:"attachments"`
					} `json:"groups"`
				} `json:"data"`
			}
			if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
				t.Fatal(err)
			}
			var groups []string
			for _, g := range resp.Data.Groups {
				var names []string
				for _, a := range g.Attachments {
					names = append(names, a.Name)
				}
				groups = append(groups, g.TestName+"/"+g.TestStatus+":"+strings.Join(names, ","))
			}
			if resp.Data.Groups == nil || strings.Join(groups, " ") != tc.wantGroups || resp.Data.Total != len(tc.atts) {
				t.Errorf("groups = %q, total = %d, want %q and %d", groups, resp.Data.Total, tc.wantGroups, len(tc.atts))
			}
		})
	}
}

func TestAttachmentHandler_ServeAttachment(t *testing.T) {
	t.Parallel()
	png := func() *mockDataStore { return &mockDataStore{content: "PNG_DATA", mimeType: "image/png"} }
	type row struct {
		name, source, query string
		ds                  *mockDataStore
		as                  *mockAttachmentStore
		want                int
		wantDisposition     string
	}
	rows := []row{
		// 68cabd8: attachments render inline unless ?dl=1 asks for a download,
		// which carries the attachment's human-readable name.
		{"inline", "screenshot.png", "", png(), &mockAttachmentStore{}, http.StatusOK, `inline; filename="screenshot.png"`},
		{"download", "abc123hash.png", "dl=1", png(), &mockAttachmentStore{source: &store.TestAttachment{Name: "my-screenshot.png", Source: "abc123hash.png"}},
			http.StatusOK, `attachment; filename="my-screenshot.png"`},
		{"file not found", "abc.png", "", &mockDataStore{errToReturn: errors.New("not found")}, &mockAttachmentStore{}, http.StatusNotFound, ""},
	}
	for _, src := range []string{"../secret", "a/../b", "a/b", "a\\b", "a\x00b"} {
		rows = append(rows, row{"path traversal " + src, src, "", png(), &mockAttachmentStore{}, http.StatusBadRequest, ""})
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// SetPathValue bypasses the URL parsing that would reject slashes.
			req := httptest.NewRequest(http.MethodGet, "/api/v1/projects/1/reports/2/attachments/x?"+tc.query, nil)
			req.SetPathValue("project_id", "1")
			req.SetPathValue("report_id", "2")
			req.SetPathValue("source", tc.source)
			rr := httptest.NewRecorder()
			newAttachmentHandler(t, tc.as, nil, tc.ds).ServeAttachment(rr, req)
			if rr.Code != tc.want {
				t.Fatalf("status = %d, want %d: %s", rr.Code, tc.want, rr.Body.String())
			}
			if tc.want != http.StatusOK {
				return
			}
			if ct, cd := rr.Header().Get("Content-Type"), rr.Header().Get("Content-Disposition"); ct != "image/png" || cd != tc.wantDisposition {
				t.Errorf("Content-Type = %q, Content-Disposition = %q, want image/png and %q", ct, cd, tc.wantDisposition)
			}
			if rr.Body.String() != "PNG_DATA" {
				t.Errorf("body = %q, want PNG_DATA", rr.Body.String())
			}
		})
	}
}
