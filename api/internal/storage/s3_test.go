package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/transfermanager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"go.uber.org/zap"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

const testBucket = "test-bucket"

var errBoom = errors.New("boom")

// fakeS3 is an in-memory bucket behind both s3API and s3Uploader. Every call
// is logged as "Op key" ("List prefix" for listings, "Copy source" for copies)
// and fails when failOn holds that entry; calls to another bucket fail too.
// The hooks run before a GetObject/UploadObject is served, to add latency or
// cancel the context.
type fakeS3 struct {
	mu                  sync.Mutex
	objects             map[string]string
	calls               []string
	failOn              map[string]error
	getHook, uploadHook func(ctx context.Context) error
}

var (
	_ s3API      = (*fakeS3)(nil)
	_ s3Uploader = (*fakeS3)(nil)
)

func newFakeS3(objects map[string]string) *fakeS3 {
	f := &fakeS3{objects: map[string]string{}}
	for k, v := range objects {
		f.objects[k] = v
	}
	return f
}

func (f *fakeS3) record(bucket *string, op, key string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, op+" "+key)
	if b := aws.ToString(bucket); b != testBucket {
		return fmt.Errorf("%s %s: unexpected bucket %q", op, key, b)
	}
	return f.failOn[op+" "+key]
}

func (f *fakeS3) get(key string) (string, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.objects[key]
	return v, ok
}

func (f *fakeS3) put(key string, r io.Reader) error {
	b, err := io.ReadAll(r)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.objects[key] = string(b)
	return err
}

func (f *fakeS3) remove(key string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.objects, key)
}

// keys returns the stored keys under prefix, sorted.
func (f *fakeS3) keys(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for k := range f.objects {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// count returns how many calls of op were made.
func (f *fakeS3) count(op string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, c := range f.calls {
		if strings.HasPrefix(c, op+" ") {
			n++
		}
	}
	return n
}

func (f *fakeS3) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	if err := f.record(in.Bucket, "Put", aws.ToString(in.Key)); err != nil {
		return nil, err
	}
	return &s3.PutObjectOutput{}, f.put(aws.ToString(in.Key), in.Body)
}

func (f *fakeS3) GetObject(ctx context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if f.getHook != nil {
		if err := f.getHook(ctx); err != nil {
			return nil, err
		}
	}
	key := aws.ToString(in.Key)
	if err := f.record(in.Bucket, "Get", key); err != nil {
		return nil, err
	}
	v, ok := f.get(key)
	if !ok {
		return nil, &s3types.NoSuchKey{}
	}
	return &s3.GetObjectOutput{Body: io.NopCloser(strings.NewReader(v))}, nil
}

func (f *fakeS3) DeleteObject(_ context.Context, in *s3.DeleteObjectInput, _ ...func(*s3.Options)) (*s3.DeleteObjectOutput, error) {
	if err := f.record(in.Bucket, "Delete", aws.ToString(in.Key)); err != nil {
		return nil, err
	}
	f.remove(aws.ToString(in.Key))
	return &s3.DeleteObjectOutput{}, nil
}

func (f *fakeS3) DeleteObjects(_ context.Context, in *s3.DeleteObjectsInput, _ ...func(*s3.Options)) (*s3.DeleteObjectsOutput, error) {
	for _, obj := range in.Delete.Objects {
		if err := f.record(in.Bucket, "Delete", aws.ToString(obj.Key)); err != nil {
			return nil, err
		}
		f.remove(aws.ToString(obj.Key))
	}
	return &s3.DeleteObjectsOutput{}, nil
}

// ListObjectsV2 returns every key under the prefix in one page, grouping keys
// past the delimiter into common prefixes.
func (f *fakeS3) ListObjectsV2(_ context.Context, in *s3.ListObjectsV2Input, _ ...func(*s3.Options)) (*s3.ListObjectsV2Output, error) {
	prefix := aws.ToString(in.Prefix)
	if err := f.record(in.Bucket, "List", prefix); err != nil {
		return nil, err
	}
	out := &s3.ListObjectsV2Output{IsTruncated: aws.Bool(false)}
	seen := map[string]bool{}
	for _, k := range f.keys(prefix) {
		v, _ := f.get(k)
		rest := strings.TrimPrefix(k, prefix)
		if d := aws.ToString(in.Delimiter); d != "" && strings.Contains(rest, d) {
			cp := prefix + rest[:strings.Index(rest, d)+len(d)]
			if !seen[cp] {
				seen[cp] = true
				out.CommonPrefixes = append(out.CommonPrefixes, s3types.CommonPrefix{Prefix: aws.String(cp)})
			}
			continue
		}
		out.Contents = append(out.Contents, s3types.Object{Key: aws.String(k), Size: aws.Int64(int64(len(v)))})
	}
	return out, nil
}

func (f *fakeS3) HeadObject(_ context.Context, in *s3.HeadObjectInput, _ ...func(*s3.Options)) (*s3.HeadObjectOutput, error) {
	if err := f.record(in.Bucket, "Head", aws.ToString(in.Key)); err != nil {
		return nil, err
	}
	if _, ok := f.get(aws.ToString(in.Key)); !ok {
		return nil, &s3types.NotFound{}
	}
	return &s3.HeadObjectOutput{}, nil
}

func (f *fakeS3) HeadBucket(_ context.Context, in *s3.HeadBucketInput, _ ...func(*s3.Options)) (*s3.HeadBucketOutput, error) {
	return &s3.HeadBucketOutput{}, f.record(in.Bucket, "HeadBucket", "")
}

// CopyObject copies within the bucket; CopySource must be "bucket/key".
func (f *fakeS3) CopyObject(_ context.Context, in *s3.CopyObjectInput, _ ...func(*s3.Options)) (*s3.CopyObjectOutput, error) {
	source := aws.ToString(in.CopySource)
	if err := f.record(in.Bucket, "Copy", source); err != nil {
		return nil, err
	}
	key, ok := strings.CutPrefix(source, testBucket+"/")
	if !ok {
		return nil, fmt.Errorf("copy source %q is not %q+key", source, testBucket+"/")
	}
	v, ok := f.get(key)
	if !ok {
		return nil, &s3types.NoSuchKey{}
	}
	return &s3.CopyObjectOutput{}, f.put(aws.ToString(in.Key), strings.NewReader(v))
}

func (f *fakeS3) UploadObject(ctx context.Context, in *transfermanager.UploadObjectInput, _ ...func(*transfermanager.Options)) (*transfermanager.UploadObjectOutput, error) {
	if f.uploadHook != nil {
		if err := f.uploadHook(ctx); err != nil {
			return nil, err
		}
	}
	if err := f.record(in.Bucket, "Upload", aws.ToString(in.Key)); err != nil {
		return nil, err
	}
	return &transfermanager.UploadObjectOutput{}, f.put(aws.ToString(in.Key), in.Body)
}

func newTestS3Store(f *fakeS3, keepHistory bool) *S3Store {
	cfg := &config.Config{KeepHistory: keepHistory, S3: config.S3Config{Bucket: testBucket, Concurrency: 10}}
	return newS3StoreWithClient(cfg, f, f, zap.NewNop())
}

// readTree maps every regular file under dir (slash path) to its content.
func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		rel, _ := filepath.Rel(dir, path)
		out[filepath.ToSlash(rel)] = string(data)
		return err
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("walk %s: %v", dir, err)
	}
	return out
}

// TestS3Store_ProjectsAndResults: projects live under projects/<id>/, where a
// .keep marker makes an empty project visible; result files are keyed
// projects/<id>/results/<batch>/<file>; CleanResults deletes only results/.
// ResultsDirHash is "", which tells the watcher to stay idle in S3 mode.
func TestS3Store_ProjectsAndResults(t *testing.T) {
	t.Parallel()
	f := newFakeS3(nil)
	st := newTestS3Store(f, true)
	ctx := context.Background()

	for _, id := range []string{"alpha", "beta"} {
		if err := st.CreateProject(ctx, id); err != nil {
			t.Fatalf("CreateProject(%s): %v", id, err)
		}
	}
	if err := st.WriteResultFile(ctx, "alpha", "batch1", "result.xml", strings.NewReader("data")); err != nil {
		t.Fatalf("WriteResultFile: %v", err)
	}
	want := []string{"projects/alpha/.keep", "projects/alpha/results/batch1/result.xml", "projects/beta/.keep"}
	if got := f.keys(""); !slices.Equal(got, want) {
		t.Errorf("keys = %v, want %v", got, want)
	}
	if projects, err := st.ListProjects(ctx); err != nil || !slices.Equal(projects, []string{"alpha", "beta"}) {
		t.Errorf("ListProjects = %v, %v", projects, err)
	}
	if hash, err := st.ResultsDirHash(ctx, "alpha"); err != nil || hash != "" {
		t.Errorf("ResultsDirHash = %q, %v; want empty", hash, err)
	}
	if err := st.CleanResults(ctx, "alpha"); err != nil {
		t.Fatalf("CleanResults: %v", err)
	}
	if got := f.keys(""); !slices.Equal(got, []string{"projects/alpha/.keep", "projects/beta/.keep"}) {
		t.Errorf("keys after CleanResults = %v", got)
	}

	f.failOn = map[string]error{"Put projects/p/.keep": errBoom, "Upload projects/p/results/b/r.xml": errBoom}
	if err := st.CreateProject(ctx, "p"); !errors.Is(err, errBoom) {
		t.Errorf("CreateProject with a failing PutObject = %v", err)
	}
	if err := st.WriteResultFile(ctx, "p", "b", "r.xml", strings.NewReader("x")); !errors.Is(err, errBoom) {
		t.Errorf("WriteResultFile with a failing upload = %v", err)
	}
}

// TestS3Store_Reports reads report state under projects/<id>/reports/:
// latest exists iff it holds objects, builds are the numeric common prefixes,
// and stats come from the Allure 2 summary widget or, for Allure 3, from
// statistic.json with the duration taken from the test result files. Report
// IDs are validated.
func TestS3Store_Reports(t *testing.T) {
	t.Parallel()
	st := newTestS3Store(newFakeS3(map[string]string{
		"projects/p/reports/latest/index.html": "<html/>",
		"projects/p/reports/1/widgets/summary.json": `{"statistic":{"passed":10,"failed":2,"broken":1,"skipped":3,"unknown":0,"total":16},` +
			`"time":{"duration":5000}}`,
		"projects/p/reports/2/index.html": "<html/>",
		// Build 2 is Allure 3: statistic.json has no timing, so the duration is the
		// wall clock from the earliest start to the latest stop (not the 6000ms sum).
		"projects/p/reports/2/widgets/statistic.json":   `{"passed":3,"failed":1,"broken":0,"skipped":0,"unknown":0,"total":4}`,
		"projects/p/reports/2/data/test-results/a.json": `{"start":1700000000000,"stop":1700000002000}`,
		"projects/p/reports/2/data/test-results/b.json": `{"start":1700000001000,"stop":1700000005000}`,
	}), true)
	ctx := context.Background()

	for id, want := range map[string]bool{"p": true, "q": false} {
		if ok, err := st.LatestReportExists(ctx, id); err != nil || ok != want {
			t.Errorf("LatestReportExists(%s) = %v, %v; want %v", id, ok, err, want)
		}
	}
	if builds, err := st.ListReportBuilds(ctx, "p"); err != nil || !slices.Equal(builds, []int{1, 2}) {
		t.Errorf("ListReportBuilds = %v, %v", builds, err)
	}
	for build, want := range map[int]BuildStats{
		1: {Passed: 10, Failed: 2, Broken: 1, Skipped: 3, Total: 16, DurationMs: 5000},
		2: {Passed: 3, Failed: 1, Total: 4, DurationMs: 5000},
	} {
		if stats, err := st.ReadBuildStats(ctx, "p", build); err != nil || stats != want {
			t.Errorf("ReadBuildStats(%d) = %+v, %v; want %+v", build, stats, err, want)
		}
	}
	for id, want := range map[string]error{"": ErrReportIDEmpty, "latest": ErrReportIDInvalid} {
		if err := st.DeleteReport(ctx, "p", id); !errors.Is(err, want) {
			t.Errorf("DeleteReport(%q) = %v, want %v", id, err, want)
		}
	}
}

// TestS3Store_KeepHistory copies reports/latest/history to results/history
// server-side (CopyObject, never a download); with KEEP_HISTORY off it
// deletes results/history instead.
func TestS3Store_KeepHistory(t *testing.T) {
	t.Parallel()
	const src, dst = "projects/p/reports/latest/history/", "projects/p/results/history/"
	history := map[string]string{src + "history.json": "h", src + "retry-trend.json": "r"}
	withStale := map[string]string{src + "history.json": "h", dst + "stale.json": "old"}
	tests := []struct {
		name      string
		keep      bool
		objects   map[string]string
		failOn    map[string]error
		wantDst   map[string]string // objects under results/history afterwards
		wantCopy  int
		wantInErr string
	}{
		{"copies history", true, history, nil, map[string]string{dst + "history.json": "h", dst + "retry-trend.json": "r"}, 2, ""},
		{"no history copies nothing", true, nil, nil, map[string]string{}, 0, ""},
		{"disabled deletes results/history", false, withStale, nil, map[string]string{}, 0, ""},
		{"copy failure", true, history, map[string]error{"Copy " + testBucket + "/" + src + "history.json": errBoom}, nil, 1, "copy history object"},
	}
	for _, tc := range tests {
		f := newFakeS3(tc.objects)
		f.failOn = tc.failOn
		err := newTestS3Store(f, tc.keep).KeepHistory(context.Background(), "p", "")
		if tc.wantInErr != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantInErr) {
				t.Errorf("%s: err = %v, want one mentioning %q", tc.name, err, tc.wantInErr)
			}
		} else if err != nil {
			t.Fatalf("%s: KeepHistory: %v", tc.name, err)
		}
		if tc.wantDst != nil {
			got := map[string]string{}
			for _, k := range f.keys(dst) {
				got[k], _ = f.get(k)
			}
			if !reflect.DeepEqual(got, tc.wantDst) {
				t.Errorf("%s: results/history = %v, want %v", tc.name, got, tc.wantDst)
			}
		}
		if f.count("Copy") != tc.wantCopy || f.count("Get") != 0 {
			t.Errorf("%s: calls %v, want %d copies and no downloads", tc.name, f.calls, tc.wantCopy)
		}
	}
}

// TestS3Store_PrepareLocal downloads results and the latest history into a
// temp project dir; a failing history download is not fatal.
func TestS3Store_PrepareLocal(t *testing.T) {
	t.Parallel()
	objects := map[string]string{
		"projects/p/results/result.json":                 "result-data",
		"projects/p/reports/latest/history/history.json": "history-data",
	}
	for _, tc := range []struct {
		name   string
		failOn map[string]error
		want   map[string]string
	}{
		{"results and history", nil, map[string]string{"results/result.json": "result-data", "results/history/history.json": "history-data"}},
		{"history failure is not fatal", map[string]error{"List projects/p/reports/latest/history/": errBoom}, map[string]string{"results/result.json": "result-data"}},
	} {
		f := newFakeS3(objects)
		f.failOn = tc.failOn
		st := newTestS3Store(f, true)
		dir, err := st.PrepareLocal(context.Background(), "p", nil)
		if err != nil {
			t.Fatalf("%s: PrepareLocal: %v", tc.name, err)
		}
		if got := readTree(t, dir); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: local files = %v, want %v", tc.name, got, tc.want)
		}
		if err := st.CleanupLocal(dir); err != nil {
			t.Errorf("%s: CleanupLocal: %v", tc.name, err)
		}
	}
}

// TestS3Store_Playwright keys Playwright reports under
// projects/<id>/playwright-reports/: upload to latest, snapshot latest into a
// numbered build server-side, read it back, and clean latest.
func TestS3Store_Playwright(t *testing.T) {
	t.Parallel()
	f := newFakeS3(nil)
	st := newTestS3Store(f, true)
	ctx := context.Background()
	const prefix = "projects/proj1/playwright-reports/"

	if err := st.CopyPlaywrightLatestToBuild(ctx, "proj1", 5); err != nil {
		t.Errorf("CopyPlaywrightLatestToBuild without latest: %v", err)
	}
	if ok, err := st.PlaywrightReportExists(ctx, "proj1", 7); err != nil || ok {
		t.Errorf("PlaywrightReportExists without a report = %v, %v", ok, err)
	}
	for _, sub := range []string{"latest/index.html", "latest/data/a.json", "latest/data/b.json"} {
		if err := st.WritePlaywrightFile(ctx, "proj1", sub, strings.NewReader(sub)); err != nil {
			t.Fatalf("WritePlaywrightFile(%s): %v", sub, err)
		}
	}
	if err := st.CopyPlaywrightLatestToBuild(ctx, "proj1", 7); err != nil {
		t.Fatalf("CopyPlaywrightLatestToBuild: %v", err)
	}
	if ok, err := st.PlaywrightReportExists(ctx, "proj1", 7); err != nil || !ok {
		t.Errorf("PlaywrightReportExists after the copy = %v, %v", ok, err)
	}
	files, err := st.ListPlaywrightDataFiles(ctx, "proj1", 7)
	sort.Strings(files)
	if err != nil || !slices.Equal(files, []string{"a.json", "b.json"}) {
		t.Errorf("ListPlaywrightDataFiles = %v, %v", files, err)
	}
	rc, contentType, err := st.ReadPlaywrightFile(ctx, "proj1", "7/index.html")
	if err != nil {
		t.Fatalf("ReadPlaywrightFile: %v", err)
	}
	data, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(data) != "latest/index.html" || !strings.HasPrefix(contentType, "text/html") {
		t.Errorf("ReadPlaywrightFile = %q (%s)", data, contentType)
	}

	if err := st.CleanPlaywrightLatest(ctx, "proj1"); err != nil {
		t.Fatalf("CleanPlaywrightLatest: %v", err)
	}
	want := []string{prefix + "7/data/a.json", prefix + "7/data/b.json", prefix + "7/index.html"}
	if got := f.keys(""); !slices.Equal(got, want) {
		t.Errorf("keys after cleaning latest = %v, want %v", got, want)
	}
}
