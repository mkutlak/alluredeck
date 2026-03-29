package parser_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/parser"
)

func testdataPath(t *testing.T, name string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot determine test file path")
	}
	return filepath.Join(filepath.Dir(file), "testdata", name)
}

func TestParseFile_Allure2_Failed(t *testing.T) {
	t.Parallel()
	path := testdataPath(t, "allure2-result.json")
	result, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile returned unexpected error: %v", err)
	}

	if result.Name != "loginWithInvalidCredentialsShouldFail" {
		t.Errorf("Name: got %q, want %q", result.Name, "loginWithInvalidCredentialsShouldFail")
	}
	if result.FullName != "com.example.auth.LoginTest.loginWithInvalidCredentialsShouldFail" {
		t.Errorf("FullName: got %q", result.FullName)
	}
	if result.Status != "failed" {
		t.Errorf("Status: got %q, want %q", result.Status, "failed")
	}
	if result.StatusMessage == "" {
		t.Error("StatusMessage: expected non-empty error message")
	}
	if result.StatusTrace == "" {
		t.Error("StatusTrace: expected non-empty stack trace")
	}
	if result.Description == "" {
		t.Error("Description: expected non-empty description")
	}

	if len(result.Labels) != 2 {
		t.Errorf("Labels: got %d, want 2", len(result.Labels))
	} else {
		hasSuite := false
		hasSeverity := false
		for _, l := range result.Labels {
			if l.Name == "suite" {
				hasSuite = true
			}
			if l.Name == "severity" {
				hasSeverity = true
			}
		}
		if !hasSuite {
			t.Error("Labels: missing label with Name=suite")
		}
		if !hasSeverity {
			t.Error("Labels: missing label with Name=severity")
		}
	}

	if len(result.Parameters) != 1 {
		t.Errorf("Parameters: got %d, want 1", len(result.Parameters))
	} else if result.Parameters[0].Name != "browser" {
		t.Errorf("Parameters[0].Name: got %q, want %q", result.Parameters[0].Name, "browser")
	}

	if len(result.Steps) != 2 {
		t.Errorf("Steps: got %d, want 2", len(result.Steps))
	} else {
		if result.Steps[0].Status != "passed" {
			t.Errorf("Steps[0].Status: got %q, want %q", result.Steps[0].Status, "passed")
		}
		if result.Steps[1].Status != "failed" {
			t.Errorf("Steps[1].Status: got %q, want %q", result.Steps[1].Status, "failed")
		}
		// Steps[0] has sub-steps or attachments
		if len(result.Steps[0].Steps) == 0 && len(result.Steps[0].Attachments) == 0 {
			t.Error("Steps[0]: expected at least 1 sub-step or 1 attachment")
		}
	}

	if result.DurationMs != 5000 {
		t.Errorf("DurationMs: got %d, want 5000", result.DurationMs)
	}
	if result.StartMs != 1709000000000 {
		t.Errorf("StartMs: got %d, want 1709000000000", result.StartMs)
	}
	if result.StopMs != 1709000005000 {
		t.Errorf("StopMs: got %d, want 1709000005000", result.StopMs)
	}

	if len(result.Attachments) != 1 {
		t.Errorf("Attachments: got %d, want 1", len(result.Attachments))
	}
}

func TestParseFile_Allure3_Passed(t *testing.T) {
	t.Parallel()
	path := testdataPath(t, "allure3-result.json")
	result, err := parser.ParseFile(path)
	if err != nil {
		t.Fatalf("ParseFile returned unexpected error: %v", err)
	}

	if result.Status != "passed" {
		t.Errorf("Status: got %q, want %q", result.Status, "passed")
	}
	if result.StatusMessage != "" {
		t.Errorf("StatusMessage: expected empty, got %q", result.StatusMessage)
	}

	if len(result.Labels) != 3 {
		t.Errorf("Labels: got %d, want 3", len(result.Labels))
	}

	if len(result.Steps) != 1 {
		t.Errorf("Steps: got %d, want 1", len(result.Steps))
	} else if result.Steps[0].Status != "passed" {
		t.Errorf("Steps[0].Status: got %q, want %q", result.Steps[0].Status, "passed")
	}

	if result.DurationMs <= 0 {
		t.Errorf("DurationMs: got %d, want > 0", result.DurationMs)
	}
	if result.StartMs <= 0 {
		t.Errorf("StartMs: got %d, want > 0", result.StartMs)
	}
}

func TestParseFile_NonExistent(t *testing.T) {
	t.Parallel()
	_, err := parser.ParseFile("/tmp/does-not-exist-allure-result.json")
	if err == nil {
		t.Fatal("expected error for non-existent file, got nil")
	}
	if !strings.Contains(err.Error(), "no such file") {
		t.Errorf("error should wrap 'no such file', got: %v", err)
	}
}

func TestParseFile_EmptyName(t *testing.T) {
	t.Parallel()
	tmp, err := os.CreateTemp(t.TempDir(), "*-result.json")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tmp.Close() }()

	data := map[string]any{
		"name":   "",
		"status": "passed",
	}
	if err := json.NewEncoder(tmp).Encode(data); err != nil {
		t.Fatal(err)
	}

	result, err := parser.ParseFile(tmp.Name())
	if err != nil {
		t.Fatalf("ParseFile returned unexpected error: %v", err)
	}
	if result.Name != "" {
		t.Errorf("Name: got %q, want empty string", result.Name)
	}
}

func TestParseDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Write 2 result files.
	for _, name := range []string{"aaa-result.json", "bbb-result.json"} {
		f, err := os.Create(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.NewEncoder(f).Encode(map[string]any{"name": name, "status": "passed"}); err != nil {
			_ = f.Close()
			t.Fatal(err)
		}
		_ = f.Close()
	}

	// Write 1 non-result file that should be skipped.
	f, err := os.Create(filepath.Join(dir, "executor.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.NewEncoder(f).Encode(map[string]any{"type": "jenkins"}); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	_ = f.Close()

	results, err := parser.ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir returned unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("ParseDir: got %d results, want 2", len(results))
	}
}

func TestParseDir_EmptyDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	results, err := parser.ParseDir(dir)
	if err != nil {
		t.Fatalf("ParseDir returned unexpected error on empty dir: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("ParseDir: got %d results, want 0", len(results))
	}
}

func TestResolveAttachments_WithMapping(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Simulate Allure-generated directory structure: data/test-results/*.json + data/attachments/
	testResultsDir := filepath.Join(dir, "test-results")
	if err := os.MkdirAll(testResultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "attachments"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Write a generated test result file with the hashed attachment mapping.
	genResult := `{
		"name": "test1",
		"attachments": [
			{
				"link": {
					"id": "abc123hash",
					"originalFileName": "screenshot-001.png",
					"ext": ".png",
					"contentType": "image/png",
					"contentLength": 4096,
					"name": "screenshot.png",
					"used": true,
					"missed": false
				},
				"type": "attachment"
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(testResultsDir, "aaa-result.json"), []byte(genResult), 0o644); err != nil {
		t.Fatal(err)
	}

	results := []*parser.Result{
		{
			Name: "test1",
			Attachments: []parser.Attachment{
				{Name: "screenshot.png", Source: "screenshot-001.png", MimeType: "image/png"},
			},
		},
	}

	parser.ResolveAttachments(results, dir)

	att := results[0].Attachments[0]
	if att.Source != "abc123hash.png" {
		t.Errorf("Source = %q, want %q", att.Source, "abc123hash.png")
	}
	if att.Size != 4096 {
		t.Errorf("Size = %d, want 4096", att.Size)
	}
}

func TestResolveAttachments_FallbackToStat(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// No test-results dir → fallback to stat-ing files directly (Allure 2 behavior).
	attDir := filepath.Join(dir, "attachments")
	if err := os.MkdirAll(attDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := []byte("hello, this is a 42-byte attachment file!!")
	if err := os.WriteFile(filepath.Join(attDir, "abc-screenshot.png"), content, 0o644); err != nil {
		t.Fatal(err)
	}

	results := []*parser.Result{
		{
			Name: "test1",
			Attachments: []parser.Attachment{
				{Name: "screenshot.png", Source: "abc-screenshot.png", MimeType: "image/png"},
			},
		},
	}

	parser.ResolveAttachments(results, dir)

	att := results[0].Attachments[0]
	if att.Source != "abc-screenshot.png" {
		t.Errorf("Source should remain %q for fallback, got %q", "abc-screenshot.png", att.Source)
	}
	if att.Size != int64(len(content)) {
		t.Errorf("Size = %d, want %d", att.Size, len(content))
	}
}

func TestResolveAttachments_MissingFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir() // empty — no test-results, no attachments

	results := []*parser.Result{
		{
			Name: "test1",
			Attachments: []parser.Attachment{
				{Name: "missing.png", Source: "no-such-file.png", MimeType: "image/png"},
			},
		},
	}

	parser.ResolveAttachments(results, dir)

	if results[0].Attachments[0].Size != 0 {
		t.Errorf("Size = %d, want 0 for missing file", results[0].Attachments[0].Size)
	}
}

func TestResolveAttachments_StepAttachments(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	testResultsDir := filepath.Join(dir, "test-results")
	if err := os.MkdirAll(testResultsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "attachments"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Generated result maps step-log.txt to a hash, but nonexistent.txt is not in the mapping.
	genResult := `{
		"name": "test1",
		"attachments": [
			{
				"link": {
					"id": "hashsteplog",
					"originalFileName": "step-log.txt",
					"ext": ".txt",
					"contentType": "text/plain",
					"contentLength": 19,
					"name": "log.txt",
					"used": true,
					"missed": false
				},
				"type": "attachment"
			}
		]
	}`
	if err := os.WriteFile(filepath.Join(testResultsDir, "bbb-result.json"), []byte(genResult), 0o644); err != nil {
		t.Fatal(err)
	}

	results := []*parser.Result{
		{
			Name: "test1",
			Steps: []parser.Step{
				{
					Name: "step1",
					Attachments: []parser.Attachment{
						{Name: "log.txt", Source: "step-log.txt", MimeType: "text/plain"},
					},
					Steps: []parser.Step{
						{
							Name: "nested-step",
							Attachments: []parser.Attachment{
								{Name: "missing.txt", Source: "nonexistent.txt", MimeType: "text/plain"},
							},
						},
					},
				},
			},
		},
	}

	parser.ResolveAttachments(results, dir)

	if results[0].Steps[0].Attachments[0].Source != "hashsteplog.txt" {
		t.Errorf("step attachment Source = %q, want %q", results[0].Steps[0].Attachments[0].Source, "hashsteplog.txt")
	}
	if results[0].Steps[0].Attachments[0].Size != 19 {
		t.Errorf("step attachment Size = %d, want 19", results[0].Steps[0].Attachments[0].Size)
	}
	if results[0].Steps[0].Steps[0].Attachments[0].Size != 0 {
		t.Errorf("nested missing attachment Size = %d, want 0", results[0].Steps[0].Steps[0].Attachments[0].Size)
	}
}
