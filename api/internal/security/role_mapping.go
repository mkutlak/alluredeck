package security

import (
	"github.com/mkutlak/alluredeck/api/internal/config"
)

// ResolveRole maps OIDC group memberships to an application role.
// Priority: admin > editor > viewer. Falls back to cfg.DefaultRole when no groups match.
func ResolveRole(groups []string, cfg *config.OIDCConfig) string {
	if len(cfg.AdminGroups) == 0 && len(cfg.EditorGroups) == 0 {
		return cfg.DefaultRole
	}

	adminSet := make(map[string]struct{}, len(cfg.AdminGroups))
	for _, g := range cfg.AdminGroups {
		adminSet[g] = struct{}{}
	}

	editorSet := make(map[string]struct{}, len(cfg.EditorGroups))
	for _, g := range cfg.EditorGroups {
		editorSet[g] = struct{}{}
	}

	isAdmin := false
	isEditor := false

	for _, g := range groups {
		if _, ok := adminSet[g]; ok {
			isAdmin = true
		}
		if _, ok := editorSet[g]; ok {
			isEditor = true
		}
	}

	switch {
	case isAdmin:
		return "admin"
	case isEditor:
		return "editor"
	default:
		return cfg.DefaultRole
	}
}
