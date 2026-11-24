// Package webview holds checks shared by the loopback servers that only the
// app's own webview may use.
package webview

import "net/url"

// OriginAllowed accepts only the webview's own origin, so a web page opened in
// a browser cannot reach a loopback bridge even if it learned the port.
// allowDev additionally accepts the loopback origins used by `wails dev` and
// the browser build.
func OriginAllowed(origin string, allowDev bool) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch {
	case parsed.Scheme == "wails" && (parsed.Host == "localhost" || parsed.Host == "wails.localhost" || parsed.Host == "wails"):
		// The page origin under Wails 3's custom scheme.
		return true
	case (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() == "wails.localhost":
		return true
	case allowDev && parsed.Scheme == "http":
		return parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1"
	}
	return false
}
