package security

import (
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/config"
)

func TestResolveRole_AdminMatch(t *testing.T) {
	cfg := &config.OIDCConfig{
		AdminGroups:  []string{"admins"},
		EditorGroups: []string{"editors"},
		DefaultRole:  "viewer",
	}
	got := ResolveRole([]string{"admins"}, cfg)
	if got != "admin" {
		t.Errorf("expected admin, got %q", got)
	}
}

func TestResolveRole_EditorMatch(t *testing.T) {
	cfg := &config.OIDCConfig{
		AdminGroups:  []string{"admins"},
		EditorGroups: []string{"editors"},
		DefaultRole:  "viewer",
	}
	got := ResolveRole([]string{"editors"}, cfg)
	if got != "editor" {
		t.Errorf("expected editor, got %q", got)
	}
}

func TestResolveRole_NoMatch(t *testing.T) {
	cfg := &config.OIDCConfig{
		AdminGroups:  []string{"admins"},
		EditorGroups: []string{"editors"},
		DefaultRole:  "viewer",
	}
	got := ResolveRole([]string{"other-group"}, cfg)
	if got != "viewer" {
		t.Errorf("expected viewer, got %q", got)
	}
}

func TestResolveRole_AdminWinsOverEditor(t *testing.T) {
	cfg := &config.OIDCConfig{
		AdminGroups:  []string{"superusers"},
		EditorGroups: []string{"superusers"},
		DefaultRole:  "viewer",
	}
	got := ResolveRole([]string{"superusers"}, cfg)
	if got != "admin" {
		t.Errorf("expected admin, got %q", got)
	}
}

func TestResolveRole_EmptyGroups(t *testing.T) {
	cfg := &config.OIDCConfig{
		AdminGroups:  []string{"admins"},
		EditorGroups: []string{"editors"},
		DefaultRole:  "viewer",
	}
	got := ResolveRole([]string{}, cfg)
	if got != "viewer" {
		t.Errorf("expected viewer, got %q", got)
	}
}

func TestResolveRole_MultipleGroups(t *testing.T) {
	cfg := &config.OIDCConfig{
		AdminGroups:  []string{"admins"},
		EditorGroups: []string{"editors"},
		DefaultRole:  "viewer",
	}
	got := ResolveRole([]string{"readers", "admins", "editors"}, cfg)
	if got != "admin" {
		t.Errorf("expected admin, got %q", got)
	}
}

func TestResolveRole_EmptyConfig(t *testing.T) {
	cfg := &config.OIDCConfig{
		AdminGroups:  []string{},
		EditorGroups: []string{},
		DefaultRole:  "viewer",
	}
	got := ResolveRole([]string{"admins", "editors"}, cfg)
	if got != "viewer" {
		t.Errorf("expected viewer, got %q", got)
	}
}
