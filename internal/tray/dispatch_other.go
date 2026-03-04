//go:build !darwin && !windows

package tray

import (
	"github.com/energye/systray"
)

// dispatchOnMainThread on Linux calls fn directly.
func dispatchOnMainThread(fn func()) {
	fn()
}

// startTray on Linux uses the standard RunWithExternalLoop approach.
func (t *Tray) startTray() {
	start, end := systray.RunWithExternalLoop(t.onReady, t.onExit)
	t.mu.Lock()
	t.endFunc = end
	t.mu.Unlock()
	dispatchOnMainThread(start)
}

