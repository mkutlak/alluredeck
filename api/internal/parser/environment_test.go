package parser

import (
	"maps"
	"os"
	"path/filepath"
	"testing"
)

// TestParseEnvironment covers Allure's environment.properties syntax: # and !
// comments, blank lines, trimmed keys/values, empty values, values that
// themselves contain '=' (Loki queries), and last-wins duplicate keys. An
// absent file is not an error and yields a nil map.
func TestParseEnvironment(t *testing.T) {
	if got, err := ParseEnvironment(t.TempDir()); err != nil || got != nil {
		t.Errorf("absent file: got %v, err %v; want nil, nil", got, err)
	}

	dir := t.TempDir()
	content := `# Comment line
! Another comment
Base.URL=https://example.com
Loki.Query={k8s_namespace_name="ns-x",job="app"}

  Key.With.Spaces  =  value with spaces
Empty.Key=
Host=first
Host=second
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
		"Loki.Query":      `{k8s_namespace_name="ns-x",job="app"}`,
		"Key.With.Spaces": "value with spaces",
		"Empty.Key":       "",
		"Host":            "second",
	}
	if !maps.Equal(got, want) {
		t.Errorf("ParseEnvironment = %v, want %v", got, want)
	}
}
