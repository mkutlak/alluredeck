package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnvironment_ParsesSample(t *testing.T) {
	dir := t.TempDir()
	content := `# Comment line
! Another comment
Base.URL=https://example.com
Loki.Query={k8s_namespace_name="ns-x"}

  Key.With.Spaces  =  value with spaces
Empty.Key=
`
	if err := os.WriteFile(filepath.Join(dir, "environment.properties"), []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got, err := ParseEnvironment(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	want := map[string]string{
		"Base.URL":        "https://example.com",
		"Loki.Query":      `{k8s_namespace_name="ns-x"}`,
		"Key.With.Spaces": "value with spaces",
		"Empty.Key":       "",
	}
	if len(got) != len(want) {
		t.Errorf("len(got)=%d want %d; got=%v", len(got), len(want), got)
	}
	for k, wv := range want {
		if gv, ok := got[k]; !ok {
			t.Errorf("missing key %q", k)
		} else if gv != wv {
			t.Errorf("key %q: got %q want %q", k, gv, wv)
		}
	}
}

func TestParseEnvironment_ValueContainsEquals(t *testing.T) {
	dir := t.TempDir()
	// Value contains = signs (e.g. a Loki query or a base64 value).
	content := "Loki.Query={k8s_namespace_name=\"ns-x\",job=\"app\"}\n"
	if err := os.WriteFile(filepath.Join(dir, "environment.properties"), []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got, err := ParseEnvironment(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `{k8s_namespace_name="ns-x",job="app"}`
	if got["Loki.Query"] != want {
		t.Errorf("got %q want %q", got["Loki.Query"], want)
	}
}

func TestParseEnvironment_FileAbsent(t *testing.T) {
	dir := t.TempDir() // no environment.properties written

	got, err := ParseEnvironment(dir)
	if err != nil {
		t.Fatalf("expected nil error for absent file, got: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil map for absent file, got: %v", got)
	}
}

func TestParseEnvironment_DuplicateKeyLastWins(t *testing.T) {
	dir := t.TempDir()
	content := "Host=first\nHost=second\n"
	if err := os.WriteFile(filepath.Join(dir, "environment.properties"), []byte(content), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}

	got, err := ParseEnvironment(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["Host"] != "second" {
		t.Errorf("duplicate key: got %q, want %q", got["Host"], "second")
	}
}
