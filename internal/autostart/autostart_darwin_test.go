//go:build darwin

package autostart

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"testing"
)

func TestEnableConfiguresStartVisibility(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := Enable(true); err != nil {
		t.Fatalf("Enable hidden: %v", err)
	}
	plist := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	data, err := os.ReadFile(plist)
	if err != nil {
		t.Fatalf("reading launch agent: %v", err)
	}
	if !bytes.Contains(data, []byte("<string>--hidden</string>")) {
		t.Fatal("launch agent does not start the app hidden")
	}
	if err := xml.Unmarshal(data, new(any)); err != nil {
		t.Fatalf("launch agent is not valid XML: %v", err)
	}

	if err := Enable(false); err != nil {
		t.Fatalf("Enable visible: %v", err)
	}
	data, err = os.ReadFile(plist)
	if err != nil {
		t.Fatalf("reading updated launch agent: %v", err)
	}
	if bytes.Contains(data, []byte("--hidden")) {
		t.Fatal("launch agent still starts the app hidden")
	}
	if err := xml.Unmarshal(data, new(any)); err != nil {
		t.Fatalf("updated launch agent is not valid XML: %v", err)
	}
}
