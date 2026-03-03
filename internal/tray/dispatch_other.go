//go:build !darwin

package tray

// dispatchOnMainThread on non-Darwin platforms calls fn directly.
// Windows and Linux systray libraries do not have the same main-thread
// restriction as macOS AppKit.
func dispatchOnMainThread(fn func()) {
	fn()
}
