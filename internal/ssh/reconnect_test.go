package ssh

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"ssh-tunnel-manager/internal/config"
	"ssh-tunnel-manager/internal/e2efixture"
)

// A server closing the connection must not leave the tunnel "connected" with a
// dead client until the next keep-alive.
func TestServerDropReconnectsAtOnceAndCommandsWork(t *testing.T) {
	setup, err := e2efixture.PrepareHome(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	server, err := e2efixture.New(setup.FilesDir, setup.Signer.PublicKey())
	if err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	connected := 0
	m := NewManager(func(event StatusEvent) {
		if event.Status == config.StatusConnected {
			mu.Lock()
			connected++
			mu.Unlock()
		}
	}, nil)
	m.WithHostKeyCheck(trustAnyHostKey)
	connections := func() int {
		mu.Lock()
		defer mu.Unlock()
		return connected
	}
	cfg := config.TunnelConfig{ID: "demo", Name: "demo", Host: "127.0.0.1", Port: server.Port, User: "demo", KeyPath: setup.KeyPath}
	if err := m.Connect(cfg); err != nil {
		t.Fatal(err)
	}
	defer m.DisconnectAll()

	waitUntil(t, 10*time.Second, func() bool { return connections() == 1 })
	if out, err := m.RunCommand(context.Background(), "demo", "df -P -B1"); err != nil || !strings.Contains(out, "/data") {
		t.Fatalf("before the drop: out=%q err=%v", out, err)
	}

	if server.DropConnections() != 1 {
		t.Fatal("expected one connection to drop")
	}
	// Far below the keep-alive interval.
	waitUntil(t, 10*time.Second, func() bool { return connections() == 2 })
	if out, err := m.RunCommand(context.Background(), "demo", "df -P -B1"); err != nil || !strings.Contains(out, "/data") {
		t.Fatalf("after the drop: out=%q err=%v", out, err)
	}
}

func waitUntil(t *testing.T, limit time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(limit)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition not met in time")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
