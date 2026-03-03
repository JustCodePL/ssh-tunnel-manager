//go:build darwin

package tray

// This file exports a Go function to C so the GCD dispatch helper in
// dispatch_darwin.go can call back into Go on the main OS thread.
// The preamble MUST contain only declarations (no definitions) because
// this file uses //export.

import "C"
import "sync"

// _pendingStart holds the systray start function to be invoked on the main thread.
var _pendingStart struct {
	sync.Mutex
	fn func()
}

//export _trayStartCallback
func _trayStartCallback() {
	_pendingStart.Lock()
	fn := _pendingStart.fn
	_pendingStart.fn = nil
	_pendingStart.Unlock()
	if fn != nil {
		fn()
	}
}
