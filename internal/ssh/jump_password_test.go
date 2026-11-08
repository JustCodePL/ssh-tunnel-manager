package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/e2efixture"
)

// jumpServer is a bastion that records passwords and relays direct-tcpip
// channels to the real target.
type jumpServer struct {
	mu        sync.Mutex
	passwords []string
	port      int
}

func (j *jumpServer) received() []string {
	j.mu.Lock()
	defer j.mu.Unlock()
	return append([]string(nil), j.passwords...)
}

func startJumpServer(t *testing.T, acceptKeys bool) *jumpServer {
	t.Helper()
	_, hostKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatal(err)
	}
	j := &jumpServer{}
	serverConfig := &gossh.ServerConfig{
		PasswordCallback: func(_ gossh.ConnMetadata, password []byte) (*gossh.Permissions, error) {
			j.mu.Lock()
			j.passwords = append(j.passwords, string(password))
			j.mu.Unlock()
			return nil, fmt.Errorf("wrong password")
		},
	}
	if acceptKeys {
		serverConfig.PublicKeyCallback = func(gossh.ConnMetadata, gossh.PublicKey) (*gossh.Permissions, error) { return nil, nil }
	}
	serverConfig.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	j.port = listener.Addr().(*net.TCPAddr).Port
	go func() {
		for {
			raw, err := listener.Accept()
			if err != nil {
				return
			}
			go serveJump(raw, serverConfig)
		}
	}()
	return j
}

func serveJump(raw net.Conn, serverConfig *gossh.ServerConfig) {
	conn, chans, reqs, err := gossh.NewServerConn(raw, serverConfig)
	if err != nil {
		raw.Close()
		return
	}
	defer conn.Close()
	go gossh.DiscardRequests(reqs)
	for newChannel := range chans {
		var target struct {
			Host       string
			Port       uint32
			OriginHost string
			OriginPort uint32
		}
		if newChannel.ChannelType() != "direct-tcpip" || gossh.Unmarshal(newChannel.ExtraData(), &target) != nil {
			_ = newChannel.Reject(gossh.UnknownChannelType, "unsupported")
			continue
		}
		upstream, err := net.Dial("tcp", net.JoinHostPort(target.Host, fmt.Sprint(target.Port)))
		if err != nil {
			_ = newChannel.Reject(gossh.ConnectionFailed, err.Error())
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			upstream.Close()
			continue
		}
		go gossh.DiscardRequests(requests)
		go func() {
			_, _ = io.Copy(upstream, channel)
			upstream.Close()
		}()
		go func() {
			_, _ = io.Copy(channel, upstream)
			channel.Close()
		}()
	}
}

// passwordTunnelThrough returns a tunnel to a target that only takes the saved
// password, reached through the jump host.
func passwordTunnelThrough(t *testing.T, jump *jumpServer) *Tunnel {
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
		Config: config.TunnelConfig{
			ID: "demo", Name: "demo", Host: "127.0.0.1", Port: server.Port, User: "demo",
			KeyPath: setup.KeyPath, ProxyJump: fmt.Sprintf("127.0.0.1:%d", jump.port),
		},
		GetLogin:     staticLogin(&PasswordLogin{Username: e2efixture.PasswordUser, Password: e2efixture.Password}),
		CheckHostKey: trustAnyHostKey,
	}
}

func dialTunnel(t *testing.T, tun *Tunnel) (*gossh.Client, error) {
	t.Helper()
	clientConfig, err := tun.buildSSHConfig()
	if err != nil {
		t.Fatal(err)
	}
	return tun.dial(context.Background(), clientConfig)
}

func TestJumpHostIsNeverOfferedTheSavedPassword(t *testing.T) {
	jump := startJumpServer(t, false)
	tun := passwordTunnelThrough(t, jump)
	if client, err := dialTunnel(t, tun); err == nil {
		client.Close()
		t.Fatal("logged in to a jump host that only takes passwords")
	}
	if got := jump.received(); len(got) != 0 {
		t.Errorf("the jump host received %d password(s)", len(got))
	}
}

func TestSavedPasswordStillReachesTheServerBehindAJumpHost(t *testing.T) {
	jump := startJumpServer(t, true)
	tun := passwordTunnelThrough(t, jump)
	client, err := dialTunnel(t, tun)
	if err != nil {
		t.Fatal(err)
	}
	client.Close()
	if got := jump.received(); len(got) != 0 {
		t.Errorf("the jump host received %d password(s)", len(got))
	}
}

func TestJumpHostWithoutAKeyExplainsWhy(t *testing.T) {
	jump := startJumpServer(t, false)
	tun := passwordTunnelThrough(t, jump)
	tun.Config.KeyPath = ""
	if _, err := dialTunnel(t, tun); err == nil {
		t.Fatal("expected an error")
	} else if !strings.Contains(err.Error(), "needs a key or the agent") {
		t.Errorf("err = %v", err)
	}
	if got := jump.received(); len(got) != 0 {
		t.Errorf("the jump host received %d password(s)", len(got))
	}
}
