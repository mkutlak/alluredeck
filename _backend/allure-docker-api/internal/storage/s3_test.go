package storage

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"

	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/config"
)

// mockS3Client is a test double for s3API.
type mockS3Client struct {
	PutObjectFn     func(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.Options)) (*s3.PutObjectOutput, error)
	GetObjectFn     func(ctx context.Context, params *s3.GetObjectInput, optFns ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	DeleteObjectsFn func(ctx context.Context, params *s3.DeleteObjectsInput, optFns ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error)
	ListObjectsV2Fn func(ctx context.Context, params *s3.ListObjectsV2Input, optFns ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	HeadObjectFn    func(ctx context.Context, params *s3.HeadObjectInput, optFns ...func(*s3.Options)) (*s3.HeadObjectOutput, error)
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

// Ensure mockS3Client satisfies s3API.
var _ s3API = (*mockS3Client)(nil)

func testCfg() *config.Config {
	return &config.Config{
		S3: config.S3Config{Bucket: "test-bucket"},
	}
}

func TestS3Store_CreateProject_WritesKeepMarker(t *testing.T) {
	var gotKey string
	mock := &mockS3Client{
		PutObjectFn: func(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			if params.Key != nil {
				gotKey = *params.Key
			}
			return &s3.PutObjectOutput{}, nil
		},
	}
	store := newS3StoreWithClient(testCfg(), mock)
	if err := store.CreateProject(context.Background(), "myproject"); err != nil {
		t.Fatalf("CreateProject returned error: %v", err)
	}
	wantKey := "projects/myproject/.keep"
	if gotKey != wantKey {
		t.Errorf("CreateProject wrote key %q, want %q", gotKey, wantKey)
	}
}

func TestS3Store_CreateProject_PropagatesError(t *testing.T) {
	mock := &mockS3Client{
		PutObjectFn: func(_ context.Context, _ *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			return nil, errors.New("s3 unavailable")
		},
	}
	store := newS3StoreWithClient(testCfg(), mock)
	err := store.CreateProject(context.Background(), "myproject")
	if err == nil {
		t.Fatal("expected error when PutObject fails, got nil")
	}
}

func TestS3Store_ResultsDirHash_Noop(t *testing.T) {
	store := newS3StoreWithClient(testCfg(), &mockS3Client{})
	hash, err := store.ResultsDirHash(context.Background(), "myproject")
	if err != nil {
		t.Fatalf("ResultsDirHash returned error: %v", err)
	}
	if hash != "" {
		t.Errorf("expected empty hash, got %q", hash)
	}
}

func TestS3Store_ListProjects(t *testing.T) {
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
	store := newS3StoreWithClient(testCfg(), mock)
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
	var capturedKey string
	mock := &mockS3Client{
		PutObjectFn: func(_ context.Context, params *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
			if params.Key != nil {
				capturedKey = *params.Key
			}
			return &s3.PutObjectOutput{}, nil
		},
	}
	store := newS3StoreWithClient(testCfg(), mock)
	err := store.WriteResultFile(context.Background(), "myproject", "result.xml", strings.NewReader("data"))
	if err != nil {
		t.Fatalf("WriteResultFile returned error: %v", err)
	}
	expected := "projects/myproject/results/result.xml"
	if capturedKey != expected {
		t.Errorf("expected key %q, got %q", expected, capturedKey)
	}
}

func TestS3Store_CleanResults(t *testing.T) {
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
	store := newS3StoreWithClient(testCfg(), mock)
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
	store := newS3StoreWithClient(testCfg(), &mockS3Client{})
	err := store.DeleteReport(context.Background(), "myproject", "")
	if !errors.Is(err, ErrReportIDEmpty) {
		t.Errorf("expected ErrReportIDEmpty, got %v", err)
	}
}

func TestS3Store_DeleteReport_InvalidID(t *testing.T) {
	store := newS3StoreWithClient(testCfg(), &mockS3Client{})
	err := store.DeleteReport(context.Background(), "myproject", "latest")
	if !errors.Is(err, ErrReportIDInvalid) {
		t.Errorf("expected ErrReportIDInvalid, got %v", err)
	}
}

func TestS3Store_LatestReportExists_True(t *testing.T) {
	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents:    []s3types.Object{{Key: aws.String("projects/myproject/reports/latest/index.html")}},
				IsTruncated: aws.Bool(false),
			}, nil
		},
	}
	store := newS3StoreWithClient(testCfg(), mock)
	exists, err := store.LatestReportExists(context.Background(), "myproject")
	if err != nil {
		t.Fatalf("LatestReportExists returned error: %v", err)
	}
	if !exists {
		t.Error("expected LatestReportExists to return true")
	}
}

func TestS3Store_LatestReportExists_False(t *testing.T) {
	mock := &mockS3Client{
		ListObjectsV2Fn: func(_ context.Context, _ *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
			return &s3.ListObjectsV2Output{
				Contents:    []s3types.Object{},
				IsTruncated: aws.Bool(false),
			}, nil
		},
	}
	store := newS3StoreWithClient(testCfg(), mock)
	exists, err := store.LatestReportExists(context.Background(), "myproject")
	if err != nil {
		t.Fatalf("LatestReportExists returned error: %v", err)
	}
	if exists {
		t.Error("expected LatestReportExists to return false")
	}
}

func TestS3Store_ReadBuildStats_Summary(t *testing.T) {
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
	store := newS3StoreWithClient(testCfg(), mock)
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
	store := newS3StoreWithClient(testCfg(), mock)
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
