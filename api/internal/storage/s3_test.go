package storage

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"

	"go.uber.org/zap"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// mockS3Client is a test double for s3API.
type mockS3Client struct {
	PutObjectFn     func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObjectFn     func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObjectsFn func(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
	ListObjectsV2Fn func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	HeadObjectFn    func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
	CopyObjectFn    func(ctx context.Context, params *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error)
}

func (m *mockS3Client) PutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if m.PutObjectFn != nil {
		return m.PutObjectFn(ctx, params, optFns...)
	}
	return &s3.PutObjectOutput{}, nil
}

func (m *mockS3Client) GetObject(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if m.GetObjectFn != nil {
		return m.GetObjectFn(ctx, params, optFns...)
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(""))}, nil
}

func (m *mockS3Client) DeleteObjects(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	if m.DeleteObjectsFn != nil {
		return m.DeleteObjectsFn(ctx, params, optFns...)
	}
	return &s3.DeleteObjectsOutput{}, nil
}

func (m *mockS3Client) ListObjectsV2(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	if m.ListObjectsV2Fn != nil {
		return m.ListObjectsV2Fn(ctx, params, optFns...)
	}
	return &s3.ListObjectsV2Output{}, nil
}

func (m *mockS3Client) HeadObject(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if m.HeadObjectFn != nil {
		return m.HeadObjectFn(ctx, params, optFns...)
	}
	return &s3.HeadObjectOutput{}, nil
}

func (m *mockS3Client) CopyObject(ctx context.Context, params *s3.CopyObjectInput, optFns ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	if m.CopyObjectFn != nil {
		return m.CopyObjectFn(ctx, params, optFns...)
	}
	return &s3.CopyObjectOutput{}, nil
}

// Ensure mockS3Client satisfies s3API.
var _ s3API = (*mockS3Client)(nil)

// mockS3Uploader is a test double for s3Uploader.
type mockS3Uploader struct {
	UploadObjectFn func(ctx context.Context, input *transfermanager.UploadObjectInput, opts ...func(*transfermanager.Options)) (*transfermanager.UploadObjectOutput, error)
}

func (m *mockS3Uploader) UploadObject(ctx context.Context, input *transfermanager.UploadObjectInput, opts ...func(*transfermanager.Options)) (*transfermanager.UploadObjectOutput, error) {
	if m.UploadObjectFn != nil {
		return m.UploadObjectFn(ctx, input, opts...)
	}
	return &transfermanager.UploadObjectOutput{}, nil
}

// Ensure mockS3Uploader satisfies s3Uploader.
var _ s3Uploader = (*mockS3Uploader)(nil)

func testCfg() *config.Config {
	return &config.Config{
		S3: config.S3Config{Bucket: "test-bucket", Concurrency: 10},
	}
}

func TestS3Store_CreateProject_WritesKeepMarker(t *testing.T) {
	t.Parallel()
	var gotKey string
	mock := &mockS3Client{
		PutObjectFn: func(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			if params.Key != nil {
				gotKey = *params.Key
			}
			return &s3.PutObjectOutput{}, nil
		},
	}
	store := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())
	if err := store.CreateProject(context.Background(), "myproject"); err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	wantKey := "projects/myproject/.keep"
	if gotKey != wantKey {
		t.Errorf("CreateProject wrote key %q, want %q", gotKey, wantKey)
	}
}

func TestS3Store_CreateProject_PropagatesError(t *testing.T) {
	t.Parallel()
	mock := &mockS3Client{
		PutObjectFn: func(_ context.Context, _ *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			return nil, errors.New("s3 unavailable")
		},
	}
	store := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())
	err := store.CreateProject(context.Background(), "myproject")
	if err == nil {
		t.Fatal("expected error when PutObject fails, got nil")
	}
}

func TestS3Store_ResultsDirHash_Noop(t *testing.T) {
	t.Parallel()
	store := newS3StoreWithClient(testCfg(), &mockS3Client{}, &mockS3Uploader{}, zap.NewNop())
	hash, err := store.ResultsDirHash(context.Background(), "myproject")
	if err != nil {
		t.Fatalf("ResultsDirHash returned error: %v", err)
	}
	if hash != "" {
		t.Errorf("expected empty hash, got %q", hash)
	}
}

func TestS3Store_ListProjects(t *testing.T) {
	t.Parallel()
	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				CommonPrefixes: []s3types.CommonPrefix{
					{Prefix: aws.String("projects/alpha/")},
					{Prefix: aws.String("projects/beta/")},
				},
				IsTruncated: aws.Bool(false),
			}, nil
		},
	}
	store := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())
	projects, err := store.ListProjects(context.Background())
	if err != nil {
		t.Fatalf("ListProjects returned error: %v", err)
	}
	if len(projects) != 2 {
		t.Fatalf("expected 2 projects, got %d: %v", len(projects), projects)
	}
	if projects[0] != "alpha" || projects[1] != "beta" {
		t.Errorf("unexpected projects: %v", projects)
	}
}

func TestS3Store_WriteResultFile(t *testing.T) {
	t.Parallel()
	var capturedKey string
	uploader := &mockS3Uploader{
		UploadObjectFn: func(_ context.Context, params *transfermanager.UploadObjectInput, _ ...func(*transfermanager.Options)) (*transfermanager.UploadObjectOutput, error) {
			if params.Key != nil {
				capturedKey = *params.Key
			}
			return &transfermanager.UploadObjectOutput{}, nil
		},
	}
	store := newS3StoreWithClient(testCfg(), &mockS3Client{}, uploader, zap.NewNop())
	err := store.WriteResultFile(context.Background(), "myproject", "result.xml", strings.NewReader("data"))
	if err != nil {
		t.Fatalf("WriteResultFile returned error: %v", err)
	}
	expected := "projects/myproject/results/result.xml"
	if capturedKey != expected {
		t.Errorf("expected key %q, got %q", expected, capturedKey)
	}
}

func TestS3Store_WriteResultFile_UploaderError(t *testing.T) {
	t.Parallel()
	uploader := &mockS3Uploader{
		UploadObjectFn: func(_ context.Context, _ *transfermanager.UploadObjectInput, _ ...func(*transfermanager.Options)) (*transfermanager.UploadObjectOutput, error) {
			return nil, errors.New("upload failed")
		},
	}
	store := newS3StoreWithClient(testCfg(), &mockS3Client{}, uploader, zap.NewNop())
	err := store.WriteResultFile(context.Background(), "myproject", "result.xml", strings.NewReader("data"))
	if err == nil {
		t.Fatal("expected error when uploader fails, got nil")
	}
}

func TestS3Store_CleanResults(t *testing.T) {
	t.Parallel()
	var listedPrefix string
	var deletedKeys []string
	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			if params.Prefix != nil {
				listedPrefix = *params.Prefix
			}
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{
					{Key: aws.String("projects/myproject/results/a.xml")},
					{Key: aws.String("projects/myproject/results/b.json")},
				},
				IsTruncated: aws.Bool(false),
			}, nil
		},
		DeleteObjectsFn: func(_ context.Context, params *s3.DeleteObjectsInput, _ ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
			for _, obj := range params.Delete.Objects {
				if obj.Key != nil {
					deletedKeys = append(deletedKeys, *obj.Key)
				}
			}
			return &s3.DeleteObjectsOutput{}, nil
		},
	}
	store := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())
	if err := store.CleanResults(context.Background(), "myproject"); err != nil {
		t.Fatalf("CleanResults returned error: %v", err)
	}
	expectedPrefix := "projects/myproject/results/"
	if listedPrefix != expectedPrefix {
		t.Errorf("expected list prefix %q, got %q", expectedPrefix, listedPrefix)
	}
	if len(deletedKeys) != 2 {
		t.Errorf("expected 2 deleted keys, got %d: %v", len(deletedKeys), deletedKeys)
	}
}

func TestS3Store_DeleteReport_EmptyID(t *testing.T) {
	t.Parallel()
	store := newS3StoreWithClient(testCfg(), &mockS3Client{}, &mockS3Uploader{}, zap.NewNop())
	err := store.DeleteReport(context.Background(), "myproject", "")
	if !errors.Is(err, ErrReportIDEmpty) {
		t.Errorf("expected ErrReportIDEmpty, got %v", err)
	}
}

func TestS3Store_DeleteReport_InvalidID(t *testing.T) {
	t.Parallel()
	store := newS3StoreWithClient(testCfg(), &mockS3Client{}, &mockS3Uploader{}, zap.NewNop())
	err := store.DeleteReport(context.Background(), "myproject", "latest")
	if !errors.Is(err, ErrReportIDInvalid) {
		t.Errorf("expected ErrReportIDInvalid, got %v", err)
	}
}

func TestS3Store_LatestReportExists_True(t *testing.T) {
	t.Parallel()
	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents:    []s3types.Object{{Key: aws.String("projects/myproject/reports/latest/index.html")}},
				IsTruncated: aws.Bool(false),
			}, nil
		},
	}
	store := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())
	exists, err := store.LatestReportExists(context.Background(), "myproject")
	if err != nil {
		t.Fatalf("LatestReportExists returned error: %v", err)
	}
	if !exists {
		t.Error("expected LatestReportExists to return true")
	}
}

func TestS3Store_LatestReportExists_False(t *testing.T) {
	t.Parallel()
	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents:    []s3types.Object{},
				IsTruncated: aws.Bool(false),
			}, nil
		},
	}
	store := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())
	exists, err := store.LatestReportExists(context.Background(), "myproject")
	if err != nil {
		t.Fatalf("LatestReportExists returned error: %v", err)
	}
	if exists {
		t.Error("expected LatestReportExists to return false")
	}
}

func TestS3Store_ReadBuildStats_Summary(t *testing.T) {
	t.Parallel()
	summaryData, _ := json.Marshal(map[string]any{
		"statistic": map[string]int{
			"passed":  10,
			"failed":  2,
			"broken":  1,
			"skipped": 3,
			"unknown": 0,
			"total":   16,
		},
		"time": map[string]int64{
			"duration": 5000,
		},
	})
	mock := &mockS3Client{
		GetObjectFn: func(_ context.Context, params *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			if params.Key != nil && strings.HasSuffix(*params.Key, "summary.json") {
				return &s3.GetObjectOutput{
					Body: io.NopCloser(strings.NewReader(string(summaryData))),
				}, nil
			}
			return nil, errors.New("not found")
		},
	}
	store := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())
	stats, err := store.ReadBuildStats(context.Background(), "myproject", 5)
	if err != nil {
		t.Fatalf("ReadBuildStats returned error: %v", err)
	}
	if stats.Passed != 10 {
		t.Errorf("expected Passed=10, got %d", stats.Passed)
	}
	if stats.Failed != 2 {
		t.Errorf("expected Failed=2, got %d", stats.Failed)
	}
	if stats.Total != 16 {
		t.Errorf("expected Total=16, got %d", stats.Total)
	}
	if stats.DurationMs != 5000 {
		t.Errorf("expected DurationMs=5000, got %d", stats.DurationMs)
	}
}

func TestS3Store_ListReportBuilds(t *testing.T) {
	t.Parallel()
	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				CommonPrefixes: []s3types.CommonPrefix{
					{Prefix: aws.String("projects/myproject/reports/1/")},
					{Prefix: aws.String("projects/myproject/reports/2/")},
					{Prefix: aws.String("projects/myproject/reports/latest/")},
				},
				IsTruncated: aws.Bool(false),
			}, nil
		},
	}
	store := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())
	builds, err := store.ListReportBuilds(context.Background(), "myproject")
	if err != nil {
		t.Fatalf("ListReportBuilds returned error: %v", err)
	}
	if len(builds) != 2 {
		t.Fatalf("expected 2 numeric builds, got %d: %v", len(builds), builds)
	}
	if builds[0] != 1 || builds[1] != 2 {
		t.Errorf("unexpected builds: %v", builds)
	}
}

func TestS3Store_KeepHistory_UsesCopyObject(t *testing.T) {
	t.Parallel()
	historyFiles := []string{
		"projects/myproject/reports/latest/history/history.json",
		"projects/myproject/reports/latest/history/retry-trend.json",
	}

	type copyCall struct {
		bucket     string
		copySource string
		key        string
	}
	var copies []copyCall
	var getObjectCalled bool

	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			prefix := ""
			if params.Prefix != nil {
				prefix = *params.Prefix
			}
			// Return history files for the source listing
			if strings.HasPrefix(prefix, "projects/myproject/reports/latest/history/") {
				var contents []s3types.Object
				for _, k := range historyFiles {
					contents = append(contents, s3types.Object{Key: aws.String(k)})
				}
				return &s3.ListObjectsV2Output{
					Contents:    contents,
					IsTruncated: aws.Bool(false),
				}, nil
			}
			// deletePrefix listing for clearing destination
			return &s3.ListObjectsV2Output{IsTruncated: aws.Bool(false)}, nil
		},
		CopyObjectFn: func(_ context.Context, params *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
			copies = append(copies, copyCall{
				bucket:     aws.ToString(params.Bucket),
				copySource: aws.ToString(params.CopySource),
				key:        aws.ToString(params.Key),
			})
			return &s3.CopyObjectOutput{}, nil
		},
		GetObjectFn: func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			getObjectCalled = true
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("data"))}, nil
		},
	}

	cfg := testCfg()
	cfg.KeepHistory = true
	store := newS3StoreWithClient(cfg, mock, &mockS3Uploader{}, zap.NewNop())

	if err := store.KeepHistory(context.Background(), "myproject"); err != nil {
		t.Fatalf("KeepHistory returned error: %v", err)
	}

	// Must NOT download+upload — should use CopyObject instead
	if getObjectCalled {
		t.Error("KeepHistory should use CopyObject, not GetObject+PutObject")
	}

	// Verify correct number of copy calls
	if len(copies) != len(historyFiles) {
		t.Fatalf("expected %d CopyObject calls, got %d", len(historyFiles), len(copies))
	}

	// Verify copy parameters
	for i, c := range copies {
		if c.bucket != "test-bucket" {
			t.Errorf("copy[%d]: bucket = %q, want %q", i, c.bucket, "test-bucket")
		}
		wantSource := "test-bucket/" + historyFiles[i]
		if c.copySource != wantSource {
			t.Errorf("copy[%d]: copySource = %q, want %q", i, c.copySource, wantSource)
		}
	}

	// Verify destination keys
	wantDstKeys := []string{
		"projects/myproject/results/history/history.json",
		"projects/myproject/results/history/retry-trend.json",
	}
	for i, c := range copies {
		if c.key != wantDstKeys[i] {
			t.Errorf("copy[%d]: key = %q, want %q", i, c.key, wantDstKeys[i])
		}
	}
}

func TestS3Store_KeepHistory_CopyObjectError(t *testing.T) {
	t.Parallel()
	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			prefix := ""
			if params.Prefix != nil {
				prefix = *params.Prefix
			}
			if strings.HasPrefix(prefix, "projects/myproject/reports/latest/history/") {
				return &s3.ListObjectsV2Output{
					Contents:    []s3types.Object{{Key: aws.String("projects/myproject/reports/latest/history/h.json")}},
					IsTruncated: aws.Bool(false),
				}, nil
			}
			return &s3.ListObjectsV2Output{IsTruncated: aws.Bool(false)}, nil
		},
		CopyObjectFn: func(_ context.Context, _ *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
			return nil, errors.New("copy failed")
		},
	}

	cfg := testCfg()
	cfg.KeepHistory = true
	store := newS3StoreWithClient(cfg, mock, &mockS3Uploader{}, zap.NewNop())

	err := store.KeepHistory(context.Background(), "myproject")
	if err == nil {
		t.Fatal("expected error when CopyObject fails, got nil")
	}
	if !strings.Contains(err.Error(), "copy history object") {
		t.Errorf("error message should mention copy: %v", err)
	}
}

func TestS3Store_KeepHistory_NoHistory(t *testing.T) {
	t.Parallel()
	var copyObjectCalled bool
	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{IsTruncated: aws.Bool(false)}, nil
		},
		CopyObjectFn: func(_ context.Context, _ *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
			copyObjectCalled = true
			return &s3.CopyObjectOutput{}, nil
		},
	}

	cfg := testCfg()
	cfg.KeepHistory = true
	store := newS3StoreWithClient(cfg, mock, &mockS3Uploader{}, zap.NewNop())

	if err := store.KeepHistory(context.Background(), "myproject"); err != nil {
		t.Fatalf("KeepHistory returned error: %v", err)
	}
	if copyObjectCalled {
		t.Error("CopyObject should not be called when no history exists")
	}
}

func TestS3Store_KeepHistory_Disabled(t *testing.T) {
	t.Parallel()
	var deletedPrefix string
	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			if params.Prefix != nil {
				deletedPrefix = *params.Prefix
			}
			return &s3.ListObjectsV2Output{IsTruncated: aws.Bool(false)}, nil
		},
	}

	cfg := testCfg()
	cfg.KeepHistory = false
	store := newS3StoreWithClient(cfg, mock, &mockS3Uploader{}, zap.NewNop())

	if err := store.KeepHistory(context.Background(), "myproject"); err != nil {
		t.Fatalf("KeepHistory returned error: %v", err)
	}
	wantPrefix := "projects/myproject/results/history/"
	if deletedPrefix != wantPrefix {
		t.Errorf("expected delete prefix %q, got %q", wantPrefix, deletedPrefix)
	}
}

func TestPrepareLocal_ParallelDownloads(t *testing.T) {
	t.Parallel()
	resultsKey := "projects/myproject/results/result.json"
	historyKey := "projects/myproject/reports/latest/history/history.json"

	var mu sync.Mutex
	var listedPrefixes []string
	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			prefix := ""
			if params.Prefix != nil {
				prefix = *params.Prefix
			}
			mu.Lock()
			listedPrefixes = append(listedPrefixes, prefix)
			mu.Unlock()

			switch {
			case strings.HasSuffix(prefix, "/results/"):
				return &s3.ListObjectsV2Output{
					Contents:    []s3types.Object{{Key: aws.String(resultsKey)}},
					IsTruncated: aws.Bool(false),
				}, nil
			case strings.HasSuffix(prefix, "/history/"):
				return &s3.ListObjectsV2Output{
					Contents:    []s3types.Object{{Key: aws.String(historyKey)}},
					IsTruncated: aws.Bool(false),
				}, nil
			default:
				return &s3.ListObjectsV2Output{IsTruncated: aws.Bool(false)}, nil
			}
		},
		GetObjectFn: func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("data"))}, nil
		},
	}

	store := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())
	tmpDir, err := store.PrepareLocal(context.Background(), "myproject")
	if err != nil {
		t.Fatalf("PrepareLocal: %v", err)
	}
	t.Cleanup(func() { _ = store.CleanupLocal(tmpDir) })

	// Both results and history prefixes must have been listed.
	var sawResults, sawHistory bool
	for _, p := range listedPrefixes {
		if strings.HasSuffix(p, "/results/") {
			sawResults = true
		}
		if strings.HasSuffix(p, "/history/") {
			sawHistory = true
		}
	}
	if !sawResults {
		t.Error("PrepareLocal did not download results prefix")
	}
	if !sawHistory {
		t.Error("PrepareLocal did not download history prefix")
	}
}

func TestPrepareLocal_HistoryFailureNonFatal(t *testing.T) {
	t.Parallel()
	resultsKey := "projects/myproject/results/result.json"

	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			prefix := ""
			if params.Prefix != nil {
				prefix = *params.Prefix
			}
			if strings.HasSuffix(prefix, "/results/") {
				return &s3.ListObjectsV2Output{
					Contents:    []s3types.Object{{Key: aws.String(resultsKey)}},
					IsTruncated: aws.Bool(false),
				}, nil
			}
			// Simulate history listing failure.
			return nil, errors.New("history unavailable")
		},
		GetObjectFn: func(_ context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("result-data"))}, nil
		},
	}

	store := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())
	tmpDir, err := store.PrepareLocal(context.Background(), "myproject")
	if err != nil {
		t.Fatalf("PrepareLocal should succeed even when history fails, got: %v", err)
	}
	t.Cleanup(func() { _ = store.CleanupLocal(tmpDir) })
}

// --- Playwright storage methods ---

func TestS3Store_WritePlaywrightFile(t *testing.T) {
	t.Parallel()
	var capturedKey string
	mock := &mockS3Client{}
	uploader := &mockS3Uploader{
		UploadObjectFn: func(_ context.Context, input *transfermanager.UploadObjectInput, _ ...func(*transfermanager.Options)) (*transfermanager.UploadObjectOutput, error) {
			if input.Key != nil {
				capturedKey = *input.Key
			}
			return &transfermanager.UploadObjectOutput{}, nil
		},
	}
	st := newS3StoreWithClient(testCfg(), mock, uploader, zap.NewNop())

	err := st.WritePlaywrightFile(context.Background(), "proj1", "latest/index.html", strings.NewReader("content"))
	if err != nil {
		t.Fatalf("WritePlaywrightFile: %v", err)
	}
	wantKey := "projects/proj1/playwright-reports/latest/index.html"
	if capturedKey != wantKey {
		t.Errorf("key = %q, want %q", capturedKey, wantKey)
	}
}

func TestS3Store_PlaywrightReportExists_Found(t *testing.T) {
	t.Parallel()
	mock := &mockS3Client{
		HeadObjectFn: func(_ context.Context, params *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
			return &s3.HeadObjectOutput{}, nil
		},
	}
	st := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())

	exists, err := st.PlaywrightReportExists(context.Background(), "proj1", 3)
	if err != nil {
		t.Fatalf("PlaywrightReportExists: %v", err)
	}
	if !exists {
		t.Error("expected true when HeadObject succeeds")
	}
}

func TestS3Store_PlaywrightReportExists_NotFound(t *testing.T) {
	t.Parallel()
	mock := &mockS3Client{
		HeadObjectFn: func(_ context.Context, _ *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
			return nil, errors.New("not found")
		},
	}
	st := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())

	exists, err := st.PlaywrightReportExists(context.Background(), "proj1", 3)
	if err != nil {
		t.Fatalf("PlaywrightReportExists: %v", err)
	}
	if exists {
		t.Error("expected false when HeadObject errors")
	}
}

func TestS3Store_CopyPlaywrightLatestToBuild_Empty(t *testing.T) {
	t.Parallel()
	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{IsTruncated: aws.Bool(false)}, nil
		},
	}
	st := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())

	if err := st.CopyPlaywrightLatestToBuild(context.Background(), "proj1", 5); err != nil {
		t.Fatalf("CopyPlaywrightLatestToBuild empty: %v", err)
	}
}

func TestS3Store_CopyPlaywrightLatestToBuild_CopiesObjects(t *testing.T) {
	t.Parallel()
	var copiedSrc, copiedDst string
	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			key := "projects/proj1/playwright-reports/latest/index.html"
			return &s3.ListObjectsV2Output{
				Contents:    []s3types.Object{{Key: aws.String(key)}},
				IsTruncated: aws.Bool(false),
			}, nil
		},
		CopyObjectFn: func(_ context.Context, params *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
			if params.CopySource != nil {
				copiedSrc = *params.CopySource
			}
			if params.Key != nil {
				copiedDst = *params.Key
			}
			return &s3.CopyObjectOutput{}, nil
		},
	}
	st := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())

	if err := st.CopyPlaywrightLatestToBuild(context.Background(), "proj1", 7); err != nil {
		t.Fatalf("CopyPlaywrightLatestToBuild: %v", err)
	}
	wantDst := "projects/proj1/playwright-reports/7/index.html"
	if copiedDst != wantDst {
		t.Errorf("dst key = %q, want %q", copiedDst, wantDst)
	}
	if copiedSrc == "" {
		t.Error("expected non-empty copy source")
	}
}

func TestS3Store_CleanPlaywrightLatest(t *testing.T) {
	t.Parallel()
	var deletedPrefix string
	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			if params.Prefix != nil {
				deletedPrefix = *params.Prefix
			}
			return &s3.ListObjectsV2Output{IsTruncated: aws.Bool(false)}, nil
		},
	}
	st := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())

	if err := st.CleanPlaywrightLatest(context.Background(), "proj1"); err != nil {
		t.Fatalf("CleanPlaywrightLatest: %v", err)
	}
	wantPrefix := "projects/proj1/playwright-reports/latest/"
	if deletedPrefix != wantPrefix {
		t.Errorf("prefix = %q, want %q", deletedPrefix, wantPrefix)
	}
}

func TestS3Store_ListPlaywrightDataFiles(t *testing.T) {
	t.Parallel()
	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, params *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			prefix := ""
			if params.Prefix != nil {
				prefix = *params.Prefix
			}
			return &s3.ListObjectsV2Output{
				Contents: []s3types.Object{
					{Key: aws.String(prefix + "a.json")},
					{Key: aws.String(prefix + "b.json")},
				},
				IsTruncated: aws.Bool(false),
			}, nil
		},
	}
	st := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())

	files, err := st.ListPlaywrightDataFiles(context.Background(), "proj1", 2)
	if err != nil {
		t.Fatalf("ListPlaywrightDataFiles: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %v", files)
	}
}

func TestS3Store_ReadPlaywrightFile(t *testing.T) {
	t.Parallel()
	var capturedKey string
	mock := &mockS3Client{
		GetObjectFn: func(_ context.Context, params *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
			if params.Key != nil {
				capturedKey = *params.Key
			}
			return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader("pw-content"))}, nil
		},
	}
	st := newS3StoreWithClient(testCfg(), mock, &mockS3Uploader{}, zap.NewNop())

	rc, ct, err := st.ReadPlaywrightFile(context.Background(), "proj1", "1/index.html")
	if err != nil {
		t.Fatalf("ReadPlaywrightFile: %v", err)
	}
	defer func() { _ = rc.Close() }()

	data, _ := io.ReadAll(rc)
	if string(data) != "pw-content" {
		t.Errorf("content = %q, want %q", string(data), "pw-content")
	}
	if ct == "" {
		t.Error("expected non-empty content type")
	}
	wantKey := "projects/proj1/playwright-reports/1/index.html"
	if capturedKey != wantKey {
		t.Errorf("key = %q, want %q", capturedKey, wantKey)
	}
}
