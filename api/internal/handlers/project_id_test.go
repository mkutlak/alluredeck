package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// TestValidateProjectID: create/rename slugs must stay under projectsDir and
// must not shadow API route segments.
func TestValidateProjectID(t *testing.T) {
	t.Parallel()
	projectsDir := t.TempDir()
	for _, tt := range []struct {
		id   string
		want error
	}{
		{"my-project", nil},
		{strings.Repeat("a", 100), nil},
		{"", ErrProjectRequired},
		{strings.Repeat("a", 101), ErrProjectTooLong},
		{"../etc/passwd", ErrProjectInvalidChars},
		{"foo..bar", ErrProjectInvalidChars},
		{"foo/bar", ErrProjectInvalidChars},
		{"foo\\bar", ErrProjectInvalidChars},
		{"swagger", ErrProjectReserved},
		{"version", ErrProjectReserved},
	} {
		if err := validateProjectID(projectsDir, tt.id); !errors.Is(err, tt.want) {
			t.Errorf("validateProjectID(%q) = %v, want %v", tt.id, err, tt.want)
		}
	}
}

// Request/response helpers for the handler tests that assert on JSON bodies.

// serveJSON serves one request to target through fn, with pathKV as
// alternating path-value names and values ("project_id", "1"), and returns
// the status and the decoded JSON body (the raw text if it is not JSON).
func serveJSON(t *testing.T, fn http.HandlerFunc, method, target, body string, pathKV ...string) (int, any) {
	t.Helper()
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for i := 0; i+1 < len(pathKV); i += 2 {
		req.SetPathValue(pathKV[i], pathKV[i+1])
	}
	rr := httptest.NewRecorder()
	fn(rr, req)
	var decoded any
	if json.Unmarshal(rr.Body.Bytes(), &decoded) != nil {
		decoded = rr.Body.String()
	}
	return rr.Code, decoded
}

// wantJSON asserts that each dotted path of want resolves to its value in a
// decoded JSON body; see jsonAt for the path syntax. Integer wants compare
// against JSON numbers, and a nil want matches JSON null or a missing key.
func wantJSON(t *testing.T, body any, want map[string]any) {
	t.Helper()
	for path, w := range want {
		switch n := w.(type) {
		case int:
			w = float64(n)
		case int64:
			w = float64(n)
		}
		if got := jsonAt(body, path); !reflect.DeepEqual(got, w) {
			t.Errorf("%s = %#v, want %#v", path, got, w)
		}
	}
}

// jsonAt walks a dotted path ("data.projects.0.slug") through decoded JSON;
// numeric segments index arrays. A trailing "#" yields the length of the array
// at the path ("#" alone: the top-level array), or nil when the value is not an
// array, so a JSON null never passes for an empty list.
func jsonAt(v any, path string) any {
	path, count := strings.CutSuffix(path, "#")
	var segs []string
	if path != "" {
		segs = strings.Split(path, ".")
	}
	for _, seg := range segs {
		switch x := v.(type) {
		case map[string]any:
			v = x[seg]
		case []any:
			i, err := strconv.Atoi(seg)
			if err != nil || i < 0 || i >= len(x) {
				return nil
			}
			v = x[i]
		default:
			return nil
		}
	}
	if !count {
		return v
	}
	if arr, ok := v.([]any); ok {
		return float64(len(arr))
	}
	return nil
}
