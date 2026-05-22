package parser

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ParseEnvironment reads an Allure environment.properties file from resultsDir
// and returns its key/value pairs as a map. The format is Java-style properties:
// lines starting with '#' or '!' are comments, blank lines are skipped, and
// each entry is split on the first '=' only (values may contain '=').
// Whitespace is trimmed from both key and value.
//
// If the file does not exist, ParseEnvironment returns (nil, nil) — environment
// metadata is best-effort context, not a required artifact.
// Any other read error is wrapped and returned.
func ParseEnvironment(resultsDir string) (map[string]string, error) {
	path := filepath.Join(resultsDir, "environment.properties")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read environment properties %q: %w", path, err)
	}

	result := make(map[string]string)
	for raw := range bytes.SplitSeq(data, []byte("\n")) {
		line := strings.TrimSpace(string(raw))
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		before, after, ok := strings.Cut(line, "=")
		if !ok {
			// No '=' on this line — skip malformed entry.
			continue
		}
		key := strings.TrimSpace(before)
		value := strings.TrimSpace(after)
		if key == "" {
			continue
		}
		result[key] = value
	}
	return result, nil
}
