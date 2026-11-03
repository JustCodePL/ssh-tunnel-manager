package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"net"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"

	"ssh-tunnel-manager/internal/config"
)

func startInnerServer(t *testing.T) (net.Conn, <-chan string) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	users := make(chan string, 1)
	serverConfig := &ssh.ServerConfig{NoClientAuth: true}
	serverConfig.AddHostKey(signer)
	// A real socket, because both SSH peers write first and net.Pipe is unbuffered.
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() {
		server, err := listener.Accept()
		if err != nil {
			return
		}
		conn, chans, reqs, err := ssh.NewServerConn(server, serverConfig)
		if err != nil {
			return
		}
		defer conn.Close()
		users <- conn.User()
		go ssh.DiscardRequests(reqs)
		for ch := range chans {
			_ = ch.Reject(ssh.Prohibited, "test server")
		}
	}()
	client, err := net.Dial("tcp", listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return client, users
}

func TestDialViaRunsHandshakeOverTheTunnel(t *testing.T) {
	conn, users := startInnerServer(t)
	dialed := ""
	tun := &Tunnel{
		Config: config.TunnelConfig{Host: "10.0.0.8", Port: 22, User: "inner", ProxyJump: "must-be-ignored"},
		Via: func(_ context.Context, addr string) (net.Conn, error) {
			dialed = addr
			return conn, nil
		},
	}
	client, err := tun.dial(context.Background(), &ssh.ClientConfig{User: "inner", HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if dialed != "10.0.0.8:22" {
		t.Fatalf("dialed %q", dialed)
	}
	if got := <-users; got != "inner" {
		t.Fatalf("user = %q", got)
	}
}

func TestDialViaTimesOutOnSilentPeer(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	tun := &Tunnel{
		Config: config.TunnelConfig{Host: "10.0.0.8", Port: 22},
		Via:    func(context.Context, string) (net.Conn, error) { return client, nil },
	}
	start := time.Now()
	_, err := tun.dial(context.Background(), &ssh.ClientConfig{HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 100 * time.Millisecond})
	if err == nil {
		t.Fatal("expected a timeout")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("took %v", time.Since(start))
	}
}

func TestDialViaReportsDialError(t *testing.T) {
	want := errors.New("refused")
	tun := &Tunnel{
		Config: config.TunnelConfig{Host: "10.0.0.8", Port: 22},
		Via:    func(context.Context, string) (net.Conn, error) { return nil, want },
	}
	if _, err := tun.dial(context.Background(), &ssh.ClientConfig{HostKeyCallback: ssh.InsecureIgnoreHostKey()}); !errors.Is(err, want) {
		t.Fatalf("err = %v", err)
	}
}
