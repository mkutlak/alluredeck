package runner

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/testutil"
)

// fakeStagedStore is a minimal storage.Store double for ParseStagedTarGzWorker
// and ExtractTarGzToStorage tests. It tracks WriteResultFile / DeleteBlob
// invocations and serves blob from OpenBlob (or fails with openErr).
type fakeStagedStore struct {
	testutil.MockStorage

	mu      sync.Mutex
	written map[string][]byte // {projectID/batchID/filename: bytes}
	deletes []string
	blob    []byte
	openErr error
}

func newFakeStagedStore(blob []byte) *fakeStagedStore {
	f := &fakeStagedStore{written: make(map[string][]byte), blob: blob}
	f.OpenBlobFn = func(_ context.Context, _ string) (io.ReadCloser, error) {
		if f.openErr != nil {
			return nil, f.openErr
		}
		return io.NopCloser(bytes.NewReader(f.blob)), nil
	}
	f.WriteResultFileFn = func(_ context.Context, projectID, batchID, filename string, r io.Reader) error {
		body, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		f.mu.Lock()
		f.written[projectID+"/"+batchID+"/"+filename] = body
		f.mu.Unlock()
		return nil
	}
	f.DeleteBlobFn = func(_ context.Context, key string) error {
		f.mu.Lock()
		f.deletes = append(f.deletes, key)
		f.mu.Unlock()
		return nil
	}
	return f
}

// makeTarGzBlob builds a tar.gz archive holding the named files.
func makeTarGzBlob(t *testing.T, names ...string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range names {
		body := []byte(`{"name":"` + name + `"}`)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
			t.Fatalf("tar header: %v", err)
		}
		if _, err := tw.Write(body); err != nil {
			t.Fatalf("tar body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar close: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gz close: %v", err)
	}
	return buf.Bytes()
}

// errAnyFailure marks rows that only need some non-nil error.
var errAnyFailure = errors.New("any failure")

// TestExtractTarGzToStorage covers the sync upload path's archive validation:
// flat regular files are written to results/<batch>/, while nested paths,
// empty archives, non-gzip streams and archives over MaxFileCount are rejected
// with the sentinel errors callers map to 4xx.
func TestExtractTarGzToStorage(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		blob    []byte
		opts    TarExtractOptions
		wantErr error
	}{
		{name: "writes every flat file", blob: makeTarGzBlob(t, "a.json", "b.json")},
		{name: "rejects a nested path", blob: makeTarGzBlob(t, "subdir/x.json"), wantErr: ErrArchiveNestedPath},
		{name: "rejects an empty archive", blob: makeTarGzBlob(t), wantErr: ErrArchiveEmpty},
		{name: "rejects a non-gzip stream", blob: []byte("not gzip"), wantErr: errAnyFailure},
		{name: "enforces MaxFileCount", blob: makeTarGzBlob(t, "a.json", "b.json", "c.json"), opts: TarExtractOptions{MaxFileCount: 2}, wantErr: ErrArchiveTooManyFiles},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			st := newFakeStagedStore(nil)
			written, err := ExtractTarGzToStorage(context.Background(), st, "proj", "batch1", bytes.NewReader(tc.blob), tc.opts)
			if matched := errors.Is(err, tc.wantErr) || (tc.wantErr == errAnyFailure && err != nil); !matched {
				t.Fatalf("error = %v, want %v", err, tc.wantErr)
			}
			if tc.wantErr != nil {
				return
			}
			slices.Sort(written)
			if !slices.Equal(written, []string{"a.json", "b.json"}) {
				t.Fatalf("written = %v, want [a.json b.json]", written)
			}
			for _, name := range written {
				if _, ok := st.written["proj/batch1/"+name]; !ok {
					t.Errorf("file %q not written to storage", name)
				}
			}
		})
	}
}

// captureWriter records progress upserts for assertion.
type captureWriter struct {
	mu     sync.Mutex
	phases []JobPhase
}

func (c *captureWriter) upsertJobProgress(_ context.Context, _ int64, phase JobPhase, _, _ int) {
	c.mu.Lock()
	c.phases = append(c.phases, phase)
	c.mu.Unlock()
}

// fakeReportGenerator is a minimal ReportGenerator stub.
type fakeReportGenerator struct{ called bool }

func (f *fakeReportGenerator) GenerateReport(_ context.Context, _ int64, _, _, _, _, _, _ string, _ bool, _, _, _, _ string) (string, error) {
	f.called = true
	return "42", nil
}

// TestParseStagedTarGzWorker drives the async upload worker. Extraction goes to
// a pod-local temp dir, so the Store must never see a WriteResultFile call (the
// guard for the removed storage round-trip). The staging blob is deleted only
// after success; a corrupt blob is left for operators to inspect, and a
// missing one fails before the generator runs.
func TestParseStagedTarGzWorker(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		blob        []byte
		openErr     error
		wantSuccess bool
	}{
		{name: "success generates and deletes the blob", blob: makeTarGzBlob(t, "r.json"), wantSuccess: true},
		{name: "corrupt blob is left in place", blob: []byte("not a gzip")},
		{name: "missing blob fails before generation", openErr: errors.New("not found")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			st := newFakeStagedStore(tc.blob)
			st.openErr = tc.openErr
			progress := &captureWriter{}
			gen := &fakeReportGenerator{}
			w := &ParseStagedTarGzWorker{store: st, generator: gen, progress: progress, logger: zap.NewNop()}
			job := &river.Job[ParseStagedTarGzArgs]{
				JobRow: &rivertype.JobRow{ID: 42, Attempt: 1, CreatedAt: time.Now()},
				Args:   ParseStagedTarGzArgs{ProjectID: 1, Slug: "p", StorageKey: "p", BatchID: "b1", StagingKey: "staging/b1.tar.gz", StoreResults: true},
			}

			err := w.Work(context.Background(), job)
			if (err == nil) != tc.wantSuccess {
				t.Fatalf("Work error = %v, want success=%v", err, tc.wantSuccess)
			}
			if gen.called != tc.wantSuccess {
				t.Errorf("ReportGenerator called = %v, want %v", gen.called, tc.wantSuccess)
			}
			if len(st.written) != 0 {
				t.Errorf("expected no WriteResultFile calls (extraction is local-only), got %v", st.written)
			}
			var wantDeletes []string
			wantLast := JobPhaseFailed
			if tc.wantSuccess {
				wantDeletes, wantLast = []string{"staging/b1.tar.gz"}, JobPhaseCompleted
			}
			if !slices.Equal(st.deletes, wantDeletes) {
				t.Errorf("DeleteBlob calls = %v, want %v", st.deletes, wantDeletes)
			}
			saw := progress.phases
			if len(saw) < 2 || saw[0] != JobPhaseExtractingStaged || saw[len(saw)-1] != wantLast {
				t.Errorf("phases = %v, want extracting_staged first and %s last", saw, wantLast)
			}
		})
	}
}
