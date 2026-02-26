//go:build darwin

package tray

import (
	"log/slog"
	"os/exec"
)

// notify sends a desktop notification on macOS via osascript.
func notify(title, body string) {
	cmd := exec.Command("osascript", "-e",
		`display notification "`+body+`" with title "`+title+`"`)
	if err := cmd.Start(); err != nil {
		slog.Debug("osascript notification failed", "error", err)
		return
	}
	go func() { _ = cmd.Wait() }()
}
