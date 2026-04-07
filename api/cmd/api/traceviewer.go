package main

import (
	"net/http"

	"github.com/mkutlak/alluredeck/api/static"
)

// traceViewerHandler serves the embedded Playwright trace viewer static files at /trace/.
// It sets a permissive CSP (the viewer requires inline scripts, styles, and eval)
// and removes X-Frame-Options so the viewer can be embedded in same-origin iframes.
//
// No authentication is required — the viewer is a static web app. The trace zip
// it loads comes from the authenticated attachments endpoint, so the data remains
// protected.
func newTraceViewerHandler(frameAncestors string) http.Handler {
	sub, err := static.TraceViewerFS()
	if err != nil {
		// trace/ is embedded at build time; failure here is a programming error.
		panic("traceviewer: failed to sub embedded FS: " + err.Error())
	}
	csp := "default-src 'self' 'unsafe-inline' 'unsafe-eval' blob: data: https:; frame-ancestors " + frameAncestors
	fileServer := http.FileServer(http.FS(sub))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", csp)
		// Remove X-Frame-Options set by SecurityHeaders middleware; CSP frame-ancestors
		// is the modern replacement and takes precedence in all current browsers.
		w.Header().Del("X-Frame-Options")
		// Strip the /trace prefix so the file server resolves paths relative to
		// the root of the embedded trace/ directory.
		http.StripPrefix("/trace", fileServer).ServeHTTP(w, r)
	})
}
