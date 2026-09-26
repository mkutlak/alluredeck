package security

import (
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

// TestResolveRole maps OIDC groups to a role: admin beats editor, anything
// else (or no group mapping configured at all) gets the default role.
func TestResolveRole(t *testing.T) {
	admins, editors := []string{"admins"}, []string{"editors"}
	tests := []struct {
		name                string
		adminGrp, editorGrp []string
		groups              []string
		want                string
	}{
		{"admin group", admins, editors, []string{"admins"}, "admin"},
		{"editor group", admins, editors, []string{"editors"}, "editor"},
		{"no matching group", admins, editors, []string{"other-group"}, "viewer"},
		{"admin wins over editor", admins, editors, []string{"readers", "admins", "editors"}, "admin"},
		{"group mapped to both roles", []string{"superusers"}, []string{"superusers"}, []string{"superusers"}, "admin"},
		{"no group mapping configured", []string{}, []string{}, []string{"admins", "editors"}, "viewer"},
	}
	for _, tc := range tests {
		cfg := &config.OIDCConfig{AdminGroups: tc.adminGrp, EditorGroups: tc.editorGrp, DefaultRole: "viewer"}
		if got := ResolveRole(tc.groups, cfg); got != tc.want {
			t.Errorf("%s: ResolveRole(%v) = %q, want %q", tc.name, tc.groups, got, tc.want)
		}
	}
}
