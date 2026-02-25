package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
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

// uploadDir walks localDir and uploads each file to S3 under s3Prefix.
func uploadDir(ctx context.Context, client s3API, bucket, localDir, s3Prefix string) error {
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
		// Use forward slashes for S3 keys
		s3Key := s3Prefix + filepath.ToSlash(rel)

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("read %q: %w", path, err)
		}
		if _, err := client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:        aws.String(bucket),
			Key:           aws.String(s3Key),
			Body:          bytes.NewReader(data),
			ContentLength: aws.Int64(int64(len(data))),
		}); err != nil {
			return fmt.Errorf("upload %q: %w", s3Key, err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("walk %q: %w", localDir, err)
	}
	return nil
}

// downloadObject downloads a single S3 object (identified by key) into localDir,
// mirroring the path structure relative to s3Prefix.
func downloadObject(ctx context.Context, client s3API, bucket, key, localDir, s3Prefix string) error {
	rel := strings.TrimPrefix(key, s3Prefix)
	if rel == "" {
		return nil
	}
	localPath := filepath.Join(localDir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(localPath), 0o755); err != nil { //nolint:gosec // G301: needed for allure web server
		return fmt.Errorf("mkdir for %q: %w", localPath, err)
	}
	out, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("get %q: %w", key, err)
	}
	data, readErr := io.ReadAll(out.Body)
	_ = out.Body.Close()
	if readErr != nil {
		return fmt.Errorf("read %q: %w", key, readErr)
	}
	if err := os.WriteFile(localPath, data, 0o644); err != nil { //nolint:gosec // G306: standard file permissions
		return fmt.Errorf("write %q: %w", localPath, err)
	}
	return nil
}

// downloadPrefix downloads all objects with the given S3 prefix into localDir.
// Creates parent directories as needed.
func downloadPrefix(ctx context.Context, client s3API, bucket, s3Prefix, localDir string) error {
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
			if obj.Key == nil {
				continue
			}
			if err := downloadObject(ctx, client, bucket, *obj.Key, localDir, s3Prefix); err != nil {
				return err
			}
		}
	}
	return nil
}
