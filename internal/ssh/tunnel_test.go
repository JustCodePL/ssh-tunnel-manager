package ssh

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"ssh-tunnel-manager/internal/config"
)

func TestBuildSSHConfig_MissingKey(t *testing.T) {
	tun := &Tunnel{
		Config: config.TunnelConfig{
			KeyPath: "/nonexistent/key",
			User:    "test",
		},
	}
	_, err := tun.buildSSHConfig()
	if err == nil {
		t.Fatal("expected error for missing key file")
	}
}

func TestBuildSSHConfig_InvalidKey(t *testing.T) {
	tmp := filepath.Join(t.TempDir(), "badkey")
	if err := os.WriteFile(tmp, []byte("not a real key"), 0o600); err != nil {
		t.Fatal(err)
	}
	tun := &Tunnel{
		Config: config.TunnelConfig{
			KeyPath: tmp,
			User:    "test",
		},
	}
	_, err := tun.buildSSHConfig()
	if err == nil {
		t.Fatal("expected error for invalid key")
	}
}

func TestBuildSSHConfig_ValidKey(t *testing.T) {
	keyPath := filepath.Join(t.TempDir(), "testkey")
	sshKeygen, err := exec.LookPath("ssh-keygen")
	if err != nil {
		t.Skip("ssh-keygen not available in PATH")
	}
	out, err := exec.Command(sshKeygen, "-t", "ed25519", "-f", keyPath, "-N", "", "-q").CombinedOutput()
	if err != nil {
		t.Fatalf("ssh-keygen failed: %v\n%s", err, out)
	}

	tun := &Tunnel{
		Config: config.TunnelConfig{
			KeyPath: keyPath,
			User:    "test",
		},
	}
	cfg, err := tun.buildSSHConfig()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.User != "test" {
		t.Fatalf("expected user %q, got %q", "test", cfg.User)
	}
}

func TestCheckPortConflicts_Available(t *testing.T) {
	tun := &Tunnel{
		Config: config.TunnelConfig{
			Name: "test",
			PortForwards: []config.PortForward{
				{LocalPort: 0}, // port 0 always available
			},
		},
	}
	// Port 0 means the OS picks a free port, so Listen will succeed
	// Use a high ephemeral port that's very likely free
	tun.Config.PortForwards[0].LocalPort = 19876
	if err := tun.CheckPortConflicts(); err != nil {
		t.Fatalf("expected no conflict, got: %v", err)
	}
}

func TestCheckPortConflicts_InUse(t *testing.T) {
	// Bind a port to create a conflict
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	port := ln.Addr().(*net.TCPAddr).Port

	tun := &Tunnel{
		Config: config.TunnelConfig{
			Name: "test",
			PortForwards: []config.PortForward{
				{LocalPort: port, RemoteHost: "127.0.0.1", RemotePort: 5432},
			},
		},
	}
	err = tun.CheckPortConflicts()
	if err == nil {
		t.Fatal("expected port conflict error")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConnect_ContextCancellation(t *testing.T) {
	tun := &Tunnel{
		Config: config.TunnelConfig{
			Name:    "cancel-test",
			Host:    "192.0.2.1", // RFC 5737 TEST-NET, will timeout
			Port:    22,
			User:    "test",
			KeyPath: filepath.Join(t.TempDir(), "nokey"),
		},
	}
	// Write a dummy key so buildSSHConfig doesn't fail on missing file
	os.WriteFile(tun.Config.KeyPath, []byte("not a real key"), 0o600)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := tun.Connect(ctx)
	if err == nil {
		t.Fatal("expected error from connect")
	}
}
