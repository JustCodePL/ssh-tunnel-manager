package ssh

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/dns"
)

func TestEffectiveHostHeader(t *testing.T) {
	tests := []struct {
		name string
		pf   config.PortForward
		want string
	}{
		{
			name: "off by default ignores explicit header",
			pf:   config.PortForward{Portless: true, RemoteHost: "10.0.0.1", HostHeader: "custom.example.com"},
			want: "",
		},
		{
			name: "off by default ignores FQDN auto-rule",
			pf:   config.PortForward{Portless: true, RemoteHost: "dev.mix-dev.com"},
			want: "",
		},
		{
			name: "on + explicit overrides everything",
			pf:   config.PortForward{Portless: true, RemoteHost: "10.0.0.1", HostHeader: "custom.example.com", HostHeaderOn: true},
			want: "custom.example.com",
		},
		{
			name: "on + explicit works even without portless",
			pf:   config.PortForward{Portless: false, RemoteHost: "10.0.0.1", HostHeader: "custom.example.com", HostHeaderOn: true},
			want: "custom.example.com",
		},
		{
			name: "on + portless + FQDN remote auto-rewrites",
			pf:   config.PortForward{Portless: true, RemoteHost: "dev.mix-dev.com", HostHeaderOn: true},
			want: "dev.mix-dev.com",
		},
		{
			name: "on + portless + IPv4 remote keeps raw TCP",
			pf:   config.PortForward{Portless: true, RemoteHost: "10.0.0.1", HostHeaderOn: true},
			want: "",
		},
		{
			name: "on + portless + localhost keeps raw TCP",
			pf:   config.PortForward{Portless: true, RemoteHost: "localhost", HostHeaderOn: true},
			want: "",
		},
		{
			name: "on + portless + single-label remote keeps raw TCP",
			pf:   config.PortForward{Portless: true, RemoteHost: "internal-db", HostHeaderOn: true},
			want: "",
		},
		{
			name: "on + non-portless + FQDN remote stays raw TCP",
			pf:   config.PortForward{Portless: false, RemoteHost: "dev.mix-dev.com", HostHeaderOn: true},
			want: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := effectiveHostHeader(tc.pf)
			if got != tc.want {
				t.Errorf("effectiveHostHeader(%+v) = %q, want %q", tc.pf, got, tc.want)
			}
		})
	}
}

func TestIsForwardingAdministrativelyProhibited(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", err: nil, want: false},
		{name: "OpenSSH wording", err: errors.New("channel 0: open failed: administratively prohibited: open failed"), want: true},
		{name: "Go SSH wording", err: errors.New("ssh: rejected: Administratively Prohibited (open failed)"), want: true},
		{name: "destination refused", err: errors.New("connect failed: Connection refused"), want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isForwardingAdministrativelyProhibited(tc.err); got != tc.want {
				t.Fatalf("isForwardingAdministrativelyProhibited(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestReportForwardingBlockedOnce(t *testing.T) {
	var events []PortForwardError
	var logs []string
	tun := &Tunnel{
		Config: config.TunnelConfig{ID: "nas-id"},
		OnForwardError: func(err PortForwardError) {
			events = append(events, err)
		},
		LogFunc: func(_ string, msg string) {
			logs = append(logs, msg)
		},
	}
	pf := config.PortForward{
		RemoteHost: "127.0.0.1",
		RemotePort: 8080,
		Portless:   true,
		Domain:     "nas",
		ExposePort: 8080,
	}
	var once sync.Once
	err := errors.New("ssh: rejected: administratively prohibited (open failed)")

	tun.reportForwardingBlocked(&once, pf, "127.0.1.17:8080", err)
	tun.reportForwardingBlocked(&once, pf, "127.0.1.17:8080", err)

	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if len(logs) != 1 {
		t.Fatalf("logs = %d, want 1", len(logs))
	}
	if events[0].TunnelID != "nas-id" {
		t.Fatalf("TunnelID = %q, want nas-id", events[0].TunnelID)
	}
	if events[0].LocalAddress != "nas.ssh-local:8080" {
		t.Fatalf("LocalAddress = %q, want nas.ssh-local:8080", events[0].LocalAddress)
	}
	if events[0].RemoteAddress != "127.0.0.1:8080" {
		t.Fatalf("RemoteAddress = %q, want 127.0.0.1:8080", events[0].RemoteAddress)
	}
	if !strings.Contains(events[0].Message, "AllowTcpForwarding") {
		t.Fatalf("Message = %q, want AllowTcpForwarding guidance", events[0].Message)
	}
}

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

func TestFallbackLocalPortUsesRemotePortWhenUnset(t *testing.T) {
	pf := config.PortForward{LocalPort: 0, RemotePort: 6379}
	if got := fallbackLocalPort(pf); got != 6379 {
		t.Fatalf("fallbackLocalPort() = %d, want 6379", got)
	}

	pf.LocalPort = 16379
	if got := fallbackLocalPort(pf); got != 16379 {
		t.Fatalf("fallbackLocalPort() = %d, want 16379", got)
	}
}

func TestCheckPortConflicts_PortlessFallbackUsesLocalPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	tun := &Tunnel{
		Config: config.TunnelConfig{
			Name: "degraded",
			PortForwards: []config.PortForward{{
				LocalPort:  port,
				RemotePort: 5432,
				Portless:   true,
				Domain:     "db",
			}},
		},
		DNSRegistry: nil,
	}
	if err := tun.CheckPortConflicts(); err == nil {
		t.Fatal("expected degraded Portless forward to check its local port")
	}
}

func TestCheckPortConflicts_ActivePortlessSkipsLocalPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	tun := &Tunnel{
		Config: config.TunnelConfig{
			Name: "active-portless",
			PortForwards: []config.PortForward{{
				LocalPort:  port,
				RemotePort: 5432,
				Portless:   true,
				Domain:     "db",
			}},
		},
		DNSRegistry: dns.NewRegistry(),
	}
	if err := tun.CheckPortConflicts(); err != nil {
		t.Fatalf("active Portless should ignore ordinary local port: %v", err)
	}
}

func TestForwardPort_PortlessWithoutDNSFallsBackToLocalListener(t *testing.T) {
	probe, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := probe.Addr().(*net.TCPAddr).Port
	_ = probe.Close()

	logCh := make(chan string, 1)
	tun := &Tunnel{
		Config: config.TunnelConfig{ID: "db-id", Name: "db"},
		LogFunc: func(_ string, msg string) {
			select {
			case logCh <- msg:
			default:
			}
		},
	}
	pf := config.PortForward{
		LocalPort:  port,
		RemoteHost: "127.0.0.1",
		RemotePort: 5432,
		Portless:   true,
		Domain:     "db",
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		tun.forwardPort(ctx, nil, pf)
		close(done)
	}()

	select {
	case msg := <-logCh:
		if !strings.Contains(msg, "fell back to 127.0.0.1") {
			t.Fatalf("fallback log = %q", msg)
		}
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("timed out waiting for local-port fallback")
	}

	conflict, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err == nil {
		_ = conflict.Close()
		cancel()
		t.Fatal("fallback did not bind the configured local port")
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("fallback listener did not stop after cancellation")
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
