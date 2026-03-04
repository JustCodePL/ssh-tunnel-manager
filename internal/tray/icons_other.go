//go:build !windows

package tray

// wrapIconBytes is a no-op on non-Windows platforms — PNG is accepted directly.
func wrapIconBytes(b []byte) []byte { return b }
