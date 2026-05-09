package runner

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/mkutlak/alluredeck/api/internal/storage"
)

// Sentinel errors raised during tar.gz extraction. They mirror the legacy set
// previously defined in internal/handlers (so the sync HTTP path returns the
// same errors.Is(...) values to clients) but live here because both the
// handler and the staged worker reuse them.
var (
	ErrArchiveEmpty         = errors.New("archive contains no files")
	ErrArchiveTooManyFiles  = errors.New("archive exceeds maximum file count")
	ErrArchiveDecompBomb    = errors.New("decompressed archive exceeds size limit")
	ErrArchiveNestedPath    = errors.New("archive entry contains nested path")
	ErrArchiveInvalidEntry  = errors.New("archive entry is not a regular file")
	ErrArchiveDuplicateFile = errors.New("archive contains duplicate file names")
)

// DefaultMaxDecompressedBytes is the default decompression bomb cap (1 GiB).
const DefaultMaxDecompressedBytes int64 = 1 << 30

// DefaultMaxArchiveFileCount is the default per-archive entry-count cap.
const DefaultMaxArchiveFileCount = 5000

// TarExtractOptions tunes the shared tar.gz extraction helper.
type TarExtractOptions struct {
	// MaxDecompressedBytes caps the cumulative decompressed size to defend
	// against decompression bombs. Zero means use DefaultMaxDecompressedBytes.
	MaxDecompressedBytes int64
	// MaxFileCount caps the number of regular-file entries. Zero means use
	// DefaultMaxArchiveFileCount.
	MaxFileCount int
	// Concurrency caps the number of in-flight WriteResultFile calls during
	// the upload phase. Zero falls back to 32.
	Concurrency int
	// Reporter, when non-nil, is invoked with phase=JobPhaseExtractingStaged
	// and (done, total) progress as files are written to storage.
	Reporter JobProgressReporter
}

// countingTarReader wraps an io.Reader and tracks cumulative bytes read.
// When the limit is exceeded it returns an explicit error instead of silently
// truncating, mirroring the package-private countingReader previously living
// in internal/handlers.
type countingTarReader struct {
	r        io.Reader
	n        int64
	limit    int64
	exceeded bool
}

func (cr *countingTarReader) Read(p []byte) (int, error) {
	n, err := cr.r.Read(p)
	cr.n += int64(n)
	if cr.n > cr.limit {
		cr.exceeded = true
		return n, fmt.Errorf("decompressed size exceeds %d bytes: %w", cr.limit, ErrArchiveDecompBomb)
	}
	return n, err
}

// secureBaseName strips any path components from name and returns just the
// final component. Used to defend against path traversal entries.
func secureBaseName(name string) string {
	return filepath.Base(filepath.Clean(name))
}

// ExtractTarGzToStorage reads a tar.gz stream from src, validates each entry
// against the configured limits, and streams the bodies of regular files to
// store under projectID/results/batchID/. Returns the sorted list of written
// filenames on success.
//
// On any validation or write error nothing is rolled back — partial files may
// remain in storage. Callers are expected to treat any non-nil error as
// "this batch is incomplete" and skip downstream work that depends on a full
// extraction (the existing sync handler already does this).
//
// Validation matches the legacy sync path:
//   - decompression bomb (countingTarReader + MaxDecompressedBytes)
//   - nested-path rejection (entry name must equal its base name)
//   - duplicate-name detection
//   - file-count limit (MaxFileCount)
//   - empty-archive rejection (no regular-file entries written)
func ExtractTarGzToStorage(
	ctx context.Context,
	store storage.Store,
	projectID, batchID string,
	src io.Reader,
	opts TarExtractOptions,
) ([]string, error) {
	maxBytes := opts.MaxDecompressedBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxDecompressedBytes
	}
	maxFiles := opts.MaxFileCount
	if maxFiles <= 0 {
		maxFiles = DefaultMaxArchiveFileCount
	}
	concurrency := opts.Concurrency
	if concurrency <= 0 {
		concurrency = 32
	}

	gz, err := gzip.NewReader(src)
	if err != nil {
		return nil, fmt.Errorf("invalid gzip stream: %w", err)
	}
	defer func() { _ = gz.Close() }()

	cr := &countingTarReader{r: gz, limit: maxBytes}
	tr := tar.NewReader(cr)

	// Phase 1: walk the tar sequentially. Buffer each regular-file entry as a
	// pending write so the parallel write phase can fan out without sharing the
	// (non-thread-safe) tar.Reader.
	type pending struct {
		name string
		body []byte
	}
	var entries []pending
	seen := make(map[string]bool)
	fileCount := 0

	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if cr.exceeded {
				return nil, ErrArchiveDecompBomb
			}
			return nil, fmt.Errorf("read tar entry: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}

		safeName := secureBaseName(hdr.Name)
		if safeName == "." || safeName == "" {
			return nil, fmt.Errorf("entry %q: %w", hdr.Name, ErrArchiveInvalidEntry)
		}
		if filepath.Clean(hdr.Name) != safeName {
			return nil, fmt.Errorf("entry %q resolves to nested path: %w", hdr.Name, ErrArchiveNestedPath)
		}
		if seen[safeName] {
			return nil, fmt.Errorf("entry %q: %w", safeName, ErrArchiveDuplicateFile)
		}
		seen[safeName] = true

		fileCount++
		if fileCount > maxFiles {
			return nil, fmt.Errorf("archive has more than %d files: %w", maxFiles, ErrArchiveTooManyFiles)
		}

		buf, err := io.ReadAll(tr)
		if err != nil {
			if cr.exceeded {
				return nil, ErrArchiveDecompBomb
			}
			return nil, fmt.Errorf("extract %q: %w", safeName, err)
		}
		entries = append(entries, pending{name: safeName, body: buf})
	}

	if len(entries) == 0 {
		return nil, ErrArchiveEmpty
	}

	// Phase 2: bounded-parallel storage writes. First error cancels the rest.
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	total := len(entries)
	if opts.Reporter != nil {
		opts.Reporter(JobPhaseExtractingStaged, 0, total)
	}

	var (
		mu      sync.Mutex
		written = make([]string, 0, total)
		done    int
	)
	for _, e := range entries {
		g.Go(func() error {
			if err := store.WriteResultFile(gctx, projectID, batchID, e.name, byteReader(e.body)); err != nil {
				return fmt.Errorf("write result file %q: %w", e.name, err)
			}
			mu.Lock()
			written = append(written, e.name)
			done++
			d := done
			mu.Unlock()
			if opts.Reporter != nil {
				opts.Reporter(JobPhaseExtractingStaged, d, total)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	sort.Strings(written)
	return written, nil
}

// byteReader is a tiny io.Reader factory that avoids importing bytes for a
// single-use buffer wrap. It is only used by ExtractTarGzToStorage above.
func byteReader(b []byte) io.Reader {
	return &sliceReader{b: b}
}

type sliceReader struct {
	b   []byte
	pos int
}

func (s *sliceReader) Read(p []byte) (int, error) {
	if s.pos >= len(s.b) {
		return 0, io.EOF
	}
	n := copy(p, s.b[s.pos:])
	s.pos += n
	return n, nil
}
