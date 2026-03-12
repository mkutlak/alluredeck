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
