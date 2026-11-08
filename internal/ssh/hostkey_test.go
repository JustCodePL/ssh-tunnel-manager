package ssh

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/e2efixture"
)

func ed25519HostKey(t *testing.T) gossh.PublicKey {
	t.Helper()
	public, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	key, err := gossh.NewPublicKey(public)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestCheckKnownHosts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "known_hosts")
	known := ed25519HostKey(t)
	line := knownhosts.Line([]string{knownhosts.Normalize("a.example:22"), knownhosts.Normalize("b.example:2222")}, known) + "\n"
	if got := CheckKnownHosts(path, "a.example:22", known); got != KnownHostsUnknown {
		t.Errorf("missing file: %v", got)
	}
	if err := os.WriteFile(path, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	otherType, err := gossh.NewPublicKey(&private.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		address string
		key     gossh.PublicKey
		want    KnownHostsVerdict
	}{
		{"match", "a.example:22", known, KnownHostsMatch},
		{"match on a port", "b.example:2222", known, KnownHostsMatch},
		{"other host", "c.example:22", known, KnownHostsUnknown},
		{"other port", "a.example:2222", known, KnownHostsUnknown},
		{"changed key", "a.example:22", ed25519HostKey(t), KnownHostsMismatch},
		{"other key type", "a.example:22", otherType, KnownHostsUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CheckKnownHosts(path, c.address, c.key); got != c.want {
				t.Errorf("got %v, want %v", got, c.want)
			}
		})
	}

	revoked := "@revoked " + knownhosts.Line([]string{knownhosts.Normalize("a.example:22")}, known) + "\n"
	if err := os.WriteFile(path, []byte(revoked), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := CheckKnownHosts(path, "a.example:22", known); got != KnownHostsRevoked {
		t.Errorf("revoked: %v", got)
	}
}

// keyTunnel returns a tunnel that logs in with the fixture's key, so the host
// key check is the only thing that can stop it.
func keyTunnel(t *testing.T, check HostKeyFunc) (*Tunnel, *e2efixture.Server) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("SSH_AUTH_SOCK", "")
	setup, err := e2efixture.PrepareHome(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server, err := e2efixture.New(setup.FilesDir, setup.Signer.PublicKey())
	if err != nil {
		t.Fatal(err)
	}
	return &Tunnel{
		Config:       config.TunnelConfig{ID: "demo", Name: "demo", Host: "127.0.0.1", Port: server.Port, User: "demo", KeyPath: setup.KeyPath},
		CheckHostKey: check,
	}, server
}

// Every way of reaching a server has to go through the check.
func TestHostKeyRefusalStopsEveryConnectionPath(t *testing.T) {
	paths := map[string]func(t *testing.T, tun *Tunnel, port int){
		"direct": func(*testing.T, *Tunnel, int) {},
		"proxy jump": func(_ *testing.T, tun *Tunnel, port int) {
			tun.Config.ProxyJump = fmt.Sprintf("127.0.0.1:%d", port)
		},
		"proxy command": func(t *testing.T, tun *Tunnel, port int) {
			if runtime.GOOS == "windows" {
				t.Skip("needs a Unix shell")
			}
			if _, err := exec.LookPath("nc"); err != nil {
				t.Skip("needs nc")
			}
			tun.Config.ProxyCommand = fmt.Sprintf("nc 127.0.0.1 %d", port)
		},
		"through a tunnel": func(_ *testing.T, tun *Tunnel, port int) {
			tun.Via = func(context.Context, string) (net.Conn, error) {
				return net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
			}
		},
	}
	for name, configure := range paths {
		t.Run(name, func(t *testing.T) {
			refused := errors.New("the host key changed")
			var addresses []string
			tun, server := keyTunnel(t, func(tunnelID, address string, _ gossh.PublicKey) error {
				if tunnelID != "demo" {
					t.Errorf("tunnel = %q", tunnelID)
				}
				addresses = append(addresses, address)
				return refused
			})
			configure(t, tun, server.Port)
			clientConfig, err := tun.buildSSHConfig()
			if err != nil {
				t.Fatal(err)
			}
			client, err := tun.dial(context.Background(), clientConfig)
			if err == nil {
				client.Close()
				t.Fatal("connected despite the refusal")
			}
			if !isHostKeyRefusal(err) {
				t.Errorf("err = %v", err)
			}
			if want := fmt.Sprintf("127.0.0.1:%d", server.Port); len(addresses) != 1 || addresses[0] != want {
				t.Errorf("checked %v, want %s", addresses, want)
			}
		})
	}
}

// The jump host passing does not excuse the server behind it.
func TestProxyJumpChecksTheTargetToo(t *testing.T) {
	refused := errors.New("the host key changed")
	checks := 0
	tun, server := keyTunnel(t, func(string, string, gossh.PublicKey) error {
		checks++
		if checks == 1 {
			return nil
		}
		return refused
	})
	tun.Config.ProxyJump = fmt.Sprintf("127.0.0.1:%d", server.Port)
	clientConfig, err := tun.buildSSHConfig()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tun.dial(context.Background(), clientConfig); !isHostKeyRefusal(err) {
		t.Fatalf("err = %v", err)
	}
	if checks != 2 {
		t.Errorf("checks = %d", checks)
	}
}

// A question to the user must not run into the handshake timeout.
func TestDialViaDoesNotCountTheHostKeyCheck(t *testing.T) {
	tun, server := keyTunnel(t, func(string, string, gossh.PublicKey) error {
		time.Sleep(300 * time.Millisecond)
		return nil
	})
	tun.Via = func(context.Context, string) (net.Conn, error) {
		return net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", server.Port))
	}
	clientConfig, err := tun.buildSSHConfig()
	if err != nil {
		t.Fatal(err)
	}
	clientConfig.Timeout = 150 * time.Millisecond
	client, err := tun.dial(context.Background(), clientConfig)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
}
