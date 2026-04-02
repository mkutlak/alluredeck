package parser_test

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/parser"
)

// buildPlaywrightHTML wraps a base64-encoded ZIP in minimal HTML that matches
// the Playwright report format.
func buildPlaywrightHTML(encoded string) []byte {
	var buf bytes.Buffer
	buf.WriteString(`<html><head></head><body><script>`)
	buf.WriteString(`window.playwrightReportBase64 = "data:application/zip;base64,`)
	buf.WriteString(encoded)
	buf.WriteString(`";</script></body></html>`)
	return buf.Bytes()
}

// buildZip creates an in-memory ZIP archive from a map of filename → content.
func buildZip(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, data := range files {
		f, err := w.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(data); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// --- TestExtractPlaywrightData ---

func TestExtractPlaywrightData(t *testing.T) {
	t.Parallel()

	reportContent := []byte(`{"startTime":1700000000000,"duration":5000,"files":[],"stats":{"total":0}}`)
	detailContent := []byte(`{"fileId":"abc123","fileName":"tests/foo.spec.ts","tests":[]}`)

	zipBytes, err := buildZip(map[string][]byte{
		"report.json": reportContent,
		"abc123.json": detailContent,
	})
	if err != nil {
		t.Fatalf("buildZip: %v", err)
	}

	encoded := base64.StdEncoding.EncodeToString(zipBytes)
	html := buildPlaywrightHTML(encoded)

	reportJSON, fileJSONs, err := parser.ExtractPlaywrightData(bytes.NewReader(html))
	if err != nil {
		t.Fatalf("ExtractPlaywrightData returned unexpected error: %v", err)
	}

	if !bytes.Equal(reportJSON, reportContent) {
		t.Errorf("reportJSON mismatch: got %q, want %q", reportJSON, reportContent)
	}
	if len(fileJSONs) != 1 {
		t.Errorf("fileJSONs: got %d entries, want 1", len(fileJSONs))
	}
	if got, ok := fileJSONs["abc123.json"]; !ok {
		t.Error("fileJSONs: missing key abc123.json")
	} else if !bytes.Equal(got, detailContent) {
		t.Errorf("fileJSONs[abc123.json] mismatch: got %q, want %q", got, detailContent)
	}
}

func TestExtractPlaywrightData_MissingMarker(t *testing.T) {
	t.Parallel()

	html := []byte(`<html><body>no playwright data here</body></html>`)
	_, _, err := parser.ExtractPlaywrightData(bytes.NewReader(html))
	if err == nil {
		t.Fatal("expected error for missing marker, got nil")
	}
}

func TestExtractPlaywrightData_InvalidBase64(t *testing.T) {
	t.Parallel()

	html := buildPlaywrightHTML("!!!not-valid-base64!!!")
	_, _, err := parser.ExtractPlaywrightData(bytes.NewReader(html))
	if err == nil {
		t.Fatal("expected error for invalid base64, got nil")
	}
}

// --- TestParsePlaywrightReport ---

func TestParsePlaywrightReport(t *testing.T) {
	t.Parallel()

	// Build report.json with 2 files and 3 tests.
	reportData := map[string]any{
		"metadata": map[string]any{
			"ci": map[string]any{
				"commitHash": "ci-sha-abc",
				"buildHref":  "https://ci.example.com/build/42",
				"branch":     "main",
			},
			"gitCommit": map[string]any{
				"hash":   "git-sha-xyz",
				"branch": "feature/foo",
			},
		},
		"startTime": float64(1700000000000),
		"duration":  float64(9000),
		"stats": map[string]any{
			"total":      3,
			"expected":   1,
			"unexpected": 1,
			"flaky":      0,
			"skipped":    1,
			"ok":         false,
		},
		"files": []any{
			map[string]any{
				"fileId":   "file1",
				"fileName": "tests/login.spec.ts",
				"tests": []any{
					map[string]any{
						"testId":      "t1",
						"title":       "should pass",
						"projectName": "chromium",
						"outcome":     "expected",
						"path":        []any{"Login"},
						"duration":    float64(1000),
						"tags":        []any{"@smoke"},
						"ok":          true,
						"results": []any{
							map[string]any{
								"startTime":   "2023-11-14T12:00:00Z",
								"duration":    float64(1000),
								"retry":       0,
								"steps":       []any{},
								"errors":      []any{},
								"status":      "passed",
								"attachments": []any{},
							},
						},
					},
					map[string]any{
						"testId":      "t2",
						"title":       "should skip",
						"projectName": "chromium",
						"outcome":     "skipped",
						"path":        []any{"Login"},
						"duration":    float64(0),
						"tags":        []any{},
						"ok":          false,
						"results":     []any{},
					},
				},
			},
			map[string]any{
				"fileId":   "file2",
				"fileName": "tests/checkout.spec.ts",
				"tests": []any{
					map[string]any{
						"testId":      "t3",
						"title":       "should fail",
						"projectName": "firefox",
						"outcome":     "unexpected",
						"path":        []any{"Checkout", "Payment"},
						"duration":    float64(3000),
						"tags":        []any{"@regression", "@payments"},
						"ok":          false,
						"results":     []any{},
					},
				},
			},
		},
	}
	reportJSON, err := json.Marshal(reportData)
	if err != nil {
		t.Fatalf("marshal reportData: %v", err)
	}

	// Build detail JSON for file2 (the failing test), with steps, errors, attachments.
	detailData := map[string]any{
		"fileId":   "file2",
		"fileName": "tests/checkout.spec.ts",
		"tests": []any{
			map[string]any{
				"testId":      "t3",
				"title":       "should fail",
				"projectName": "firefox",
				"outcome":     "unexpected",
				"path":        []any{"Checkout", "Payment"},
				"duration":    float64(3000),
				"tags":        []any{"@regression", "@payments"},
				"ok":          false,
				"results": []any{
					map[string]any{
						"startTime": "2023-11-14T12:01:00Z",
						"duration":  float64(3000),
						"retry":     0,
						"status":    "failed",
						"errors":    []any{"Expected 200 but got 500"},
						"attachments": []any{
							map[string]any{
								"name":        "screenshot",
								"contentType": "image/png",
								"path":        "data/abc123.png",
							},
						},
						"steps": []any{
							map[string]any{
								"title":    "Navigate to checkout",
								"duration": float64(500),
								"steps": []any{
									map[string]any{
										"title":       "Click pay button",
										"duration":    float64(200),
										"steps":       []any{},
										"attachments": []any{},
										"error": map[string]any{
											"message": "click failed",
											"stack":   "at line 42",
										},
									},
								},
								"attachments": []any{},
							},
						},
					},
				},
			},
		},
	}
	detailJSON, err := json.Marshal(detailData)
	if err != nil {
		t.Fatalf("marshal detailData: %v", err)
	}

	fileJSONs := map[string][]byte{
		"file2.json": detailJSON,
	}

	results, meta, err := parser.ParsePlaywrightReport(reportJSON, fileJSONs)
	if err != nil {
		t.Fatalf("ParsePlaywrightReport returned unexpected error: %v", err)
	}

	// --- Result count ---
	if len(results) != 3 {
		t.Fatalf("results count: got %d, want 3", len(results))
	}

	// --- Status mapping ---
	byID := make(map[string]*parser.Result, len(results))
	for _, r := range results {
		byID[r.HistoryID] = r
	}

	t1 := byID["t1"]
	if t1 == nil {
		t.Fatal("result for testId t1 not found")
	}
	if t1.Status != "passed" {
		t.Errorf("t1 status: got %q, want %q", t1.Status, "passed")
	}

	t2 := byID["t2"]
	if t2 == nil {
		t.Fatal("result for testId t2 not found")
	}
	if t2.Status != "skipped" {
		t.Errorf("t2 status: got %q, want %q", t2.Status, "skipped")
	}

	t3 := byID["t3"]
	if t3 == nil {
		t.Fatal("result for testId t3 not found")
	}
	if t3.Status != "failed" {
		t.Errorf("t3 status: got %q, want %q", t3.Status, "failed")
	}

	// --- Name/FullName construction ---
	if t1.Name != "Login > should pass" {
		t.Errorf("t1.Name: got %q, want %q", t1.Name, "Login > should pass")
	}
	if t1.FullName != "tests/login.spec.ts > Login > should pass" {
		t.Errorf("t1.FullName: got %q, want %q", t1.FullName, "tests/login.spec.ts > Login > should pass")
	}
	if t3.Name != "Checkout > Payment > should fail" {
		t.Errorf("t3.Name: got %q, want %q", t3.Name, "Checkout > Payment > should fail")
	}

	// --- HistoryID ---
	if t1.HistoryID != "t1" {
		t.Errorf("t1.HistoryID: got %q, want %q", t1.HistoryID, "t1")
	}

	// --- Labels ---
	findLabel := func(labels []parser.Label, name, value string) bool {
		for _, l := range labels {
			if l.Name == name && l.Value == value {
				return true
			}
		}
		return false
	}

	if !findLabel(t1.Labels, "tag", "smoke") {
		t.Error("t1 labels: missing tag=smoke (@ should be stripped)")
	}
	if !findLabel(t1.Labels, "suite", "tests/login.spec.ts") {
		t.Error("t1 labels: missing suite=tests/login.spec.ts")
	}
	if !findLabel(t1.Labels, "parentSuite", "chromium") {
		t.Error("t1 labels: missing parentSuite=chromium")
	}
	if !findLabel(t1.Labels, "framework", "playwright") {
		t.Error("t1 labels: missing framework=playwright")
	}

	if !findLabel(t3.Labels, "tag", "regression") {
		t.Error("t3 labels: missing tag=regression")
	}
	if !findLabel(t3.Labels, "tag", "payments") {
		t.Error("t3 labels: missing tag=payments")
	}

	// --- Steps (t3 uses detail JSON) ---
	if len(t3.Steps) != 1 {
		t.Fatalf("t3 steps count: got %d, want 1", len(t3.Steps))
	}
	if t3.Steps[0].Name != "Navigate to checkout" {
		t.Errorf("t3 steps[0].Name: got %q, want %q", t3.Steps[0].Name, "Navigate to checkout")
	}
	if len(t3.Steps[0].Steps) != 1 {
		t.Fatalf("t3 steps[0].Steps count: got %d, want 1", len(t3.Steps[0].Steps))
	}
	nestedStep := t3.Steps[0].Steps[0]
	if nestedStep.Name != "Click pay button" {
		t.Errorf("nested step Name: got %q, want %q", nestedStep.Name, "Click pay button")
	}
	if nestedStep.Status != "failed" {
		t.Errorf("nested step Status: got %q, want %q", nestedStep.Status, "failed")
	}
	if nestedStep.StatusMessage != "click failed" {
		t.Errorf("nested step StatusMessage: got %q, want %q", nestedStep.StatusMessage, "click failed")
	}

	// --- Attachments (data/ prefix stripped) ---
	if len(t3.Attachments) != 1 {
		t.Fatalf("t3 attachments count: got %d, want 1", len(t3.Attachments))
	}
	if t3.Attachments[0].Source != "abc123.png" {
		t.Errorf("t3 attachment Source: got %q, want %q (data/ prefix should be stripped)", t3.Attachments[0].Source, "abc123.png")
	}
	if t3.Attachments[0].MimeType != "image/png" {
		t.Errorf("t3 attachment MimeType: got %q, want %q", t3.Attachments[0].MimeType, "image/png")
	}

	// --- Timing ---
	if t1.StartMs == 0 {
		t.Error("t1.StartMs: expected non-zero")
	}
	if t1.DurationMs != 1000 {
		t.Errorf("t1.DurationMs: got %d, want 1000", t1.DurationMs)
	}
	if t1.StopMs != t1.StartMs+t1.DurationMs {
		t.Errorf("t1.StopMs: got %d, want StartMs+DurationMs=%d", t1.StopMs, t1.StartMs+t1.DurationMs)
	}

	// --- PlaywrightMeta ---
	if meta == nil {
		t.Fatal("meta is nil")
	}
	// gitCommit takes precedence over CI for CommitSHA.
	if meta.CommitSHA != "git-sha-xyz" {
		t.Errorf("meta.CommitSHA: got %q, want %q", meta.CommitSHA, "git-sha-xyz")
	}
	// gitCommit branch takes precedence.
	if meta.Branch != "feature/foo" {
		t.Errorf("meta.Branch: got %q, want %q", meta.Branch, "feature/foo")
	}
	if meta.BuildURL != "https://ci.example.com/build/42" {
		t.Errorf("meta.BuildURL: got %q, want %q", meta.BuildURL, "https://ci.example.com/build/42")
	}
	if meta.StartTime != 1700000000000 {
		t.Errorf("meta.StartTime: got %d, want 1700000000000", meta.StartTime)
	}
	if meta.Duration != 9000 {
		t.Errorf("meta.Duration: got %d, want 9000", meta.Duration)
	}
	if meta.Stats.Total != 3 {
		t.Errorf("meta.Stats.Total: got %d, want 3", meta.Stats.Total)
	}
	if meta.Stats.Expected != 1 {
		t.Errorf("meta.Stats.Expected: got %d, want 1", meta.Stats.Expected)
	}
	if meta.Stats.Unexpected != 1 {
		t.Errorf("meta.Stats.Unexpected: got %d, want 1", meta.Stats.Unexpected)
	}
	if meta.Stats.Skipped != 1 {
		t.Errorf("meta.Stats.Skipped: got %d, want 1", meta.Stats.Skipped)
	}

	// --- StatusMessage from errors ---
	if t3.StatusMessage != "Expected 200 but got 500" {
		t.Errorf("t3.StatusMessage: got %q, want %q", t3.StatusMessage, "Expected 200 but got 500")
	}
}

func TestParsePlaywrightReport_FallbackBranch(t *testing.T) {
	t.Parallel()

	// No gitCommit block; CI block provides branch + commitHash.
	reportData := map[string]any{
		"metadata": map[string]any{
			"ci": map[string]any{
				"commitHash": "ci-only-sha",
				"buildHref":  "https://ci.example.com/1",
				"branch":     "release/1.0",
			},
		},
		"startTime": float64(0),
		"duration":  float64(0),
		"stats":     map[string]any{},
		"files":     []any{},
	}
	reportJSON, _ := json.Marshal(reportData)

	_, meta, err := parser.ParsePlaywrightReport(reportJSON, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta.CommitSHA != "ci-only-sha" {
		t.Errorf("meta.CommitSHA: got %q, want %q", meta.CommitSHA, "ci-only-sha")
	}
	if meta.Branch != "release/1.0" {
		t.Errorf("meta.Branch: got %q, want %q", meta.Branch, "release/1.0")
	}
}

// --- TestExtractErrorString ---

// extractErrorStringViaReport exercises the error-extraction path by embedding
// the error inside a real ParsePlaywrightReport call, keeping the test within
// the parser_test package boundary (extractErrorString is unexported).
func TestExtractErrorString_ViaReport(t *testing.T) {
	t.Parallel()

	makeReport := func(errors []any) []byte {
		data := map[string]any{
			"startTime": float64(0),
			"duration":  float64(0),
			"stats":     map[string]any{},
			"files": []any{
				map[string]any{
					"fileId":   "f1",
					"fileName": "test.spec.ts",
					"tests": []any{
						map[string]any{
							"testId":      "tx",
							"title":       "err test",
							"projectName": "chromium",
							"outcome":     "unexpected",
							"path":        []any{},
							"duration":    float64(100),
							"tags":        []any{},
							"ok":          false,
							"results": []any{
								map[string]any{
									"startTime":   "2023-01-01T00:00:00Z",
									"duration":    float64(100),
									"retry":       0,
									"status":      "failed",
									"errors":      errors,
									"steps":       []any{},
									"attachments": []any{},
								},
							},
						},
					},
				},
			},
		}
		b, _ := json.Marshal(data)
		return b
	}

	tests := []struct {
		name    string
		errors  []any
		wantMsg string
	}{
		{
			name:    "string error",
			errors:  []any{"something went wrong"},
			wantMsg: "something went wrong",
		},
		{
			name:    "object error with message field",
			errors:  []any{map[string]any{"message": "object error msg", "stack": "at line 1"}},
			wantMsg: "object error msg",
		},
		{
			name:    "multiple errors joined",
			errors:  []any{"first error", "second error"},
			wantMsg: "first error", // StatusMessage is just the first
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reportJSON := makeReport(tc.errors)
			results, _, err := parser.ParsePlaywrightReport(reportJSON, nil)
			if err != nil {
				t.Fatalf("ParsePlaywrightReport error: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("results count: got %d, want 1", len(results))
			}
			if results[0].StatusMessage != tc.wantMsg {
				t.Errorf("StatusMessage: got %q, want %q", results[0].StatusMessage, tc.wantMsg)
			}
		})
	}
}

func TestExtractErrorString_MultipleErrorsTrace(t *testing.T) {
	t.Parallel()

	reportData := map[string]any{
		"startTime": float64(0),
		"duration":  float64(0),
		"stats":     map[string]any{},
		"files": []any{
			map[string]any{
				"fileId":   "f1",
				"fileName": "test.spec.ts",
				"tests": []any{
					map[string]any{
						"testId":      "tx",
						"title":       "multi-err test",
						"projectName": "chromium",
						"outcome":     "unexpected",
						"path":        []any{},
						"duration":    float64(100),
						"tags":        []any{},
						"ok":          false,
						"results": []any{
							map[string]any{
								"startTime":   "2023-01-01T00:00:00Z",
								"duration":    float64(100),
								"retry":       0,
								"status":      "failed",
								"errors":      []any{"error one", "error two"},
								"steps":       []any{},
								"attachments": []any{},
							},
						},
					},
				},
			},
		},
	}
	reportJSON, _ := json.Marshal(reportData)

	results, _, err := parser.ParsePlaywrightReport(reportJSON, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results count: got %d, want 1", len(results))
	}
	wantTrace := "error one\nerror two"
	if results[0].StatusTrace != wantTrace {
		t.Errorf("StatusTrace: got %q, want %q", results[0].StatusTrace, wantTrace)
	}
}
