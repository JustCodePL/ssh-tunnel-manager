//go:build windows

package autostart

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	regKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	regValue   = "SSHTunnelManager"
)

// IsEnabled returns true if the autostart registry entry exists.
func IsEnabled() (bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, regKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false, fmt.Errorf("opening registry key: %w", err)
	}
	defer key.Close()

	_, _, err = key.GetStringValue(regValue)
	if err == registry.ErrNotExist {
		return false, nil
	}
	return err == nil, err
}

// Enable adds the executable to the Windows startup registry.
func Enable() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("getting executable path: %w", err)
	}
	key, err := registry.OpenKey(registry.CURRENT_USER, regKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening registry key: %w", err)
	}
	defer key.Close()

	if err := key.SetStringValue(regValue, exePath); err != nil {
		return fmt.Errorf("setting registry value: %w", err)
	}
	return nil
}

// Disable removes the autostart registry entry.
func Disable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, regKeyPath, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("opening registry key: %w", err)
	}
	defer key.Close()

	err = key.DeleteValue(regValue)
	if err == registry.ErrNotExist {
		return nil
	}
	return err
}
