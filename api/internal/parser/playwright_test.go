package parser_test

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"maps"
	"reflect"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/parser"
)

// zipBase64 builds an in-memory ZIP of files and returns it base64-encoded.
func zipBase64(t *testing.T, files map[string][]byte) string {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, data := range files {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// pwRun is one attempt entry (test.results[i]) of a Playwright test.
func pwRun(status string, retry int, errs ...any) map[string]any {
	return map[string]any{"startTime": "2023-11-14T12:00:00Z", "duration": 1000, "retry": retry, "status": status,
		"errors": append([]any{}, errs...), "steps": []any{}, "attachments": []any{}}
}

// pwTest is one Playwright test entry; path is its describe() chain.
func pwTest(id, title, project, outcome string, path, tags []string, runs ...map[string]any) map[string]any {
	return map[string]any{"testId": id, "title": title, "projectName": project, "outcome": outcome, "path": path,
		"tags": tags, "duration": 1000, "ok": outcome != "unexpected", "results": append([]map[string]any{}, runs...)}
}

func pwFile(id, name string, tests ...map[string]any) map[string]any {
	return map[string]any{"fileId": id, "fileName": name, "tests": tests}
}

// pwReport marshals a report.json with fixed timing and stats.
func pwReport(t *testing.T, metadata map[string]any, files ...map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"metadata": metadata, "startTime": 1700000000000, "duration": 9000, "files": files,
		"stats": map[string]any{"total": 4, "expected": 1, "unexpected": 1, "flaky": 1, "skipped": 1, "ok": false},
	})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// TestExtractPlaywrightData locates the embedded base64 ZIP in both HTML
// report shells — the legacy script variable and the v1.59+ <template>
// element — and returns report.json plus every other JSON file.
func TestExtractPlaywrightData(t *testing.T) {
	t.Parallel()
	report := []byte(`{"startTime":1700000000000,"duration":5000,"files":[],"stats":{"total":0}}`)
	detail := []byte(`{"fileId":"abc123","fileName":"tests/foo.spec.ts","tests":[]}`)
	script := func(enc string) []byte {
		return []byte(`<html><head></head><body><script>window.playwrightReportBase64 = "data:application/zip;base64,` + enc + `";</script></body></html>`)
	}
	template := func(enc string) []byte {
		return []byte(`<html><head></head><body><template id="playwrightReportBase64">data:application/zip;base64,` + enc + `</template></body></html>`)
	}
	tests := []struct {
		name      string
		html      []byte
		wantFiles map[string][]byte // nil = extraction must fail
	}{
		{"legacy script variable", script(zipBase64(t, map[string][]byte{"report.json": report, "abc123.json": detail})), map[string][]byte{"abc123.json": detail}},
		{"v1.59+ template element", template(zipBase64(t, map[string][]byte{"report.json": report})), map[string][]byte{}},
		{"missing marker", []byte(`<html><body>no playwright data here</body></html>`), nil},
		{"invalid base64", script("!!!not-valid-base64!!!"), nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gotReport, gotFiles, err := parser.ExtractPlaywrightData(bytes.NewReader(tc.html))
			if (err != nil) != (tc.wantFiles == nil) {
				t.Fatalf("ExtractPlaywrightData error = %v, want error: %v", err, tc.wantFiles == nil)
			}
			if tc.wantFiles == nil {
				return
			}
			if !bytes.Equal(gotReport, report) {
				t.Errorf("reportJSON = %q, want %q", gotReport, report)
			}
			if !maps.EqualFunc(gotFiles, tc.wantFiles, bytes.Equal) {
				t.Errorf("fileJSONs = %q, want %q", gotFiles, tc.wantFiles)
			}
		})
	}
}

// TestParsePlaywrightReport maps report.json (plus per-file detail JSON, which
// replaces the summary tests when present) onto results: outcome → status,
// describe path → Name/FullName, testId → HistoryID, tags (sans @) and
// suite/project/framework labels, steps with their errors, attachments with the
// data/ prefix stripped, timing from the last attempt, and flaky outcomes
// surfaced as Flaky with Retries from the last attempt while still "passed".
func TestParsePlaywrightReport(t *testing.T) {
	t.Parallel()
	failing := pwRun("failed", 0, "Expected 200 but got 500")
	failing["attachments"] = []any{map[string]any{"name": "screenshot", "contentType": "image/png", "path": "data/abc123.png"}}
	failing["steps"] = []any{map[string]any{"title": "Navigate to checkout", "duration": 500, "attachments": []any{},
		"steps": []any{map[string]any{"title": "Click pay button", "duration": 200, "steps": []any{}, "attachments": []any{},
			"error": map[string]any{"message": "click failed", "stack": "at line 42"}}}}}
	checkout := []string{"Checkout", "Payment"}
	reportJSON := pwReport(t, nil,
		pwFile("file1", "tests/login.spec.ts",
			pwTest("t1", "should pass", "chromium", "expected", []string{"Login"}, []string{"@smoke"}, pwRun("passed", 0)),
			pwTest("t2", "should skip", "chromium", "skipped", []string{"Login"}, nil),
			pwTest("t4", "should eventually pass", "chromium", "flaky", []string{"Flaky"}, nil,
				pwRun("failed", 0, "flaky failure on attempt 1"), pwRun("passed", 1))),
		pwFile("file2", "tests/checkout.spec.ts", pwTest("t3", "should fail", "firefox", "unexpected", checkout, []string{"@regression", "@payments"})),
	)
	detail, _ := json.Marshal(pwFile("file2", "tests/checkout.spec.ts",
		pwTest("t3", "should fail", "firefox", "unexpected", checkout, []string{"@regression", "@payments"}, failing)))

	results, _, err := parser.ParsePlaywrightReport(reportJSON, map[string][]byte{"file2.json": detail})
	if err != nil {
		t.Fatalf("ParsePlaywrightReport: %v", err)
	}
	byID := map[string]*parser.Result{}
	for _, r := range results {
		byID[r.HistoryID] = r
	}
	type identity struct {
		Name, FullName, Status string
		Flaky                  bool
		Retries                int
	}
	wantIdentity := map[string]identity{
		"t1": {"Login > should pass", "tests/login.spec.ts > Login > should pass", "passed", false, 0},
		"t2": {"Login > should skip", "tests/login.spec.ts > Login > should skip", "skipped", false, 0},
		"t3": {"Checkout > Payment > should fail", "tests/checkout.spec.ts > Checkout > Payment > should fail", "failed", false, 0},
		"t4": {"Flaky > should eventually pass", "tests/login.spec.ts > Flaky > should eventually pass", "passed", true, 1},
	}
	if len(results) != len(wantIdentity) {
		t.Fatalf("results count: got %d, want %d", len(results), len(wantIdentity))
	}
	for id, want := range wantIdentity {
		r := byID[id]
		if r == nil {
			t.Fatalf("result for testId %s not found", id)
		}
		if got := (identity{r.Name, r.FullName, r.Status, r.Flaky, r.Retries}); got != want {
			t.Errorf("%s: got %+v, want %+v", id, got, want)
		}
	}

	t1, t3 := byID["t1"], byID["t3"]
	label := func(name, value string) parser.Label { return parser.Label{Name: name, Value: value} }
	if want := []parser.Label{label("tag", "smoke"), label("suite", "tests/login.spec.ts"), label("parentSuite", "chromium"), label("framework", "playwright")}; !reflect.DeepEqual(t1.Labels, want) {
		t.Errorf("t1 labels = %+v, want %+v", t1.Labels, want)
	}
	if want := []parser.Label{label("tag", "regression"), label("tag", "payments"), label("suite", "tests/checkout.spec.ts"), label("parentSuite", "firefox"), label("framework", "playwright")}; !reflect.DeepEqual(t3.Labels, want) {
		t.Errorf("t3 labels = %+v, want %+v", t3.Labels, want)
	}
	if t3.StatusMessage != "Expected 200 but got 500" {
		t.Errorf("t3.StatusMessage = %q, want %q", t3.StatusMessage, "Expected 200 but got 500")
	}
	if len(t3.Steps) != 1 || t3.Steps[0].Name != "Navigate to checkout" || len(t3.Steps[0].Steps) != 1 {
		t.Fatalf("t3 steps = %+v, want Navigate to checkout > Click pay button", t3.Steps)
	}
	if nested := t3.Steps[0].Steps[0]; nested.Name != "Click pay button" || nested.Status != "failed" || nested.StatusMessage != "click failed" {
		t.Errorf("nested step = %+v, want failed Click pay button with message %q", nested, "click failed")
	}
	if want := []parser.Attachment{{Name: "screenshot", Source: "abc123.png", MimeType: "image/png"}}; !reflect.DeepEqual(t3.Attachments, want) {
		t.Errorf("t3 attachments = %+v, want %+v", t3.Attachments, want)
	}
	if t1.StartMs != 1699963200000 || t1.DurationMs != 1000 || t1.StopMs != t1.StartMs+t1.DurationMs {
		t.Errorf("t1 timing = start %d, duration %d, stop %d; want 1699963200000, 1000, start+duration", t1.StartMs, t1.DurationMs, t1.StopMs)
	}
}

// TestParsePlaywrightReport_Meta verifies report-level metadata: gitCommit
// takes precedence over the ci block for commit and branch, ci fills them in
// when gitCommit is absent, and timing/stats come from the report.
func TestParsePlaywrightReport_Meta(t *testing.T) {
	t.Parallel()
	ci := map[string]any{"commitHash": "ci-sha-abc", "buildHref": "https://ci.example.com/build/42", "branch": "main"}
	stats := parser.PlaywrightStats{Total: 4, Expected: 1, Unexpected: 1, Flaky: 1, Skipped: 1}
	tests := []struct {
		name     string
		metadata map[string]any
		want     parser.PlaywrightMeta
	}{
		{
			name:     "gitCommit wins over ci",
			metadata: map[string]any{"ci": ci, "gitCommit": map[string]any{"hash": "git-sha-xyz", "branch": "feature/foo"}},
			want:     parser.PlaywrightMeta{Branch: "feature/foo", CommitSHA: "git-sha-xyz", BuildURL: "https://ci.example.com/build/42", StartTime: 1700000000000, Duration: 9000, Stats: stats},
		},
		{
			name:     "ci fills in without gitCommit",
			metadata: map[string]any{"ci": ci},
			want:     parser.PlaywrightMeta{Branch: "main", CommitSHA: "ci-sha-abc", BuildURL: "https://ci.example.com/build/42", StartTime: 1700000000000, Duration: 9000, Stats: stats},
		},
	}
	for _, tc := range tests {
		_, meta, err := parser.ParsePlaywrightReport(pwReport(t, tc.metadata), nil)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if meta == nil || *meta != tc.want {
			t.Errorf("%s: meta = %+v, want %+v", tc.name, meta, tc.want)
		}
	}
}

// TestParsePlaywrightReport_Attempts pins error extraction and per-attempt
// capture. Errors may be strings or {message} objects; the last attempt's
// first error is the status message and all of them form the trace. EVERY
// entry of test.results is recorded as an attempt, not just the last one —
// the last-attempt-only shape is what left triage's retry-consistency signal
// permanently reporting "single".
func TestParsePlaywrightReport_Attempts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name               string
		runs               []map[string]any
		wantMsg, wantTrace string
		want               []parser.Attempt
	}{
		{
			name: "string error", runs: []map[string]any{pwRun("failed", 0, "something went wrong")},
			wantMsg: "something went wrong", wantTrace: "something went wrong",
			want: []parser.Attempt{{Index: 0, Status: "failed", StatusMessage: "something went wrong"}},
		},
		{
			name: "object error uses its message", runs: []map[string]any{pwRun("failed", 0, map[string]any{"message": "object error msg", "stack": "at line 1"})},
			wantMsg: "object error msg", wantTrace: "object error msg",
			want: []parser.Attempt{{Index: 0, Status: "failed", StatusMessage: "object error msg"}},
		},
		{
			name: "multiple errors on one attempt are joined", runs: []map[string]any{pwRun("failed", 0, "error one", map[string]any{"message": "error two"})},
			wantMsg: "error one", wantTrace: "error one\nerror two",
			want: []parser.Attempt{{Index: 0, Status: "failed", StatusMessage: "error one\nerror two"}},
		},
		{
			name:    "two attempts, different errors",
			runs:    []map[string]any{pwRun("failed", 0, "expected 200, got 500"), pwRun("timedOut", 1, "Test timeout of 30000ms exceeded")},
			wantMsg: "Test timeout of 30000ms exceeded", wantTrace: "Test timeout of 30000ms exceeded",
			want: []parser.Attempt{
				{Index: 0, Status: "failed", StatusMessage: "expected 200, got 500"},
				{Index: 1, Status: "timedOut", StatusMessage: "Test timeout of 30000ms exceeded"},
			},
		},
		{
			name: "a passing retry carries an empty message", runs: []map[string]any{pwRun("failed", 0, "boom"), pwRun("passed", 1)},
			want: []parser.Attempt{{Index: 0, Status: "failed", StatusMessage: "boom"}, {Index: 1, Status: "passed", StatusMessage: ""}},
		},
		{name: "no results yields no attempts"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			report := pwReport(t, nil, pwFile("f1", "tests/retry.spec.ts",
				pwTest("t-retry", "retried test", "chromium", "unexpected", []string{"Retry"}, nil, tc.runs...)))
			results, _, err := parser.ParsePlaywrightReport(report, nil)
			if err != nil {
				t.Fatalf("ParsePlaywrightReport: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("results count: got %d, want 1", len(results))
			}
			r := results[0]
			if r.StatusMessage != tc.wantMsg || r.StatusTrace != tc.wantTrace {
				t.Errorf("status = (%q, %q), want (%q, %q)", r.StatusMessage, r.StatusTrace, tc.wantMsg, tc.wantTrace)
			}
			if len(r.Attempts) != len(tc.want) || (len(tc.want) > 0 && !reflect.DeepEqual(r.Attempts, tc.want)) {
				t.Errorf("attempts = %+v, want %+v", r.Attempts, tc.want)
			}
		})
	}
}
