package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

// TestResponseEnvelope pins the JSON envelope every handler answers with
// (269fdc0): {data, metadata.message[, pagination]} with a JSON content type.
func TestResponseEnvelope(t *testing.T) {
	t.Parallel()
	rows := []struct {
		name   string
		status int
		want   map[string]any
		write  func(w http.ResponseWriter)
	}{
		{"writeJSON", http.StatusCreated, map[string]any{"data": "hello"},
			func(w http.ResponseWriter) { writeJSON(w, http.StatusCreated, map[string]any{"data": "hello"}) }},
		{"writeError", http.StatusBadRequest, map[string]any{"metadata": map[string]any{"message": "went wrong"}},
			func(w http.ResponseWriter) { writeError(w, http.StatusBadRequest, "went wrong") }},
		{"writeSuccess", http.StatusOK, map[string]any{"data": map[string]any{"id": "p1"}, "metadata": map[string]any{"message": "Created"}},
			func(w http.ResponseWriter) { writeSuccess(w, http.StatusOK, map[string]string{"id": "p1"}, "Created") }},
		{"writePagedSuccess", http.StatusOK, map[string]any{
			"data":       []any{"a", "b"},
			"metadata":   map[string]any{"message": "Listed"},
			"pagination": map[string]any{"page": 1.0, "per_page": 20.0, "total": 42.0, "total_pages": 3.0},
		}, func(w http.ResponseWriter) {
			writePagedSuccess(w, []string{"a", "b"}, "Listed", newPaginationMeta(1, 20, 42))
		}},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rr := httptest.NewRecorder()
			tc.write(rr)
			if rr.Code != tc.status || rr.Header().Get("Content-Type") != "application/json" {
				t.Errorf("status = %d, Content-Type = %q, want %d and application/json", rr.Code, rr.Header().Get("Content-Type"), tc.status)
			}
			var got map[string]any
			if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("body = %v, want %v", got, tc.want)
			}
		})
	}
}
