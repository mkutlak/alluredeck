package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/mkutlak/allure-docker-service/allure-docker-api/internal/config"
)

// S3Store implements Store backed by S3/MinIO.
type S3Store struct {
	cfg    *config.Config
	client s3API
	bucket string
}

// NewS3Store creates an S3Store from configuration.
func NewS3Store(cfg *config.Config) (*S3Store, error) {
	opts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.S3.Region),
	}
	if cfg.S3.AccessKey != "" {
		opts = append(opts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.S3.AccessKey, cfg.S3.SecretKey, ""),
		))
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(context.Background(), opts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config: %w", err)
	}

	clientOpts := []func(*s3.Options){}
	if cfg.S3.Endpoint != "" {
		endpoint := cfg.S3.Endpoint
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = cfg.S3.PathStyle
		})
	}

	client := s3.NewFromConfig(awsCfg, clientOpts...)
	return &S3Store{
		cfg:    cfg,
		client: client,
		bucket: cfg.S3.Bucket,
	}, nil
}

// newS3StoreWithClient creates an S3Store with a pre-built client (for testing).
func newS3StoreWithClient(cfg *config.Config, client s3API) *S3Store {
	return &S3Store{cfg: cfg, client: client, bucket: cfg.S3.Bucket}
}

// s3Key builds the S3 key from parts joined by "/".
func (s *S3Store) s3Key(parts ...string) string {
	return strings.Join(parts, "/")
}

// CreateProject writes a ".keep" marker object under the project prefix so that
// ProjectExists and ListProjects return correct results for newly created projects
// that have not yet had any results uploaded.  Without this marker, an empty
// project has no S3 objects and ProjectExists would return false, causing
// SendResults to 404.  The marker also allows SyncMetadata to rediscover the
// project from S3 after a SQLite wipe.
func (s *S3Store) CreateProject(ctx context.Context, projectID string) error {
	key := s.s3Key("projects", projectID, ".keep")
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        bytes.NewReader([]byte{}),
		ContentType: aws.String("application/octet-stream"),
	})
	if err != nil {
		return fmt.Errorf("create project marker for %q: %w", projectID, err)
	}
	return nil
}

// DeleteProject removes all S3 objects under the project prefix.
// Returns ErrProjectNotFound if the project has no objects in S3.
func (s *S3Store) DeleteProject(ctx context.Context, projectID string) error {
	exists, err := s.ProjectExists(ctx, projectID)
	if err != nil {
		return fmt.Errorf("check project %q: %w", projectID, err)
	}
	if !exists {
		return fmt.Errorf("project %q: %w", projectID, ErrProjectNotFound)
	}
	prefix := s.s3Key("projects", projectID) + "/"
	if err := deletePrefix(ctx, s.client, s.bucket, prefix); err != nil {
		return fmt.Errorf("delete project %q: %w", projectID, err)
	}
	return nil
}

// ProjectExists checks whether any objects exist under the project prefix.
func (s *S3Store) ProjectExists(ctx context.Context, projectID string) (bool, error) {
	prefix := s.s3Key("projects", projectID) + "/"
	out, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(s.bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return false, fmt.Errorf("list objects for project %q: %w", projectID, err)
	}
	return len(out.Contents) > 0, nil
}

// ListProjects returns all project IDs by listing common prefixes under "projects/".
func (s *S3Store) ListProjects(ctx context.Context) ([]string, error) {
	prefix := "projects/"
	var projects []string
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket:    aws.String(s.bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list projects: %w", err)
		}
		for _, cp := range page.CommonPrefixes {
			if cp.Prefix == nil {
				continue
			}
			// "projects/foo/" → "foo"
			trimmed := strings.TrimPrefix(*cp.Prefix, prefix)
			trimmed = strings.TrimSuffix(trimmed, "/")
			if trimmed != "" {
				projects = append(projects, trimmed)
			}
		}
	}
	return projects, nil
}

// WriteResultFile uploads a result file to S3.
func (s *S3Store) WriteResultFile(ctx context.Context, projectID, filename string, r io.Reader) error {
	// S3 PutObject requires Content-Length for streaming; buffer the content.
	data, err := io.ReadAll(r)
	if err != nil {
		return fmt.Errorf("read result file %q: %w", filename, err)
	}
	key := s.s3Key("projects", projectID, "results", filename)
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(data),
		ContentLength: aws.Int64(int64(len(data))),
	})
	if err != nil {
		return fmt.Errorf("put result file %q: %w", filename, err)
	}
	return nil
}

// ListResultFiles lists all result file names for a project.
func (s *S3Store) ListResultFiles(ctx context.Context, projectID string) ([]string, error) {
	prefix := s.s3Key("projects", projectID, "results") + "/"
	var names []string
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list result files: %w", err)
		}
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			name := strings.TrimPrefix(*obj.Key, prefix)
			if name != "" && !strings.Contains(name, "/") {
				names = append(names, name)
			}
		}
	}
	return names, nil
}

// CleanResults deletes all result files for a project.
func (s *S3Store) CleanResults(ctx context.Context, projectID string) error {
	prefix := s.s3Key("projects", projectID, "results") + "/"
	return deletePrefix(ctx, s.client, s.bucket, prefix)
}

// PrepareLocal creates a temp dir and downloads results + history from S3.
func (s *S3Store) PrepareLocal(ctx context.Context, projectID string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "allure-s3-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}

	// Download results
	resultsPrefix := s.s3Key("projects", projectID, "results") + "/"
	resultsDir := filepath.Join(tmpDir, "results")
	if err := downloadPrefix(ctx, s.client, s.bucket, resultsPrefix, resultsDir); err != nil {
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("download results: %w", err)
	}

	// Download history from latest report (for KeepHistory flow)
	historyPrefix := s.s3Key("projects", projectID, "reports", "latest", "history") + "/"
	historyDir := filepath.Join(tmpDir, "results", "history")
	if err := downloadPrefix(ctx, s.client, s.bucket, historyPrefix, historyDir); err != nil {
		// History may not exist — non-fatal
		log.Printf("S3Store.PrepareLocal: no history for %q (non-fatal): %v", projectID, err)
	}

	// Ensure reports dir exists for allure generate output
	if err := os.MkdirAll(filepath.Join(tmpDir, "reports"), 0o755); err != nil { //nolint:gosec // G301: needed for allure web server
		_ = os.RemoveAll(tmpDir)
		return "", fmt.Errorf("create reports dir: %w", err)
	}

	return tmpDir, nil
}

// CleanupLocal removes the temp directory created by PrepareLocal.
func (s *S3Store) CleanupLocal(localProjectDir string) error {
	if err := os.RemoveAll(localProjectDir); err != nil {
		return fmt.Errorf("cleanup local dir %q: %w", localProjectDir, err)
	}
	return nil
}

// PublishReport uploads the generated report from the local temp dir to S3.
// It uploads to both reports/latest/ and reports/{buildOrder}/.
func (s *S3Store) PublishReport(ctx context.Context, projectID string, buildOrder int, localProjectDir string) error {
	latestDir := filepath.Join(localProjectDir, "reports", "latest")

	// Upload to reports/latest/
	latestPrefix := s.s3Key("projects", projectID, "reports", "latest") + "/"
	// First clear old latest
	if err := deletePrefix(ctx, s.client, s.bucket, latestPrefix); err != nil {
		return fmt.Errorf("clear latest: %w", err)
	}
	if err := uploadDir(ctx, s.client, s.bucket, latestDir, latestPrefix); err != nil {
		return fmt.Errorf("upload to latest: %w", err)
	}

	// Upload only variable dirs (data, widgets, history) to reports/{buildOrder}/
	buildPrefix := s.s3Key("projects", projectID, "reports", strconv.Itoa(buildOrder)) + "/"
	for _, dir := range []string{"data", "widgets", "history"} {
		srcDir := filepath.Join(latestDir, dir)
		if _, err := os.Stat(srcDir); os.IsNotExist(err) {
			continue
		}
		dirPrefix := buildPrefix + dir + "/"
		if err := uploadDir(ctx, s.client, s.bucket, srcDir, dirPrefix); err != nil {
			return fmt.Errorf("upload %s to build %d: %w", dir, buildOrder, err)
		}
	}
	return nil
}

// DeleteReport removes all S3 objects for a numbered report.
func (s *S3Store) DeleteReport(ctx context.Context, projectID, reportID string) error {
	if reportID == "" {
		return ErrReportIDEmpty
	}
	for _, ch := range reportID {
		if ch < '0' || ch > '9' {
			return fmt.Errorf("report ID %q: %w", reportID, ErrReportIDInvalid)
		}
	}
	prefix := s.s3Key("projects", projectID, "reports", reportID) + "/"
	if err := deletePrefix(ctx, s.client, s.bucket, prefix); err != nil {
		return fmt.Errorf("delete report %q: %w", reportID, err)
	}
	return nil
}

// PruneReportDirs removes S3 objects for multiple build orders.
func (s *S3Store) PruneReportDirs(ctx context.Context, projectID string, buildOrders []int) error {
	for _, bo := range buildOrders {
		prefix := s.s3Key("projects", projectID, "reports", strconv.Itoa(bo)) + "/"
		if err := deletePrefix(ctx, s.client, s.bucket, prefix); err != nil {
			return fmt.Errorf("prune build %d: %w", bo, err)
		}
	}
	return nil
}

// KeepHistory copies history from reports/latest/history/ to results/history/ in S3.
// This is used before report generation to preserve trend history.
func (s *S3Store) KeepHistory(ctx context.Context, projectID string) error {
	if !s.cfg.KeepHistory {
		// Delete results/history/ prefix when history disabled
		histPrefix := s.s3Key("projects", projectID, "results", "history") + "/"
		return deletePrefix(ctx, s.client, s.bucket, histPrefix)
	}

	// Copy from reports/latest/history/ to results/history/
	srcPrefix := s.s3Key("projects", projectID, "reports", "latest", "history") + "/"
	dstPrefix := s.s3Key("projects", projectID, "results", "history") + "/"

	// List source objects
	var srcKeys []string
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(srcPrefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("list history objects: %w", err)
		}
		for _, obj := range page.Contents {
			if obj.Key != nil {
				srcKeys = append(srcKeys, *obj.Key)
			}
		}
	}
	if len(srcKeys) == 0 {
		return nil // no history to copy
	}

	// Clear destination, then copy
	if err := deletePrefix(ctx, s.client, s.bucket, dstPrefix); err != nil {
		return fmt.Errorf("clear results history: %w", err)
	}
	for _, srcKey := range srcKeys {
		relPath := strings.TrimPrefix(srcKey, srcPrefix)
		dstKey := dstPrefix + relPath
		// Download then upload (S3 copy-object requires same-region same-bucket or complex setup)
		out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
			Bucket: aws.String(s.bucket),
			Key:    aws.String(srcKey),
		})
		if err != nil {
			return fmt.Errorf("get history object %q: %w", srcKey, err)
		}
		data, err := io.ReadAll(out.Body)
		_ = out.Body.Close()
		if err != nil {
			return fmt.Errorf("read history object %q: %w", srcKey, err)
		}
		if _, err := s.client.PutObject(ctx, &s3.PutObjectInput{
			Bucket:        aws.String(s.bucket),
			Key:           aws.String(dstKey),
			Body:          bytes.NewReader(data),
			ContentLength: aws.Int64(int64(len(data))),
		}); err != nil {
			return fmt.Errorf("put history object %q: %w", dstKey, err)
		}
	}
	return nil
}

// CleanHistory removes all reports and history data for a project.
func (s *S3Store) CleanHistory(ctx context.Context, projectID string) error {
	// Clear latest report
	latestPrefix := s.s3Key("projects", projectID, "reports", "latest") + "/"
	if err := deletePrefix(ctx, s.client, s.bucket, latestPrefix); err != nil {
		return err
	}

	// Clear all numbered reports
	builds, err := s.ListReportBuilds(ctx, projectID)
	if err != nil {
		return err
	}
	reportsPrefix := s.s3Key("projects", projectID, "reports") + "/"
	for _, bo := range builds {
		buildPrefix := reportsPrefix + strconv.Itoa(bo) + "/"
		if err := deletePrefix(ctx, s.client, s.bucket, buildPrefix); err != nil {
			return err
		}
	}

	// Clear results history
	histPrefix := s.s3Key("projects", projectID, "results", "history") + "/"
	return deletePrefix(ctx, s.client, s.bucket, histPrefix)
}

// ReadBuildStats reads widget stats for a build from S3.
func (s *S3Store) ReadBuildStats(ctx context.Context, projectID string, buildOrder int) (BuildStats, error) {
	widgetsPrefix := s.s3Key("projects", projectID, "reports", strconv.Itoa(buildOrder), "widgets")

	// Try Allure 2: summary.json
	if data, err := s.getObjectBytes(ctx, widgetsPrefix+"/summary.json"); err == nil {
		var summary struct {
			Statistic struct {
				Passed  int `json:"passed"`
				Failed  int `json:"failed"`
				Broken  int `json:"broken"`
				Skipped int `json:"skipped"`
				Unknown int `json:"unknown"`
				Total   int `json:"total"`
			} `json:"statistic"`
			Time *struct {
				Duration int64 `json:"duration"`
			} `json:"time"`
		}
		if json.Unmarshal(data, &summary) == nil {
			stats := BuildStats{
				Passed:  summary.Statistic.Passed,
				Failed:  summary.Statistic.Failed,
				Broken:  summary.Statistic.Broken,
				Skipped: summary.Statistic.Skipped,
				Unknown: summary.Statistic.Unknown,
				Total:   summary.Statistic.Total,
			}
			if summary.Time != nil {
				stats.DurationMs = summary.Time.Duration
			}
			return stats, nil
		}
	}

	// Try Allure 3: statistic.json
	if data, err := s.getObjectBytes(ctx, widgetsPrefix+"/statistic.json"); err == nil {
		var stat struct {
			Passed  int `json:"passed"`
			Failed  int `json:"failed"`
			Broken  int `json:"broken"`
			Skipped int `json:"skipped"`
			Unknown int `json:"unknown"`
			Total   int `json:"total"`
		}
		if json.Unmarshal(data, &stat) == nil && stat.Total > 0 {
			return BuildStats{
				Passed:  stat.Passed,
				Failed:  stat.Failed,
				Broken:  stat.Broken,
				Skipped: stat.Skipped,
				Unknown: stat.Unknown,
				Total:   stat.Total,
			}, nil
		}
	}

	return BuildStats{}, fmt.Errorf("build %d: %w", buildOrder, ErrStatsNotFound)
}

// ReadFile reads a project-relative file from S3.
func (s *S3Store) ReadFile(ctx context.Context, projectID, relPath string) ([]byte, error) {
	key := s.s3Key("projects", projectID, relPath)
	return s.getObjectBytes(ctx, key)
}

// s3DirEntryFiles builds file DirEntry values from page.Contents, stripping prefix.
func s3DirEntryFiles(page *s3.ListObjectsV2Output, prefix string) []DirEntry {
	var entries []DirEntry
	for _, obj := range page.Contents {
		if obj.Key == nil {
			continue
		}
		name := strings.TrimPrefix(*obj.Key, prefix)
		if name == "" || strings.Contains(name, "/") {
			continue
		}
		var size int64
		if obj.Size != nil {
			size = *obj.Size
		}
		entries = append(entries, DirEntry{Name: name, Size: size, IsDir: false})
	}
	return entries
}

// s3DirEntrySubdirs builds subdir DirEntry values from page.CommonPrefixes, stripping prefix.
func s3DirEntrySubdirs(page *s3.ListObjectsV2Output, prefix string) []DirEntry {
	var entries []DirEntry
	for _, cp := range page.CommonPrefixes {
		if cp.Prefix == nil {
			continue
		}
		name := strings.TrimPrefix(*cp.Prefix, prefix)
		name = strings.TrimSuffix(name, "/")
		if name != "" {
			entries = append(entries, DirEntry{Name: name, IsDir: true})
		}
	}
	return entries
}

// ReadDir lists objects under a project-relative path prefix.
func (s *S3Store) ReadDir(ctx context.Context, projectID, relPath string) ([]DirEntry, error) {
	prefix := s.s3Key("projects", projectID, relPath) + "/"
	var entries []DirEntry
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket:    aws.String(s.bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list dir %q: %w", relPath, err)
		}
		entries = append(entries, s3DirEntryFiles(page, prefix)...)
		entries = append(entries, s3DirEntrySubdirs(page, prefix)...)
	}
	return entries, nil
}

// OpenReportFile downloads a report file from S3 and returns a reader.
func (s *S3Store) OpenReportFile(ctx context.Context, projectID, reportID, filePath string) (io.ReadCloser, string, error) {
	key := s.s3Key("projects", projectID, "reports", reportID, filePath)
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, "", fmt.Errorf("get report file: %w", err)
	}
	contentType := mime.TypeByExtension(filepath.Ext(filePath))
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return out.Body, contentType, nil
}

// ListReportBuilds returns all numeric build orders for a project.
func (s *S3Store) ListReportBuilds(ctx context.Context, projectID string) ([]int, error) {
	prefix := s.s3Key("projects", projectID, "reports") + "/"
	var builds []int
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket:    aws.String(s.bucket),
		Prefix:    aws.String(prefix),
		Delimiter: aws.String("/"),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list report builds: %w", err)
		}
		for _, cp := range page.CommonPrefixes {
			if cp.Prefix == nil {
				continue
			}
			name := strings.TrimPrefix(*cp.Prefix, prefix)
			name = strings.TrimSuffix(name, "/")
			if bo, err := strconv.Atoi(name); err == nil {
				builds = append(builds, bo)
			}
		}
	}
	return builds, nil
}

// LatestReportExists checks if reports/latest/ has any objects.
func (s *S3Store) LatestReportExists(ctx context.Context, projectID string) (bool, error) {
	prefix := s.s3Key("projects", projectID, "reports", "latest") + "/"
	out, err := s.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket:  aws.String(s.bucket),
		Prefix:  aws.String(prefix),
		MaxKeys: aws.Int32(1),
	})
	if err != nil {
		return false, fmt.Errorf("check latest report: %w", err)
	}
	return len(out.Contents) > 0, nil
}

// ResultsDirHash returns ("", nil) — watcher is always disabled in S3 mode.
func (s *S3Store) ResultsDirHash(_ context.Context, _ string) (string, error) {
	return "", nil
}

// getObjectBytes is a helper that downloads a full S3 object to memory.
func (s *S3Store) getObjectBytes(ctx context.Context, key string) ([]byte, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("get object %q: %w", key, err)
	}
	defer out.Body.Close()
	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, fmt.Errorf("read object %q: %w", key, err)
	}
	return data, nil
}

// Ensure S3Store implements Store at compile time.
var _ Store = (*S3Store)(nil)
