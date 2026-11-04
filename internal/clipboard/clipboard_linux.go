//go:build linux

package clipboard

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
)

func useWayland() bool {
	if os.Getenv("WAYLAND_DISPLAY") == "" {
		return false
	}
	_, err := exec.LookPath("wl-copy")
	return err == nil
}

// ReadImage returns the PNG on the clipboard, or nil when there is none.
func ReadImage() ([]byte, error) {
	var cmd *exec.Cmd
	switch {
	case useWayland():
		cmd = exec.Command("wl-paste", "--type", "image/png")
	default:
		if _, err := exec.LookPath("xclip"); err != nil {
			return nil, ErrUnavailable
		}
		cmd = exec.Command("xclip", "-selection", "clipboard", "-t", "image/png", "-o")
	}
	out, err := cmd.Output()
	if err != nil || !IsPNG(out) {
		return nil, nil
	}
	return out, nil
}

// WriteImage puts a PNG on the clipboard.
func WriteImage(data []byte) error {
	var cmd *exec.Cmd
	switch {
	case useWayland():
		cmd = exec.Command("wl-copy", "--type", "image/png")
	default:
		if _, err := exec.LookPath("xclip"); err != nil {
			return ErrUnavailable
		}
		cmd = exec.Command("xclip", "-selection", "clipboard", "-t", "image/png", "-i")
	}
	cmd.Stdin = bytes.NewReader(data)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("setting clipboard image: %w", err)
	}
	// Both tools stay running to serve the clipboard, so do not wait for them.
	go func() { _ = cmd.Wait() }()
	return nil
}
