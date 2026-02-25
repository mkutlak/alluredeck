package version_test

import (
	"testing"

	"github.com/mkutlak/alluredeck/api/internal/version"
)

func TestDefaults(t *testing.T) {
	if version.Version == "" {
		t.Error("Version must not be empty")
	}
	if version.BuildDate == "" {
		t.Error("BuildDate must not be empty")
	}
	if version.BuildRef == "" {
		t.Error("BuildRef must not be empty")
	}
}
