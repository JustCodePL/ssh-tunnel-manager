//go:build windows

package tray

import (
	"runtime"

	"github.com/energye/systray"
)

// startTray runs the system tray on Windows.
//
// On Windows, Win32 message dispatch requires that the window is created
// AND the GetMessage loop runs on the SAME OS thread. RunWithExternalLoop
// creates the window synchronously (in the caller's goroutine) but then
// nativeStart() spawns a NEW goroutine for the message loop — so they end
// up on different OS threads and tray clicks are never delivered.
//
// Fix: use systray.Run() (blocking) inside a goroutine that is pinned to a
// single OS thread via runtime.LockOSThread(). Both initInstance (window
// creation) and nativeLoop (GetMessage) then run on the same OS thread.
//
// The end/quit callback is stored before blocking; systray.Quit() stops
// the loop from another goroutine.
func (t *Tray) startTray() {
	ready := make(chan struct{})

	go func() {
		runtime.LockOSThread()

		// Wrap onReady so we can signal the caller that setup is done.
		wrappedReady := func() {
			t.onReady()
			close(ready)
		}

		// Store a quit function before blocking.
		t.mu.Lock()
		t.endFunc = func() { systray.Quit() }
		t.mu.Unlock()

		// Run blocks until systray.Quit() is called.
		systray.Run(wrappedReady, t.onExit)
	}()

	<-ready // wait for tray to be fully initialised before returning
}
