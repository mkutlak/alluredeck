package parser_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/parser"
)

// TestParseFile_Fixtures parses the real Allure 2 and Allure 3 result files in
// testdata/. Allure 2 nests timing in a "time" object; Allure 3 puts it at the
// top level. A missing file is an error, not an empty result.
func TestParseFile_Fixtures(t *testing.T) {
	t.Parallel()
	type summary struct {
		name, fullName, status, message string
		hasTrace, hasDescription        bool
		start, stop, duration           int64
		labels, params                  []string // names
		steps                           []string // status/sub-steps/attachments per top-level step
		attachments                     int
	}
	summarize := func(r *parser.Result) summary {
		s := summary{name: r.Name, fullName: r.FullName, status: r.Status, message: r.StatusMessage,
			hasTrace: r.StatusTrace != "", hasDescription: r.Description != "",
			start: r.StartMs, stop: r.StopMs, duration: r.DurationMs, attachments: len(r.Attachments)}
		for _, l := range r.Labels {
			s.labels = append(s.labels, l.Name)
		}
		for _, p := range r.Parameters {
			s.params = append(s.params, p.Name)
		}
		for _, st := range r.Steps {
			s.steps = append(s.steps, fmt.Sprintf("%s/%d/%d", st.Status, len(st.Steps), len(st.Attachments)))
		}
		return s
	}
	tests := []struct {
		file string
		want *summary // nil = ParseFile must fail
	}{
		{"allure2-result.json", &summary{
			name: "loginWithInvalidCredentialsShouldFail", fullName: "com.example.auth.LoginTest.loginWithInvalidCredentialsShouldFail",
			status: "failed", message: "Expected status 401 but got 200", hasTrace: true, hasDescription: true,
			start: 1709000000000, stop: 1709000005000, duration: 5000,
			labels: []string{"suite", "severity"}, params: []string{"browser"}, steps: []string{"passed/1/1", "failed/0/0"}, attachments: 1,
		}},
		{"allure3-result.json", &summary{
			name: "userProfileLoadsSucessfully", fullName: "com.example.profile.ProfileTest.userProfileLoadsSucessfully",
			status: "passed", hasDescription: true, start: 1709000100000, stop: 1709000101000, duration: 1000,
			labels: []string{"feature", "owner", "severity"}, steps: []string{"passed/0/0"},
		}},
		{"does-not-exist-result.json", nil},
	}
	for _, tc := range tests {
		t.Run(tc.file, func(t *testing.T) {
			t.Parallel()
			r, err := parser.ParseFile(filepath.Join("testdata", tc.file))
			if tc.want == nil {
				if !errors.Is(err, fs.ErrNotExist) {
					t.Fatalf("ParseFile error = %v, want one wrapping fs.ErrNotExist", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			if got := summarize(r); !reflect.DeepEqual(got, *tc.want) {
				t.Errorf("ParseFile(%s)\n  got:  %+v\n  want: %+v", tc.file, got, *tc.want)
			}
		})
	}
}

// TestParseDir verifies only *-result.json files are parsed.
func TestParseDir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		files []string
		want  int
	}{
		{[]string{"aaa-result.json", "bbb-result.json", "executor.json"}, 2},
		{nil, 0},
	}
	for _, tc := range tests {
		dir := t.TempDir()
		for _, name := range tc.files {
			mustWrite(t, filepath.Join(dir, name), []byte(`{"name":"`+name+`","status":"passed"}`))
		}
		results, err := parser.ParseDir(dir)
		if err != nil || len(results) != tc.want {
			t.Errorf("ParseDir(%v) = %d results, err %v; want %d", tc.files, len(results), err, tc.want)
		}
	}
}

// TestResolveAttachments verifies attachment sources and sizes are resolved
// from the generated report: Allure 3 renames attachments to content hashes,
// recorded in data/test-results/*.json (originalFileName → id+ext,
// contentLength); without a mapping (Allure 2, which keeps names) the size is
// stat-ed from data/attachments/. Step and nested-step attachments resolve the
// same way, and a missing file leaves the size at 0.
func TestResolveAttachments(t *testing.T) {
	t.Parallel()
	generated := `{"name":"test1","attachments":[
		{"link":{"id":"abc123hash","originalFileName":"screenshot-001.png","ext":".png","contentType":"image/png","contentLength":4096},"type":"attachment"},
		{"link":{"id":"hashsteplog","originalFileName":"step-log.txt","ext":".txt","contentType":"text/plain","contentLength":19},"type":"attachment"}]}`
	statted := []byte("hello, this is a 42-byte attachment file!!")

	type resolved struct {
		Source string
		Size   int64
	}
	tests := []struct {
		name    string
		mapping bool
		want    []resolved // top-level ×3, step, nested step
	}{
		{name: "generated mapping (Allure 3)", mapping: true, want: []resolved{
			{"abc123hash.png", 4096}, {"abc-screenshot.png", 42}, {"no-such-file.png", 0}, {"hashsteplog.txt", 19}, {"nonexistent.txt", 0},
		}},
		{name: "no test-results dir (Allure 2)", want: []resolved{
			{"screenshot-001.png", 0}, {"abc-screenshot.png", 42}, {"no-such-file.png", 0}, {"step-log.txt", 0}, {"nonexistent.txt", 0},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			if tc.mapping {
				mustWrite(t, filepath.Join(dir, "test-results", "aaa-result.json"), []byte(generated))
			}
			mustWrite(t, filepath.Join(dir, "attachments", "abc-screenshot.png"), statted)

			att := func(src string) []parser.Attachment { return []parser.Attachment{{Name: src, Source: src}} }
			r := &parser.Result{
				Attachments: append(append(att("screenshot-001.png"), att("abc-screenshot.png")...), att("no-such-file.png")...),
				Steps:       []parser.Step{{Attachments: att("step-log.txt"), Steps: []parser.Step{{Attachments: att("nonexistent.txt")}}}},
			}
			parser.ResolveAttachments([]*parser.Result{r}, dir)

			var got []resolved
			for _, a := range append(append(r.Attachments, r.Steps[0].Attachments...), r.Steps[0].Steps[0].Attachments...) {
				got = append(got, resolved{a.Source, a.Size})
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("resolved = %+v, want %+v", got, tc.want)
			}
		})
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestParseFile_DeriveStatusFromFailedStep verifies the Allure 3 ("awesome")
// derivation: when the test-level statusDetails is empty, ParseFile adopts the
// deepest failed (fallback broken) step's message/trace as the result's status;
// an explicit test-level message is never overridden.
func TestParseFile_DeriveStatusFromFailedStep(t *testing.T) {
	t.Parallel()
	step := func(name, status, msg, trace string, children ...map[string]any) map[string]any {
		s := map[string]any{"name": name, "status": status, "steps": children}
		if msg != "" || trace != "" {
			s["statusDetails"] = map[string]any{"message": msg, "trace": trace}
		}
		return s
	}
	tests := []struct {
		name, testMsg      string
		steps              []map[string]any
		wantMsg, wantTrace string
	}{
		{
			name:    "deepest failed step wins over shallower failed step",
			steps:   []map[string]any{step("outer", "failed", "outer failure", "", step("inner", "failed", "deep assertion failed", "at inner.go:42"))},
			wantMsg: "deep assertion failed", wantTrace: "at inner.go:42",
		},
		{name: "broken step used when no failed step exists", steps: []map[string]any{step("setup", "broken", "fixture exploded", "")}, wantMsg: "fixture exploded"},
		{
			name:    "failed step preferred over broken step",
			steps:   []map[string]any{step("broken-step", "broken", "broken msg", ""), step("failed-step", "failed", "failed msg", "")},
			wantMsg: "failed msg",
		},
		{name: "failed step with empty message falls back to step name", steps: []map[string]any{step("the failing step", "failed", "", "")}, wantMsg: "the failing step"},
		{name: "no failed/broken step leaves message empty", steps: []map[string]any{step("ok", "passed", "", "")}},
		{name: "test-level message is not overridden", testMsg: "explicit test message", steps: []map[string]any{step("step", "failed", "step message", "")}, wantMsg: "explicit test message"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			raw := map[string]any{"name": "", "status": "failed", "steps": tc.steps}
			if tc.testMsg != "" {
				raw["statusDetails"] = map[string]any{"message": tc.testMsg}
			}
			data, _ := json.Marshal(raw)
			path := filepath.Join(t.TempDir(), "x-result.json")
			mustWrite(t, path, data)

			result, err := parser.ParseFile(path)
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			if result.StatusMessage != tc.wantMsg || result.StatusTrace != tc.wantTrace {
				t.Errorf("status = (%q, %q), want (%q, %q)", result.StatusMessage, result.StatusTrace, tc.wantMsg, tc.wantTrace)
			}
		})
	}
}
