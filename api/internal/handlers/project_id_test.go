package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

func TestExtractProjectID(t *testing.T) {
	t.Parallel()
	projectsDir := t.TempDir()

	tests := []struct {
		name       string
		pathValue  string
		wantOK     bool
		wantID     string
		wantStatus int
		wantMsg    string
	}{
		{name: "empty defaults to default", pathValue: "", wantOK: true, wantID: "default"},
		{name: "invalid encoding rejected", pathValue: "%zz", wantOK: false, wantStatus: http.StatusBadRequest, wantMsg: "invalid project_id encoding"},
		{name: "normal id", pathValue: "my-project", wantOK: true, wantID: "my-project"},
		{name: "url encoded space", pathValue: "my%20project", wantOK: true, wantID: "my project"},
		{name: "path traversal rejected", pathValue: "../etc/passwd", wantOK: false, wantStatus: http.StatusBadRequest},
		{name: "double dot rejected", pathValue: "foo..bar", wantOK: false, wantStatus: http.StatusBadRequest},
		{name: "slash rejected", pathValue: "foo/bar", wantOK: false, wantStatus: http.StatusBadRequest},
		{name: "backslash rejected", pathValue: "foo\\bar", wantOK: false, wantStatus: http.StatusBadRequest},
		{name: "reserved name swagger", pathValue: "swagger", wantOK: false, wantStatus: http.StatusBadRequest},
		{name: "reserved name version", pathValue: "version", wantOK: false, wantStatus: http.StatusBadRequest},
		{name: "overlong id rejected", pathValue: strings.Repeat("a", 101), wantOK: false, wantStatus: http.StatusBadRequest},
		{name: "max length accepted", pathValue: strings.Repeat("a", 100), wantOK: true, wantID: strings.Repeat("a", 100)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req, _ := http.NewRequest(http.MethodGet, "/test", nil)
			req.SetPathValue("project_id", tt.pathValue)
			rr := httptest.NewRecorder()

			gotID, gotOK := extractProjectID(rr, req, projectsDir)

			if gotOK != tt.wantOK {
				t.Fatalf("extractProjectID() ok = %v, want %v; body: %s", gotOK, tt.wantOK, rr.Body.String())
			}
			if tt.wantOK {
				if gotID != tt.wantID {
					t.Errorf("extractProjectID() id = %q, want %q", gotID, tt.wantID)
				}
			} else if rr.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body: %s", rr.Code, tt.wantStatus, rr.Body.String())
			}
			if tt.wantMsg != "" {
				var resp struct {
					Metadata ResponseMeta `json:"metadata"`
				}
				if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
					t.Fatal(err)
				}
				if resp.Metadata.Message != tt.wantMsg {
					t.Errorf("message = %q, want %q", resp.Metadata.Message, tt.wantMsg)
				}
			}
		})
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
