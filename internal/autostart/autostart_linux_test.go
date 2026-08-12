//go:build linux

package autostart

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestEnableDisable(t *testing.T) {
	// Use a temp dir as XDG_CONFIG_HOME so we don't touch the real autostart
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	enabled, err := IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled: %v", err)
	}
	if enabled {
		t.Fatal("expected autostart to be disabled initially")
	}

	if err := Enable(true); err != nil {
		t.Fatalf("Enable: %v", err)
	}

	enabled, err = IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled after enable: %v", err)
	}
	if !enabled {
		t.Fatal("expected autostart to be enabled after Enable()")
	}

	// Verify the .desktop file was created
	desktopPath := filepath.Join(tmpDir, "autostart", "ssh-tunnel-manager.desktop")
	data, err := os.ReadFile(desktopPath)
	if err != nil {
		t.Fatalf("reading desktop file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("desktop file is empty")
	}
	if !bytes.Contains(data, []byte(" --hidden")) {
		t.Fatal("desktop file does not start the app hidden")
	}

	if err := Enable(false); err != nil {
		t.Fatalf("Enable without minimized start: %v", err)
	}
	data, err = os.ReadFile(desktopPath)
	if err != nil {
		t.Fatalf("reading updated desktop file: %v", err)
	}
	if bytes.Contains(data, []byte("--hidden")) {
		t.Fatal("desktop file still starts the app hidden")
	}

	if err := Disable(); err != nil {
		t.Fatalf("Disable: %v", err)
	}

	enabled, err = IsEnabled()
	if err != nil {
		t.Fatalf("IsEnabled after disable: %v", err)
	}
	if enabled {
		t.Fatal("expected autostart to be disabled after Disable()")
	}
}

func TestDisable_NotEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmpDir)

	// Should not error if already disabled
	if err := Disable(); err != nil {
		t.Fatalf("Disable when not enabled: %v", err)
	}
}
