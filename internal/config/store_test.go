package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"ssh-tunnel-manager/internal/sshconfig"
)

func tempSSHConfigPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "config")
}

func writeSSHConfig(t *testing.T, path, content string) {
	t.Helper()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
}

func TestNewStoreWithPath_MissingFile(t *testing.T) {
	s, err := NewStoreWithPath(tempSSHConfigPath(t))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(s.GetTunnels()) != 0 {
		t.Fatalf("expected 0 tunnels, got %d", len(s.GetTunnels()))
	}
}

func TestNewStoreWithPath_ExistingHosts(t *testing.T) {
	path := tempSSHConfigPath(t)
	writeSSHConfig(t, path, `Host alpha
    HostName alpha.example.com
    User admin

Host beta
    HostName beta.example.com
    User deploy
    LocalForward 8080 127.0.0.1:80
`)

	s, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	tunnels := s.GetTunnels()
	if len(tunnels) != 2 {
		t.Fatalf("expected 2 tunnels, got %d", len(tunnels))
	}
	if tunnels[0].Name != "alpha" {
		t.Errorf("first name = %q, want %q", tunnels[0].Name, "alpha")
	}
	if tunnels[1].Name != "beta" {
		t.Errorf("second name = %q, want %q", tunnels[1].Name, "beta")
	}
}

func TestRoundTrip(t *testing.T) {
	path := tempSSHConfigPath(t)
	s, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	tunnel := TunnelConfig{
		ID:   "test-tunnel",
		Name: "test-tunnel",
		Host: "example.com",
		Port: 22,
		User: "admin",
		PortForwards: []PortForward{
			{LocalPort: 8080, RemoteHost: "127.0.0.1", RemotePort: 80},
		},
	}
	if err := s.AddTunnel(tunnel); err != nil {
		t.Fatalf("add tunnel: %v", err)
	}

	// Load from disk in a new store
	s2, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	tunnels := s2.GetTunnels()
	if len(tunnels) != 1 {
		t.Fatalf("expected 1 tunnel, got %d", len(tunnels))
	}
	if tunnels[0].Name != "test-tunnel" {
		t.Fatalf("expected name %q, got %q", "test-tunnel", tunnels[0].Name)
	}
	if len(tunnels[0].PortForwards) != 1 {
		t.Fatalf("expected 1 port forward, got %d", len(tunnels[0].PortForwards))
	}
}

func TestCRUD(t *testing.T) {
	s, err := NewStoreWithPath(tempSSHConfigPath(t))
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	// Add
	t1 := TunnelConfig{ID: "one", Name: "one", Host: "h1", Port: 22, User: "u"}
	t2 := TunnelConfig{ID: "two", Name: "two", Host: "h2", Port: 22, User: "u"}
	if err := s.AddTunnel(t1); err != nil {
		t.Fatalf("add t1: %v", err)
	}
	if err := s.AddTunnel(t2); err != nil {
		t.Fatalf("add t2: %v", err)
	}
	if len(s.GetTunnels()) != 2 {
		t.Fatalf("expected 2 tunnels")
	}

	// Get
	got, ok := s.GetTunnel("one")
	if !ok {
		t.Fatal("one not found")
	}
	if got.Name != "one" {
		t.Fatalf("expected name %q, got %q", "one", got.Name)
	}

	_, ok = s.GetTunnel("nonexistent")
	if ok {
		t.Fatal("expected nonexistent tunnel to not be found")
	}

	// Update
	t1.Host = "updated.com"
	if err := s.UpdateTunnel(t1); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ = s.GetTunnel("one")
	if got.Host != "updated.com" {
		t.Fatalf("expected updated host, got %q", got.Host)
	}

	// Update nonexistent
	if err := s.UpdateTunnel(TunnelConfig{ID: "nope", Name: "nope"}); err == nil {
		t.Fatal("expected error updating nonexistent tunnel")
	}

	// Delete
	if err := s.DeleteTunnel("one"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(s.GetTunnels()) != 1 {
		t.Fatalf("expected 1 tunnel after delete")
	}

	// Delete nonexistent
	if err := s.DeleteTunnel("one"); err == nil {
		t.Fatal("expected error deleting nonexistent tunnel")
	}
}

func TestAddDuplicate(t *testing.T) {
	s, err := NewStoreWithPath(tempSSHConfigPath(t))
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	t1 := TunnelConfig{ID: "dup", Name: "dup", Host: "h1", Port: 22, User: "u"}
	if err := s.AddTunnel(t1); err != nil {
		t.Fatalf("first add: %v", err)
	}
	if err := s.AddTunnel(t1); err == nil {
		t.Fatal("expected error adding duplicate tunnel")
	}
}

func TestPreserveExistingContent(t *testing.T) {
	path := tempSSHConfigPath(t)
	writeSSHConfig(t, path, `# Global settings
ServerAliveInterval 60

Host *
    AddKeysToAgent yes

Host existing
    HostName existing.example.com
    User admin
`)

	s, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	// Add a new tunnel
	if err := s.AddTunnel(TunnelConfig{
		ID: "new-host", Name: "new-host",
		Host: "new.example.com", Port: 22, User: "deploy",
	}); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Read raw file to verify existing content preserved
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "ServerAliveInterval 60") {
		t.Error("global settings should be preserved")
	}
	if !strings.Contains(content, "Host *") {
		t.Error("wildcard host should be preserved")
	}
	if !strings.Contains(content, "Host existing") {
		t.Error("existing host should be preserved")
	}
	if !strings.Contains(content, "Host new-host") {
		t.Error("new host should be present")
	}
}

func TestRenameTunnel(t *testing.T) {
	path := tempSSHConfigPath(t)
	s, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	if err := s.AddTunnel(TunnelConfig{
		ID: "old-name", Name: "old-name",
		Host: "example.com", Port: 22, User: "admin",
	}); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Rename: ID stays as old-name (that's how we find it), Name changes
	updated := TunnelConfig{
		ID: "old-name", Name: "new-name",
		Host: "example.com", Port: 22, User: "admin",
	}
	if err := s.UpdateTunnel(updated); err != nil {
		t.Fatalf("update (rename): %v", err)
	}

	// After rename, ID should be updated to new-name
	_, ok := s.GetTunnel("old-name")
	if ok {
		t.Error("old-name should no longer exist")
	}
	got, ok := s.GetTunnel("new-name")
	if !ok {
		t.Fatal("new-name should exist")
	}
	if got.Name != "new-name" {
		t.Errorf("Name = %q, want %q", got.Name, "new-name")
	}

	// Verify file content
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	content := string(data)
	if strings.Contains(content, "Host old-name") {
		t.Error("old Host block should be removed from file")
	}
	if !strings.Contains(content, "Host new-name") {
		t.Error("new Host block should be in file")
	}
}

func TestUpdateTunnelPersistsPortlessForwardWithAutomaticLocalPort(t *testing.T) {
	path := tempSSHConfigPath(t)
	store, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	if err := store.AddTunnel(TunnelConfig{
		ID: "dev", Name: "dev", Host: "dev.example.com", Port: 22, User: "deploy",
		PortForwards: []PortForward{{
			LocalPort: 5432, RemoteHost: "127.0.0.1", RemotePort: 5432, Description: "database",
		}},
	}); err != nil {
		t.Fatalf("add: %v", err)
	}

	updated, ok := store.GetTunnel("dev")
	if !ok {
		t.Fatal("dev tunnel not found")
	}
	updated.PortForwards = append(updated.PortForwards, PortForward{
		LocalPort:   0,
		RemoteHost:  "127.0.0.1",
		RemotePort:  6379,
		Description: "cache",
		Portless:    true,
		Domain:      "redis.dev",
	})
	if err := store.UpdateTunnel(updated); err != nil {
		t.Fatalf("update: %v", err)
	}

	reloaded, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatalf("reload store: %v", err)
	}
	got, ok := reloaded.GetTunnel("dev")
	if !ok {
		t.Fatal("reloaded dev tunnel not found")
	}
	if len(got.PortForwards) != 2 {
		t.Fatalf("reloaded port forwards = %d, want 2", len(got.PortForwards))
	}
	pf := got.PortForwards[1]
	if !pf.Portless || pf.Domain != "redis.dev" || pf.RemotePort != 6379 {
		t.Fatalf("reloaded portless forward = %+v", pf)
	}
	if pf.LocalPort != pf.RemotePort {
		t.Fatalf("reloaded local port = %d, want remote-port fallback %d", pf.LocalPort, pf.RemotePort)
	}
}

func TestConcurrentAccess(t *testing.T) {
	s, err := NewStoreWithPath(tempSSHConfigPath(t))
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			tunnel := TunnelConfig{
				ID:   fmt.Sprintf("tunnel-%d", n),
				Name: fmt.Sprintf("tunnel-%d", n),
				Host: "localhost",
				Port: 22,
				User: "user",
			}
			_ = s.AddTunnel(tunnel)
			_ = s.GetTunnels()
			_, _ = s.GetTunnel(tunnel.ID)
		}(i)
	}
	wg.Wait()

	tunnels := s.GetTunnels()
	if len(tunnels) != 50 {
		t.Fatalf("expected 50 tunnels, got %d", len(tunnels))
	}
}

func TestDirCreatedOnAdd(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "nested", "config")
	s, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	if err := s.AddTunnel(TunnelConfig{ID: "x", Name: "x", Host: "x", Port: 22, User: "x"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
}

func TestMultiFileInclude(t *testing.T) {
	dir := t.TempDir()
	confDir := filepath.Join(dir, "config.d")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Write included file
	included := filepath.Join(confDir, "work.conf")
	writeSSHConfig(t, included, `Host work-server
    HostName work.example.com
    User deploy
`)

	// Write main config with Include
	mainConfig := filepath.Join(dir, "config")
	writeSSHConfig(t, mainConfig, `Host main-server
    HostName main.example.com
    User admin

Include `+confDir+`/*.conf
`)

	s, err := NewStoreWithPath(mainConfig)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}
	tunnels := s.GetTunnels()
	if len(tunnels) != 2 {
		t.Fatalf("expected 2 tunnels, got %d", len(tunnels))
	}

	// Verify source files are tracked
	absMain, _ := filepath.Abs(mainConfig)
	absIncluded, _ := filepath.Abs(included)
	if tunnels[0].SourceFile != absMain {
		t.Errorf("tunnel[0].SourceFile = %q, want %q", tunnels[0].SourceFile, absMain)
	}
	if tunnels[1].SourceFile != absIncluded {
		t.Errorf("tunnel[1].SourceFile = %q, want %q", tunnels[1].SourceFile, absIncluded)
	}

	// Update a tunnel in the included file — should write back to included file
	updated := tunnels[1]
	updated.Host = "updated-work.example.com"
	if err := s.UpdateTunnel(updated); err != nil {
		t.Fatalf("update: %v", err)
	}

	// Verify included file was modified
	data, err := os.ReadFile(included)
	if err != nil {
		t.Fatalf("read included: %v", err)
	}
	if !strings.Contains(string(data), "updated-work.example.com") {
		t.Error("included file should contain updated hostname")
	}

	// Verify main config was NOT modified
	mainData, err := os.ReadFile(mainConfig)
	if err != nil {
		t.Fatalf("read main: %v", err)
	}
	if strings.Contains(string(mainData), "updated-work.example.com") {
		t.Error("main config should not contain the updated hostname")
	}

	// Delete from included file
	if err := s.DeleteTunnel("work-server"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(s.GetTunnels()) != 1 {
		t.Fatalf("expected 1 tunnel after delete, got %d", len(s.GetTunnels()))
	}

	// Verify included file had the host removed
	data, err = os.ReadFile(included)
	if err != nil {
		t.Fatalf("read included: %v", err)
	}
	if strings.Contains(string(data), "Host work-server") {
		t.Error("included file should no longer contain work-server")
	}
}

func TestGetIncludedConfigFiles(t *testing.T) {
	dir := t.TempDir()
	confDir := filepath.Join(dir, "config.d")
	if err := os.MkdirAll(confDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	included := filepath.Join(confDir, "work.conf")
	writeSSHConfig(t, included, `Host work-server
    HostName work.example.com
    User deploy
`)

	mainConfig := filepath.Join(dir, "config")
	writeSSHConfig(t, mainConfig, `Host main-server
    HostName main.example.com
    User admin

Include `+confDir+`/*.conf
`)

	s, err := NewStoreWithPath(mainConfig)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	files, err := s.GetIncludedConfigFiles()
	if err != nil {
		t.Fatalf("GetIncludedConfigFiles: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("expected 2 files, got %d", len(files))
	}

	absMain, _ := filepath.Abs(mainConfig)
	absIncluded, _ := filepath.Abs(included)
	if files[0].Path != absMain {
		t.Errorf("files[0].Path = %q, want %q", files[0].Path, absMain)
	}
	if files[1].Path != absIncluded {
		t.Errorf("files[1].Path = %q, want %q", files[1].Path, absIncluded)
	}
	if files[0].Label != sshconfig.CollapseTildePath(absMain) {
		t.Errorf("files[0].Label = %q, want %q", files[0].Label, sshconfig.CollapseTildePath(absMain))
	}
	if files[1].Label != sshconfig.CollapseTildePath(absIncluded) {
		t.Errorf("files[1].Label = %q, want %q", files[1].Label, sshconfig.CollapseTildePath(absIncluded))
	}
}

func TestMetadataRoundTrip(t *testing.T) {
	path := tempSSHConfigPath(t)
	s, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatalf("create store: %v", err)
	}

	if err := s.AddTunnel(TunnelConfig{
		ID: "meta-test", Name: "meta-test",
		Host: "meta.example.com", Port: 22, User: "admin",
		Group: "production", AutoConnect: true,
	}); err != nil {
		t.Fatalf("add: %v", err)
	}

	// Reload and check metadata
	s2, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	got, ok := s2.GetTunnel("meta-test")
	if !ok {
		t.Fatal("meta-test not found")
	}
	if got.Group != "production" {
		t.Errorf("Group = %q, want %q", got.Group, "production")
	}
	if !got.AutoConnect {
		t.Error("AutoConnect = false, want true")
	}
}
