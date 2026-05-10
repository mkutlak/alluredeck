package runner

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/storage"
)

// fakeRiverJob constructs a minimal river.Job around the given args so a
// worker's Work method can be invoked without a live River client.
func fakeRiverJob(args ParseStagedTarGzArgs) *river.Job[ParseStagedTarGzArgs] {
	return &river.Job[ParseStagedTarGzArgs]{
		JobRow: &rivertype.JobRow{ID: 42, Attempt: 1, CreatedAt: time.Now()},
		Args:   args,
	}
}

// fakeStagedStore is a minimal storage.Store double for ParseStagedTarGzWorker
// and ExtractTarGzToStorage tests. It tracks WriteResultFile / DeleteBlob /
// OpenBlob invocations so assertions can be made on the worker's behavior.
type fakeStagedStore struct {
	storage.MockStore

	mu        sync.Mutex
	written   map[string][]byte // {projectID/batchID/filename: bytes}
	deletes   []string
	blob      []byte
	openErr   error
	writeErr  error
	deleteErr error
}

func newFakeStagedStore(blob []byte) *fakeStagedStore {
	f := &fakeStagedStore{
		written: make(map[string][]byte),
		blob:    blob,
	}
	f.OpenBlobFn = func(_ context.Context, _ string) (io.ReadCloser, error) {
		if f.openErr != nil {
			return nil, f.openErr
		}
		return io.NopCloser(bytes.NewReader(f.blob)), nil
	}
	f.WriteResultFileFn = func(_ context.Context, projectID, batchID, filename string, r io.Reader) error {
		if f.writeErr != nil {
			return f.writeErr
		}
		body, err := io.ReadAll(r)
		if err != nil {
			return err
		}
		key := projectID + "/" + batchID + "/" + filename
		f.mu.Lock()
		f.written[key] = body
		f.mu.Unlock()
		return nil
	}
	f.DeleteBlobFn = func(_ context.Context, key string) error {
		if f.deleteErr != nil {
			return f.deleteErr
		}
		f.mu.Lock()
		f.deletes = append(f.deletes, key)
		f.mu.Unlock()
		return nil
	}
	return f
}

// makeTarGzBlob builds a small tar.gz archive containing the given files.
func makeTarGzBlob(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{
			Name:     name,
			Mode:     0o644,
			Size:     int64(len(body)),
			Typeflag: tar.TypeReg,
		}); err != nil {
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

// TestExtractTarGzToStorage_HappyPath verifies a small valid archive ends up
// fully written to storage in deterministic order.
func TestExtractTarGzToStorage_HappyPath(t *testing.T) {
	t.Parallel()
	files := map[string][]byte{
		"a.json": []byte(`{"a":1}`),
		"b.json": []byte(`{"b":2}`),
	}
	blob := makeTarGzBlob(t, files)

	store := newFakeStagedStore(nil)
	written, err := ExtractTarGzToStorage(context.Background(), store, "proj", "batch1", bytes.NewReader(blob), TarExtractOptions{})
	if err != nil {
		t.Fatalf("ExtractTarGzToStorage: %v", err)
	}
	if len(written) != 2 {
		t.Fatalf("expected 2 files, got %d", len(written))
	}
	for _, name := range written {
		if _, ok := store.written["proj/batch1/"+name]; !ok {
			t.Errorf("file %q not written to storage", name)
		}
	}
}

// TestExtractTarGzToStorage_RejectsNestedPath confirms the same rejection as
// the legacy sync handler.
func TestExtractTarGzToStorage_RejectsNestedPath(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	body := []byte("x")
	_ = tw.WriteHeader(&tar.Header{Name: "subdir/x.json", Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg})
	_, _ = tw.Write(body)
	_ = tw.Close()
	_ = gz.Close()

	store := newFakeStagedStore(nil)
	_, err := ExtractTarGzToStorage(context.Background(), store, "proj", "batch", bytes.NewReader(buf.Bytes()), TarExtractOptions{})
	if !errors.Is(err, ErrArchiveNestedPath) {
		t.Fatalf("expected ErrArchiveNestedPath, got %v", err)
	}
}

// TestExtractTarGzToStorage_EmptyArchive rejects archives with zero
// regular-file entries.
func TestExtractTarGzToStorage_EmptyArchive(t *testing.T) {
	t.Parallel()
	blob := makeTarGzBlob(t, map[string][]byte{})
	store := newFakeStagedStore(nil)
	_, err := ExtractTarGzToStorage(context.Background(), store, "proj", "batch", bytes.NewReader(blob), TarExtractOptions{})
	if !errors.Is(err, ErrArchiveEmpty) {
		t.Fatalf("expected ErrArchiveEmpty, got %v", err)
	}
}

// TestExtractTarGzToStorage_BadGzip surfaces a non-gzip stream as a generic
// error (callers map this to 4xx).
func TestExtractTarGzToStorage_BadGzip(t *testing.T) {
	t.Parallel()
	store := newFakeStagedStore(nil)
	_, err := ExtractTarGzToStorage(context.Background(), store, "proj", "batch", strings.NewReader("not gzip"), TarExtractOptions{})
	if err == nil {
		t.Fatal("expected error for invalid gzip")
	}
}

// TestExtractTarGzToStorage_FileCountLimit ensures MaxFileCount is enforced.
func TestExtractTarGzToStorage_FileCountLimit(t *testing.T) {
	t.Parallel()
	files := map[string][]byte{
		"a.json": []byte("a"),
		"b.json": []byte("b"),
		"c.json": []byte("c"),
	}
	blob := makeTarGzBlob(t, files)
	store := newFakeStagedStore(nil)
	_, err := ExtractTarGzToStorage(context.Background(), store, "proj", "batch", bytes.NewReader(blob), TarExtractOptions{MaxFileCount: 2})
	if !errors.Is(err, ErrArchiveTooManyFiles) {
		t.Fatalf("expected ErrArchiveTooManyFiles, got %v", err)
	}
}

// captureWriter records progress upserts for assertion.
type captureWriter struct {
	mu      sync.Mutex
	updates []phaseUpdate
}

type phaseUpdate struct {
	phase JobPhase
	done  int
	total int
}

func (c *captureWriter) upsertJobProgress(_ context.Context, _ int64, phase JobPhase, done, total int) {
	c.mu.Lock()
	c.updates = append(c.updates, phaseUpdate{phase, done, total})
	c.mu.Unlock()
}

func (c *captureWriter) phases() []JobPhase {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]JobPhase, len(c.updates))
	for i, u := range c.updates {
		out[i] = u.phase
	}
	return out
}

// fakeReportGenerator is a minimal ReportGenerator stub.
type fakeReportGenerator struct {
	called bool
	err    error
	output string
}

func (f *fakeReportGenerator) GenerateReport(_ context.Context, _ int64, _, _, _, _, _, _ string, _ bool, _, _, _, _ string) (string, error) {
	f.called = true
	return f.output, f.err
}

// TestParseStagedTarGzWorker_Success drives the worker end-to-end against a
// fake store and a fake generator. The new extraction path writes to a pod-
// local temp dir (not to the Store), so the assertion confirms the generator
// was invoked and the staging blob was deleted; the absence of any
// WriteResultFile call against the Store is the regression guard for the
// round-trip-removal change.
func TestParseStagedTarGzWorker_Success(t *testing.T) {
	t.Parallel()
	files := map[string][]byte{"r.json": []byte(`{"x":1}`)}
	blob := makeTarGzBlob(t, files)
	store := newFakeStagedStore(blob)

	progress := &captureWriter{}
	gen := &fakeReportGenerator{output: "42"}
	w := &ParseStagedTarGzWorker{
		store:     store,
		generator: gen,
		progress:  progress,
		reportIDs: &sync.Map{},
		logger:    zap.NewNop(),
	}

	args := ParseStagedTarGzArgs{
		ProjectID:    1,
		Slug:         "p",
		StorageKey:   "p",
		BatchID:      "b1",
		StagingKey:   "staging/b1.tar.gz",
		StoreResults: true,
	}
	if err := w.Work(context.Background(), fakeRiverJob(args)); err != nil {
		t.Fatalf("Work: %v", err)
	}

	if !gen.called {
		t.Error("expected ReportGenerator.GenerateReport to be called")
	}
	if len(store.written) != 0 {
		t.Errorf("expected no WriteResultFile calls (extraction is local-only now), got %v", store.written)
	}
	if len(store.deletes) != 1 || store.deletes[0] != "staging/b1.tar.gz" {
		t.Errorf("expected staging blob deletion, got %v", store.deletes)
	}

	// Phase progression must include extracting_staged followed by completed.
	saw := progress.phases()
	if len(saw) == 0 || saw[0] != JobPhaseExtractingStaged {
		t.Errorf("expected first phase = extracting_staged, got %v", saw)
	}
	if saw[len(saw)-1] != JobPhaseCompleted {
		t.Errorf("expected last phase = completed, got %v", saw)
	}
}

// TestParseStagedTarGzWorker_LeavesBlobOnExtractError verifies that a corrupt
// staged blob does NOT trigger a DeleteBlob call (so operators can inspect it).
func TestParseStagedTarGzWorker_LeavesBlobOnExtractError(t *testing.T) {
	t.Parallel()
	store := newFakeStagedStore([]byte("not a gzip"))
	progress := &captureWriter{}
	gen := &fakeReportGenerator{}
	w := &ParseStagedTarGzWorker{
		store:     store,
		generator: gen,
		progress:  progress,
		reportIDs: &sync.Map{},
		logger:    zap.NewNop(),
	}

	args := ParseStagedTarGzArgs{ProjectID: 1, Slug: "p", StorageKey: "p", BatchID: "b1", StagingKey: "staging/b1.tar.gz"}
	if err := w.Work(context.Background(), fakeRiverJob(args)); err == nil {
		t.Fatal("expected extraction failure")
	}
	if gen.called {
		t.Error("ReportGenerator should not be called when extraction fails")
	}
	if len(store.deletes) != 0 {
		t.Errorf("expected staging blob to remain, got deletes=%v", store.deletes)
	}
	saw := progress.phases()
	if len(saw) == 0 || saw[len(saw)-1] != JobPhaseFailed {
		t.Errorf("expected terminal phase = failed, got %v", saw)
	}
}

// TestParseStagedTarGzWorker_OpenBlobError treats a missing staging blob as a
// terminal failure and never calls the generator.
func TestParseStagedTarGzWorker_OpenBlobError(t *testing.T) {
	t.Parallel()
	store := newFakeStagedStore(nil)
	store.openErr = errors.New("not found")
	gen := &fakeReportGenerator{}
	w := &ParseStagedTarGzWorker{
		store:     store,
		generator: gen,
		progress:  &captureWriter{},
		reportIDs: &sync.Map{},
		logger:    zap.NewNop(),
	}
	args := ParseStagedTarGzArgs{ProjectID: 1, Slug: "p", StorageKey: "p", BatchID: "b1", StagingKey: "staging/b1.tar.gz"}
	if err := w.Work(context.Background(), fakeRiverJob(args)); err == nil {
		t.Fatal("expected error")
	}
	if gen.called {
		t.Error("ReportGenerator should not be called when blob is missing")
	}
}
