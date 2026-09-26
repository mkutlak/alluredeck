package storage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// inflightHook returns a fakeS3 hook that tracks the peak number of
// concurrent calls; with cancel set, the first call cancels the context.
func inflightHook(peak *atomic.Int32, cancel context.CancelFunc) func(context.Context) error {
	var inflight atomic.Int32
	return func(ctx context.Context) error {
		cur := inflight.Add(1)
		defer inflight.Add(-1)
		for {
			if p := peak.Load(); cur <= p || peak.CompareAndSwap(p, cur) {
				break
			}
		}
		if cancel != nil {
			cancel()
			return ctx.Err()
		}
		time.Sleep(time.Millisecond) // let calls overlap so the limit is exercised
		return nil
	}
}

// TestDownloadPrefix mirrors every object under the prefix into the local dir
// with at most `concurrency` downloads in flight, warns once the total size
// passes the threshold, and fails when any download fails or the context is
// cancelled.
func TestDownloadPrefix(t *testing.T) {
	t.Parallel()
	twenty := map[string]string{"other/x.json": "outside the prefix"}
	mirrored := map[string]string{}
	for i := range 20 {
		twenty[fmt.Sprintf("p/file%02d.json", i)] = fmt.Sprintf("content-%02d", i)
		mirrored[fmt.Sprintf("file%02d.json", i)] = fmt.Sprintf("content-%02d", i)
	}
	big := map[string]string{"p/a.bin": strings.Repeat("x", 600), "p/b.bin": strings.Repeat("x", 600)}
	tests := []struct {
		name      string
		objects   map[string]string
		prefix    string
		warnAt    int64
		failOn    map[string]error
		cancel    bool
		wantFiles map[string]string // nil when the download must fail
		wantWarn  bool
	}{
		{"mirrors the prefix", twenty, "p/", 0, nil, false, mirrored, false},
		{"empty prefix downloads nothing", twenty, "empty/", 0, nil, false, map[string]string{}, false},
		{"total size over the threshold warns", big, "p/", 1000, nil, false, map[string]string{"a.bin": big["p/a.bin"], "b.bin": big["p/b.bin"]}, true},
		{"one failing object fails the download", map[string]string{"p/ok.json": "ok", "p/fail.json": "x"}, "p/", 0,
			map[string]error{"Get p/fail.json": errBoom}, false, nil, false},
		{"cancelled context fails the download", twenty, "p/", 0, nil, true, nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			const concurrency = 5
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := newFakeS3(tc.objects)
			f.failOn = tc.failOn
			var peak atomic.Int32
			if tc.cancel {
				f.getHook = inflightHook(&peak, cancel)
			} else {
				f.getHook = inflightHook(&peak, nil)
			}
			core, logs := observer.New(zapcore.WarnLevel)
			dir := t.TempDir()

			err := downloadPrefix(ctx, f, testBucket, tc.prefix, dir, concurrency, tc.warnAt, zap.New(core), nil)
			if (err != nil) != (tc.wantFiles == nil) {
				t.Fatalf("downloadPrefix error = %v, want error: %v", err, tc.wantFiles == nil)
			}
			if tc.wantFiles != nil {
				if got := readTree(t, dir); !reflect.DeepEqual(got, tc.wantFiles) {
					t.Errorf("local files = %v, want %v", got, tc.wantFiles)
				}
			}
			if p := peak.Load(); p > concurrency {
				t.Errorf("peak inflight %d exceeds concurrency limit %d", p, concurrency)
			}
			if got := logs.Len() > 0; got != tc.wantWarn {
				t.Errorf("size warning logged = %v, want %v", got, tc.wantWarn)
			}
		})
	}
}

// TestUploadDir uploads every file under the dir, keyed by its path below the
// prefix, with at most `concurrency` uploads in flight; an empty dir uploads
// nothing and a cancelled context fails the upload.
func TestUploadDir(t *testing.T) {
	t.Parallel()
	const concurrency = 4
	for _, tc := range []struct {
		name   string
		files  int
		cancel bool
	}{
		{"uploads every file", 15, false},
		{"empty dir uploads nothing", 0, false},
		{"cancelled context fails the upload", 5, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			var want []string
			for i := range tc.files {
				name := fmt.Sprintf("sub/file%02d.json", i)
				if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, filepath.FromSlash(name)), []byte(`{}`), 0o644); err != nil {
					t.Fatal(err)
				}
				want = append(want, "prefix/"+name)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			f := newFakeS3(nil)
			var peak atomic.Int32
			if tc.cancel {
				f.uploadHook = inflightHook(&peak, cancel)
			} else {
				f.uploadHook = inflightHook(&peak, nil)
			}

			err := uploadDir(ctx, f, testBucket, dir, "prefix/", concurrency, nil)
			if (err != nil) != tc.cancel {
				t.Fatalf("uploadDir error = %v, want error: %v", err, tc.cancel)
			}
			if got := f.keys(""); !tc.cancel && !slices.Equal(got, want) {
				t.Errorf("uploaded keys = %v, want %v", got, want)
			}
			if p := peak.Load(); p > concurrency {
				t.Errorf("peak inflight %d exceeds concurrency limit %d", p, concurrency)
			}
		})
	}
}
