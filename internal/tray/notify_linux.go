//go:build linux

package tray

import (
	"log/slog"
	"os/exec"
)

// notify sends a desktop notification on Linux via notify-send.
func notify(title, body string) {
	path, err := exec.LookPath("notify-send")
	if err != nil {
		slog.Debug("notify-send not available", "error", err)
		return
	}
	cmd := exec.Command(path, "--app-name=SSH Tunnel Manager", title, body)
	if err := cmd.Start(); err != nil {
		slog.Debug("notify-send failed", "error", err)
		return
	}
	// Don't wait — fire and forget
	go func() { _ = cmd.Wait() }()
}
