// Package version exposes build-time variables populated via -ldflags.
package version

// These variables are set at build time via:
//
//	-ldflags="-X github.com/mkutlak/alluredeck/api/internal/version.Version=..."
var (
	Version   = "dev"
	BuildDate = "unknown"
	BuildRef  = "unknown"
)
