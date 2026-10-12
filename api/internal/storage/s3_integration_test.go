//go:build integration

package storage_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"slices"
	"sort"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
	"github.com/mkutlak/alluredeck/api/internal/storage"
)

// Integration tests require a running MinIO instance.
// Start with: docker run -p 9000:9000 -p 9001:9001 pgsty/silo server /data
// Set env vars: TEST_S3_ENDPOINT, S3_BUCKET, S3_ACCESS_KEY, S3_SECRET_KEY
// Run with:    go test -tags=integration ./internal/storage/...

func integrationConfig(t *testing.T) *config.Config {
	t.Helper()
	endpoint := os.Getenv("TEST_S3_ENDPOINT")
	if endpoint == "" {
		t.Skip("TEST_S3_ENDPOINT not set; skipping S3 integration test")
	}
	bucket := getEnvOrDefault("S3_BUCKET", "allure-integration-test")
	accessKey := getEnvOrDefault("S3_ACCESS_KEY", "minioadmin")
	secretKey := getEnvOrDefault("S3_SECRET_KEY", "minioadmin")

	return &config.Config{
		StorageType: "s3",
		KeepHistory: true,
		S3: config.S3Config{
			Endpoint:              endpoint,
			Bucket:                bucket,
			Region:                "us-east-1",
			AccessKey:             accessKey,
			SecretKey:             secretKey,
			TLSInsecureSkipVerify: false,
			PathStyle:             true,
		},
	}
}

func getEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func TestS3Store_Integration_FullWorkflow(t *testing.T) {
	cfg := integrationConfig(t)
	st, err := storage.NewS3Store(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}

	ctx := context.Background()
	projectID := fmt.Sprintf("integration-test-%d", time.Now().UnixNano())
	batchID := fmt.Sprintf("batch-%d", time.Now().UnixNano())

	// CreateProject is a no-op but should not error
	if err := st.CreateProject(ctx, projectID); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	// Write a result file
	content := []byte(`{"name":"test","status":"passed"}`)
	if err := st.WriteResultFile(ctx, projectID, batchID, "result.json", bytes.NewReader(content)); err != nil {
		t.Fatalf("WriteResultFile: %v", err)
	}

	// List result files
	files, err := st.ListResultFiles(ctx, projectID, batchID)
	if err != nil {
		t.Fatalf("ListResultFiles: %v", err)
	}
	if len(files) != 1 || files[0] != "result.json" {
		t.Errorf("ListResultFiles: want [result.json], got %v", files)
	}

	// ProjectExists
	exists, err := st.ProjectExists(ctx, projectID)
	if err != nil {
		t.Fatalf("ProjectExists: %v", err)
	}
	if !exists {
		t.Error("ProjectExists: want true, got false")
	}

	// ListProjects
	projects, err := st.ListProjects(ctx)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	found := false
	for _, p := range projects {
		if p == projectID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ListProjects: project %q not found in %v", projectID, projects)
	}

	// ResultsDirHash should return ("", nil)
	hash, err := st.ResultsDirHash(ctx, projectID)
	if err != nil {
		t.Fatalf("ResultsDirHash: %v", err)
	}
	if hash != "" {
		t.Errorf("ResultsDirHash: want empty string, got %q", hash)
	}

	// CleanResults
	if err := st.CleanResults(ctx, projectID); err != nil {
		t.Fatalf("CleanResults: %v", err)
	}
	files, _ = st.ListResultFiles(ctx, projectID, batchID)
	if len(files) != 0 {
		t.Errorf("CleanResults: expected 0 files, got %v", files)
	}

	t.Log("Integration test passed for project:", projectID)
}

// TestS3Store_Integration_RenameEscapedKeys renames a project whose result
// files have names that must be URL-escaped in the copy source (a space, a
// plus sign and a percent sign) and expects all of them under the new id.
func TestS3Store_Integration_RenameEscapedKeys(t *testing.T) {
	cfg := integrationConfig(t)
	st, err := storage.NewS3Store(cfg, zap.NewNop())
	if err != nil {
		t.Fatalf("NewS3Store: %v", err)
	}

	ctx := context.Background()
	oldID := fmt.Sprintf("integration-rename-%d", time.Now().UnixNano())
	newID := oldID + "-renamed"
	t.Cleanup(func() {
		_ = st.DeleteProject(ctx, oldID)
		_ = st.DeleteProject(ctx, newID)
	})

	names := []string{"100%.json", "a%20b.json", "a+b.json", "my report.json"}
	for _, name := range names {
		if err := st.WriteResultFile(ctx, oldID, "batch", name, bytes.NewReader([]byte(name))); err != nil {
			t.Fatalf("WriteResultFile(%q): %v", name, err)
		}
	}
	if err := st.RenameProject(ctx, oldID, newID); err != nil {
		t.Fatalf("RenameProject: %v", err)
	}

	got, err := st.ListResultFiles(ctx, newID, "batch")
	if err != nil {
		t.Fatalf("ListResultFiles: %v", err)
	}
	sort.Strings(got)
	if !slices.Equal(got, names) {
		t.Errorf("files under the new project = %v, want %v", got, names)
	}
	if left, _ := st.ListResultFiles(ctx, oldID, "batch"); len(left) != 0 {
		t.Errorf("files left under the old project = %v, want none", left)
	}
}
