package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// buildListResponse constructs a single-page ListObjectsV2Output from a slice of keys.
// The fake size is set to the key length so size-tracking tests have predictable totals.
func buildListResponse(keys []string) *s3.ListObjectsV2Output {
	objs := make([]s3types.Object, 0, len(keys))
	for _, k := range keys {
		size := int64(len(k))
		objs = append(objs, s3types.Object{Key: aws.String(k), Size: &size})
	}
	return &s3.ListObjectsV2Output{
		Contents:    objs,
		IsTruncated: aws.Bool(false),
	}
}

func TestDownloadPrefix_Parallel_MultipleObjects(t *testing.T) {
	t.Parallel()
	const n = 20
	const concurrency = 5
	prefix := "test-prefix/"

	keys := make([]string, n)
	for i := range n {
		keys[i] = fmt.Sprintf("%sfile%02d.json", prefix, i)
	}

	var inflight atomic.Int32
	var peak atomic.Int32

	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return buildListResponse(keys), nil
		},
		GetObjectFn: func(_ context.Context, params *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			cur := inflight.Add(1)
			for {
				p := peak.Load()
				if cur <= p || peak.CompareAndSwap(p, cur) {
					break
				}
			}
			defer inflight.Add(-1)
			key := aws.ToString(params.Key)
			return &s3.GetObjectOutput{
				Body: io.NopCloser(strings.NewReader("content-" + key)),
			}, nil
		},
	}

	tmpDir := t.TempDir()
	if err := downloadPrefix(context.Background(), mock, "bucket", prefix, tmpDir, concurrency, 0, zap.NewNop()); err != nil {
		t.Fatalf("downloadPrefix: %v", err)
	}

	for i := range n {
		path := filepath.Join(tmpDir, fmt.Sprintf("file%02d.json", i))
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing file %s: %v", path, err)
		}
	}
	if p := peak.Load(); p > int32(concurrency) {
		t.Errorf("peak inflight %d exceeds concurrency limit %d", p, concurrency)
	}
}

func TestDownloadPrefix_EmptyPrefix(t *testing.T) {
	t.Parallel()
	var getObjectCalled bool
	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{IsTruncated: aws.Bool(false)}, nil
		},
		GetObjectFn: func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			getObjectCalled = true
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(""))}, nil
		},
	}

	tmpDir := t.TempDir()
	if err := downloadPrefix(context.Background(), mock, "bucket", "empty/", tmpDir, 10, 0, zap.NewNop()); err != nil {
		t.Fatalf("downloadPrefix: %v", err)
	}
	if getObjectCalled {
		t.Error("GetObject should not be called for empty prefix")
	}
}

func TestDownloadPrefix_SingleObject(t *testing.T) {
	t.Parallel()
	prefix := "results/"
	key := prefix + "result.json"
	content := `{"status":"passed"}`

	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			size := int64(len(content))
			return &s3.ListObjectsV2Output{
				Contents:    []s3types.Object{{Key: aws.String(key), Size: &size}},
				IsTruncated: aws.Bool(false),
			}, nil
		},
		GetObjectFn: func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(content))}, nil
		},
	}

	tmpDir := t.TempDir()
	if err := downloadPrefix(context.Background(), mock, "bucket", prefix, tmpDir, 10, 0, zap.NewNop()); err != nil {
		t.Fatalf("downloadPrefix: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "result.json"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != content {
		t.Errorf("file content: want %q, got %q", content, string(data))
	}
}

func TestDownloadPrefix_ContextCancellation(t *testing.T) {
	t.Parallel()
	prefix := "results/"
	keys := []string{prefix + "a.json", prefix + "b.json", prefix + "c.json"}

	ctx, cancel := context.WithCancel(context.Background())
	var callCount atomic.Int32

	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return buildListResponse(keys), nil
		},
		GetObjectFn: func(ctx context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			if callCount.Add(1) == 1 {
				cancel()
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("data"))}, nil
		},
	}

	tmpDir := t.TempDir()
	err := downloadPrefix(ctx, mock, "bucket", prefix, tmpDir, 1, 0, zap.NewNop())
	if err == nil {
		t.Error("expected error after context cancellation, got nil")
	}
}

func TestDownloadPrefix_SizeWarning(t *testing.T) {
	t.Parallel()
	prefix := "results/"
	content600 := strings.Repeat("x", 600)
	keys := []string{prefix + "a.bin", prefix + "b.bin"}

	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			size := int64(600)
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{
					{Key: aws.String(keys[0]), Size: &size},
					{Key: aws.String(keys[1]), Size: &size},
				},
				IsTruncated: aws.Bool(false),
			}, nil
		},
		GetObjectFn: func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(content600))}, nil
		},
	}

	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	tmpDir := t.TempDir()
	const sizeThreshold int64 = 1000
	if err := downloadPrefix(context.Background(), mock, "bucket", prefix, tmpDir, 10, sizeThreshold, logger); err != nil {
		t.Fatalf("downloadPrefix: %v", err)
	}

	if logs.Len() == 0 {
		t.Error("expected at least one warning log for size threshold exceeded, got none")
	}
}

func TestDownloadPrefix_ErrorInOneObject(t *testing.T) {
	t.Parallel()
	prefix := "results/"
	keys := []string{prefix + "ok.json", prefix + "fail.json"}
	failKey := prefix + "fail.json"

	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return buildListResponse(keys), nil
		},
		GetObjectFn: func(_ context.Context, params *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			if aws.ToString(params.Key) == failKey {
				return nil, fmt.Errorf("s3 error: access denied")
			}
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("ok"))}, nil
		},
	}

	tmpDir := t.TempDir()
	err := downloadPrefix(context.Background(), mock, "bucket", prefix, tmpDir, 10, 0, zap.NewNop())
	if err == nil {
		t.Error("expected error when one object fails, got nil")
	}
}

func TestUploadDir_Parallel_MultipleFiles(t *testing.T) {
	t.Parallel()
	const n = 15
	const concurrency = 4

	tmpDir := t.TempDir()
	for i := range n {
		path := filepath.Join(tmpDir, fmt.Sprintf("file%02d.json", i))
		if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	var inflight atomic.Int32
	var peak atomic.Int32

	mock := &mockS3Uploader{
		UploadObjectFn: func(_ context.Context, _ *transfermanager.UploadObjectInput, _ ...func(*transfermanager.Options)) (*transfermanager.UploadObjectOutput, error) {
			cur := inflight.Add(1)
			for {
				p := peak.Load()
				if cur <= p || peak.CompareAndSwap(p, cur) {
					break
				}
			}
			defer inflight.Add(-1)
			return &transfermanager.UploadObjectOutput{}, nil
		},
	}

	if err := uploadDir(context.Background(), mock, "bucket", tmpDir, "prefix/", concurrency); err != nil {
		t.Fatalf("uploadDir: %v", err)
	}
	if p := peak.Load(); p > int32(concurrency) {
		t.Errorf("peak inflight %d exceeds concurrency limit %d", p, concurrency)
	}
}

func TestUploadDir_EmptyDir(t *testing.T) {
	t.Parallel()
	var uploadCalled bool
	mock := &mockS3Uploader{
		UploadObjectFn: func(_ context.Context, _ *transfermanager.UploadObjectInput, _ ...func(*transfermanager.Options)) (*transfermanager.UploadObjectOutput, error) {
			uploadCalled = true
			return &transfermanager.UploadObjectOutput{}, nil
		},
	}

	tmpDir := t.TempDir()
	if err := uploadDir(context.Background(), mock, "bucket", tmpDir, "prefix/", 10); err != nil {
		t.Fatalf("uploadDir: %v", err)
	}
	if uploadCalled {
		t.Error("Upload should not be called for empty directory")
	}
}

func TestUploadDir_ContextCancellation(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()
	for i := range 5 {
		path := filepath.Join(tmpDir, fmt.Sprintf("file%d.json", i))
		if err := os.WriteFile(path, []byte(`{}`), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	var callCount atomic.Int32

	mock := &mockS3Uploader{
		UploadObjectFn: func(ctx context.Context, _ *transfermanager.UploadObjectInput, _ ...func(*transfermanager.Options)) (*transfermanager.UploadObjectOutput, error) {
			if callCount.Add(1) == 1 {
				cancel()
			}
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return &transfermanager.UploadObjectOutput{}, nil
		},
	}

	err := uploadDir(ctx, mock, "bucket", tmpDir, "prefix/", 1)
	if err == nil {
		t.Error("expected error after context cancellation, got nil")
	}
}
