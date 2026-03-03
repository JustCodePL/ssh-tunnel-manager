//go:build darwin

package tray

// dispatchOnMainThread schedules fn on the macOS main thread via GCD.
// On macOS, AppKit (NSStatusBar, NSWindow, etc.) requires all UI operations
// to happen on the main OS thread. Wails calls startup() from a goroutine
// that is NOT the main OS thread, so we must use dispatch_async_f to
// forward the systray start() call to the main thread's run queue.

/*
#cgo LDFLAGS: -framework Foundation
#include <dispatch/dispatch.h>

extern void _trayStartCallback(void);

static void _tray_dispatch_helper(void *ctx) {
	_trayStartCallback();
}

static void _tray_dispatch_to_main(void) {
	dispatch_async_f(dispatch_get_main_queue(), NULL, _tray_dispatch_helper);
}
*/
import "C"

func dispatchOnMainThread(fn func()) {
	_pendingStart.Lock()
	_pendingStart.fn = fn
	_pendingStart.Unlock()
	C._tray_dispatch_to_main()
}
