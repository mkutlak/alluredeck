// Package static embeds static asset directories served by the API.
// Keeping embed declarations here keeps //go:embed paths simple and avoids
// the Go restriction that embed paths cannot contain "..".
package static

import (
	"embed"
	"io/fs"
)

//go:embed all:trace
var traceFS embed.FS

// TraceViewerFS returns a sub-FS rooted at the embedded trace/ directory,
// suitable for passing to http.FileServer(http.FS(...)).
func TraceViewerFS() (fs.FS, error) {
	return fs.Sub(traceFS, "trace")
}
