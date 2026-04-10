package handlers

import "testing"

func TestNamespacedProjectID(t *testing.T) {
	tests := []struct {
		parentID string
		shortID  string
		want     string
	}{
		{"", "api-licences", "api-licences"},
		{"roger-api-tests", "api-licences", "roger-api-tests--api-licences"},
		{"roger-ui-tests", "api-exports", "roger-ui-tests--api-exports"},
	}
	for _, tt := range tests {
		got := NamespacedProjectID(tt.parentID, tt.shortID)
		if got != tt.want {
			t.Errorf("NamespacedProjectID(%q, %q) = %q, want %q", tt.parentID, tt.shortID, got, tt.want)
		}
	}
}

func TestShortProjectName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"api-licences", "api-licences"},
		{"roger-api-tests--api-licences", "api-licences"},
		{"roger-ui-tests--api-exports", "api-exports"},
		{"no-namespace-here", "no-namespace-here"},
	}
	for _, tt := range tests {
		got := ShortProjectName(tt.input)
		if got != tt.want {
			t.Errorf("ShortProjectName(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
