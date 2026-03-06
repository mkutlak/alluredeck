package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"
	"golang.org/x/sync/errgroup"
)

// deletePrefix deletes all S3 objects with the given prefix (batch delete, max 1000/request).
func deletePrefix(ctx context.Context, client s3API, bucket, prefix string) error {
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list for delete %q: %w", prefix, err)
		}
		if len(page.Contents) == 0 {
			continue
		}
		var objs []s3types.ObjectIdentifier
		for _, obj := range page.Contents {
			if obj.Key != nil {
				objs = append(objs, s3types.ObjectIdentifier{Key: obj.Key})
			}
		}
		if _, err := client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(bucket),
			Delete: &s3types.Delete{Objects: objs, Quiet: aws.Bool(true)},
		}); err != nil {
			return fmt.Errorf("delete objects under %q: %w", prefix, err)
		}
	}
	return nil
}

// fileEntry holds a local file path and its S3-relative path for uploads.
type fileEntry struct {
	path  string
	s3Key string
}

// uploadDir walks localDir and uploads each file to S3 under s3Prefix using bounded
// concurrency. Phase 1: walk to collect file entries (sequential). Phase 2: fan-out
// uploads with errgroup limited to concurrency workers.
// Files are streamed directly via the uploader — no full-file buffering in memory.
func uploadDir(ctx context.Context, uploader s3Uploader, bucket, localDir, s3Prefix string, concurrency int) error {
	if concurrency <= 0 {
		concurrency = 10
	}

	// Phase 1: collect all file entries (Walk is not goroutine-safe).
	var entries []fileEntry
	if err := filepath.Walk(localDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(localDir, path)
		if err != nil {
			return fmt.Errorf("rel path for %q: %w", path, err)
		}
		entries = append(entries, fileEntry{
			path:  path,
			s3Key: s3Prefix + filepath.ToSlash(rel),
		})
		return nil
	}); err != nil {
		return fmt.Errorf("walk %q: %w", localDir, err)
	}

	if len(entries) == 0 {
		return nil
	}

	// Phase 2: fan-out uploads with bounded concurrency.
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for _, e := range entries {
		g.Go(func() error {
			f, err := os.Open(e.path)
			if err != nil {
				return fmt.Errorf("open %q: %w", e.path, err)
			}
			defer func() { _ = f.Close() }()
			if _, err := uploader.UploadObject(gctx, &transfermanager.UploadObjectInput{
				Bucket: aws.String(bucket),
				Key:    aws.String(e.s3Key),
				Body:   f,
			}); err != nil {
				return fmt.Errorf("upload %q: %w", e.s3Key, err)
			}
			return nil
		})
	}

	return g.Wait()
}

// downloadObject downloads a single S3 object (identified by key) into localDir,
// mirroring the path structure relative to s3Prefix.
// Returns the number of bytes written on success.
func downloadObject(ctx context.Context, client s3API, bucket, key, localDir, s3Prefix string) (int64, error) {
	rel := strings.TrimPrefix(key, s3Prefix)
	if rel == "" {
		return 0, nil
	}
	localPath := filepath.Join(localDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil { //nolint:gosec // G301: needed for allure web server
		return 0, fmt.Errorf("mkdir for %q: %w", localPath, err)
	}
	out, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return 0, fmt.Errorf("get %q: %w", key, err)
	}
	data, readErr := io.ReadAll(out.Body)
	_ = out.Body.Close()
	if readErr != nil {
		return 0, fmt.Errorf("read %q: %w", key, readErr)
	}
	if err := os.WriteFile(localPath, data, 0o644); err != nil { //nolint:gosec // G306: standard file permissions
		return 0, fmt.Errorf("write %q: %w", localPath, err)
	}
	return int64(len(data)), nil
}

// downloadPrefix downloads all objects with the given S3 prefix into localDir using
// bounded concurrency. Phase 1: paginate and collect all keys (sequential — the paginator
// is stateful and not goroutine-safe). Phase 2: fan-out downloads with errgroup limited
// to concurrency workers.
//
// If sizeWarnBytes > 0 and the total downloaded bytes exceed that threshold, a warning
// is logged but the operation continues normally.
func downloadPrefix(ctx context.Context, client s3API, bucket, s3Prefix, localDir string, concurrency int, sizeWarnBytes int64, logger *zap.Logger) error {
	if concurrency <= 0 {
		concurrency = 10
	}

	// Phase 1: collect all keys (paginator is not goroutine-safe).
	var keys []string
	paginator := s3.NewListObjectsV2Paginator(client, &s3.ListObjectsV2Input{
		Bucket: aws.String(bucket),
		Prefix: aws.String(s3Prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list for download %q: %w", s3Prefix, err)
		}
		for _, obj := range page.Contents {
			if obj.Key != nil {
				keys = append(keys, *obj.Key)
			}
		}
	}

	if len(keys) == 0 {
		return nil
	}

	// Phase 2: fan-out downloads with bounded concurrency.
	var totalSize atomic.Int64
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(concurrency)

	for _, key := range keys {
		g.Go(func() error {
			n, err := downloadObject(gctx, client, bucket, key, localDir, s3Prefix)
			if err != nil {
				return err
			}
			totalSize.Add(n)
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
	}

	// Soft size guard: warn if total download exceeds threshold.
	if sizeWarnBytes > 0 && totalSize.Load() > sizeWarnBytes {
		logger.Warn("S3 download exceeded size threshold",
			zap.String("prefix", s3Prefix),
			zap.Int64("total_bytes", totalSize.Load()),
			zap.Int64("warn_threshold_bytes", sizeWarnBytes),
		)
	}

	return nil
}
