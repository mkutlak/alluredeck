package handlers

import (
	"net/http"
	"strings"
	"testing"
)

func TestSearch(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  int
	}{
		{name: "missing query", query: "", want: http.StatusBadRequest},
		{name: "query too short", query: "q=a", want: http.StatusBadRequest},
		{name: "query too long", query: "q=" + strings.Repeat("x", 101), want: http.StatusBadRequest},
		{name: "no matches", query: "q=nonexistent", want: http.StatusOK},
		// Out-of-range and malformed limits fall back to a bound, never a 400.
		{name: "limit above max", query: "q=test&limit=999", want: http.StatusOK},
		{name: "non-numeric limit", query: "q=test&limit=abc", want: http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, body := serveJSON(t, newTestSearchHandler(t).Search, http.MethodGet, "/api/v1/search?"+tc.query, "")
			if code != tc.want {
				t.Fatalf("status = %d, want %d: %v", code, tc.want, body)
			}
			if code != http.StatusOK {
				if msg, _ := jsonAt(body, "metadata.message").(string); msg == "" {
					t.Errorf("400 without an error message: %v", body)
				}
				return
			}
			wantJSON(t, body, map[string]any{"data.projects#": 0, "data.tests#": 0})
		})
	}
}
