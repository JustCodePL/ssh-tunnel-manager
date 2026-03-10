//go:build linux

package autostart

import (
	"fmt"
	"os"
	"path/filepath"
)

const desktopEntry = `[Desktop Entry]
Type=Application
Name=SSH Tunnel Manager
Exec=%s --hidden
Hidden=false
NoDisplay=false
X-GNOME-Autostart-enabled=true
Comment=SSH tunnel manager with port forwarding
`

// IsEnabled returns true if the autostart .desktop file exists.
func IsEnabled() (bool, error) {
	path, err := desktopFilePath()
	if err != nil {
		return false, err
	}
	_, err = os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	return err == nil, err
}

// Enable creates a .desktop file in ~/.config/autostart/.
func Enable() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}
	path, err := desktopFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating autostart dir: %w", err)
	}
	content := fmt.Sprintf(desktopEntry, exePath)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing autostart file: %w", err)
	}
	return nil
}

// Disable removes the autostart .desktop file.
func Disable() error {
	path, err := desktopFilePath()
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func desktopFilePath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("getting config dir: %w", err)
	}
	return filepath.Join(configDir, "autostart", "ssh-tunnel-manager.desktop"), nil
}
