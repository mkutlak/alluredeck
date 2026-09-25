package handlers

import "testing"

// NamespacedProjectID and ShortProjectName round-trip through the "--"
// separator; a project without a parent keeps its short name.
func TestProjectNamespace(t *testing.T) {
	t.Parallel()
	tests := []struct{ parentID, shortID, namespaced string }{
		{"", "api-licences", "api-licences"},
		{"acme-api-tests", "api-licences", "acme-api-tests--api-licences"},
		{"acme-ui-tests", "api-exports", "acme-ui-tests--api-exports"},
	}
	for _, tt := range tests {
		if got := NamespacedProjectID(tt.parentID, tt.shortID); got != tt.namespaced {
			t.Errorf("NamespacedProjectID(%q, %q) = %q, want %q", tt.parentID, tt.shortID, got, tt.namespaced)
		}
		if got := ShortProjectName(tt.namespaced); got != tt.shortID {
			t.Errorf("ShortProjectName(%q) = %q, want %q", tt.namespaced, got, tt.shortID)
		}
	}
}
