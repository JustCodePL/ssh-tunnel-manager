//go:build !windows

package tray

// wrapIconBytes and wrapDotIconBytes are no-ops on non-Windows — PNG is accepted directly.
func wrapIconBytes(b []byte) []byte    { return b }
func wrapDotIconBytes(b []byte) []byte { return b }
