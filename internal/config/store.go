package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"ssh-tunnel-manager/internal/sshconfig"
)

// Store provides thread-safe CRUD for tunnel configs backed by ~/.ssh/config.
type Store struct {
	mu       sync.RWMutex
	tunnels  []TunnelConfig
	filePath string
}

// NewStore creates a Store that reads/writes to the default SSH config
// (~/.ssh/config).
func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home dir: %w", err)
	}
	return NewStoreWithPath(filepath.Join(home, ".ssh", "config"))
}

// NewStoreWithPath creates a Store backed by the given SSH config file path.
func NewStoreWithPath(path string) (*Store, error) {
	s := &Store{filePath: path}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// GetTunnels returns a copy of all tunnel configs.
func (s *Store) GetTunnels() []TunnelConfig {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]TunnelConfig, len(s.tunnels))
	copy(out, s.tunnels)
	return out
}

// GetTunnel returns the tunnel with the given ID (Host alias), or false if not found.
func (s *Store) GetTunnel(id string) (TunnelConfig, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, t := range s.tunnels {
		if t.ID == id {
			return t, true
		}
	}
	return TunnelConfig{}, false
}

// AddTunnel appends a new Host block to the SSH config file.
// If SourceFile is set, the block is written to that file; otherwise to the
// main config.
func (s *Store) AddTunnel(t TunnelConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, existing := range s.tunnels {
		if existing.ID == t.ID {
			return fmt.Errorf("tunnel %q already exists", t.ID)
		}
	}

	targetFile := s.filePath
	if t.SourceFile != "" {
		targetFile = t.SourceFile
	}

	lines, err := sshconfig.ReadLines(targetFile)
	if err != nil {
		return fmt.Errorf("reading ssh config: %w", err)
	}

	block := sshconfig.RenderHostBlock(toHostEntry(t))
	lines = sshconfig.AppendHostBlock(lines, block)

	if err := s.ensureDirFor(targetFile); err != nil {
		return err
	}
	if err := sshconfig.WriteLines(targetFile, lines); err != nil {
		return fmt.Errorf("writing ssh config: %w", err)
	}

	if t.SourceFile == "" {
		t.SourceFile = s.filePath
	}
	t.SourceFileLabel = sshconfig.CollapseTildePath(t.SourceFile)
	s.tunnels = append(s.tunnels, t)
	return nil
}

// UpdateTunnel replaces the Host block for the tunnel with matching ID.
// If the name changed (rename), the old block is removed and a new one is
// written with the new alias. Writes to the tunnel's SourceFile.
func (s *Store) UpdateTunnel(t TunnelConfig) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	for i, existing := range s.tunnels {
		if existing.ID == t.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("tunnel %q not found", t.ID)
	}

	oldTunnel := s.tunnels[idx]
	targetFile := oldTunnel.SourceFile
	if targetFile == "" {
		targetFile = s.filePath
	}

	lines, err := sshconfig.ReadLines(targetFile)
	if err != nil {
		return fmt.Errorf("reading ssh config: %w", err)
	}

	block := sshconfig.RenderHostBlock(toHostEntry(t))

	if oldTunnel.Name != t.Name {
		// Rename: remove old block, append new
		lines, err = sshconfig.RemoveHostBlock(lines, oldTunnel.Name)
		if err != nil {
			return fmt.Errorf("removing old host block %q: %w", oldTunnel.Name, err)
		}
		lines = sshconfig.AppendHostBlock(lines, block)
	} else {
		lines, err = sshconfig.ReplaceHostBlock(lines, t.Name, block)
		if err != nil {
			return fmt.Errorf("replacing host block %q: %w", t.Name, err)
		}
	}

	if err := sshconfig.WriteLines(targetFile, lines); err != nil {
		return fmt.Errorf("writing ssh config: %w", err)
	}

	// Preserve the source file
	t.SourceFile = targetFile
	t.SourceFileLabel = sshconfig.CollapseTildePath(targetFile)
	// Update ID to match new name on rename
	t.ID = t.Name
	s.tunnels[idx] = t
	return nil
}

// DeleteTunnel removes the Host block with the given ID (alias) from its
// source file.
func (s *Store) DeleteTunnel(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	for i, t := range s.tunnels {
		if t.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("tunnel %q not found", id)
	}

	targetFile := s.tunnels[idx].SourceFile
	if targetFile == "" {
		targetFile = s.filePath
	}

	lines, err := sshconfig.ReadLines(targetFile)
	if err != nil {
		return fmt.Errorf("reading ssh config: %w", err)
	}

	lines, err = sshconfig.RemoveHostBlock(lines, s.tunnels[idx].Name)
	if err != nil {
		return fmt.Errorf("removing host block %q: %w", s.tunnels[idx].Name, err)
	}

	if err := sshconfig.WriteLines(targetFile, lines); err != nil {
		return fmt.Errorf("writing ssh config: %w", err)
	}

	s.tunnels = append(s.tunnels[:idx], s.tunnels[idx+1:]...)
	return nil
}

func (s *Store) load() error {
	entries, err := sshconfig.ParseFile(s.filePath)
	if err != nil {
		return fmt.Errorf("loading ssh config %s: %w", s.filePath, err)
	}
	if entries == nil {
		slog.Info("ssh config file not found, starting with empty config", "path", s.filePath)
		s.tunnels = nil
	} else {
		s.tunnels = make([]TunnelConfig, len(entries))
		for i, e := range entries {
			s.tunnels[i] = fromHostEntry(e)
		}
		slog.Info("loaded ssh config", "path", s.filePath, "tunnels", len(s.tunnels))
	}
	return nil
}

func (s *Store) ensureDirFor(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("creating directory %s: %w", dir, err)
	}
	return nil
}

// GetConfigFiles returns a deduplicated, ordered list of all source files
// that contributed tunnel configs (main config + included files).
func (s *Store) GetConfigFiles() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := make(map[string]bool)
	var files []string
	for _, t := range s.tunnels {
		sf := t.SourceFile
		if sf == "" {
			sf = s.filePath
		}
		if !seen[sf] {
			seen[sf] = true
			files = append(files, sf)
		}
	}
	return files
}

// GetIncludedConfigFiles returns the list of files referenced by Include
// directives, beginning with the main config file.
func (s *Store) GetIncludedConfigFiles() ([]ConfigFileInfo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	files, err := sshconfig.IncludedConfigFiles(s.filePath)
	if err != nil {
		return nil, err
	}
	result := make([]ConfigFileInfo, 0, len(files))
	seen := make(map[string]bool)
	for _, file := range files {
		if file == "" || seen[file] {
			continue
		}
		seen[file] = true
		result = append(result, ConfigFileInfo{
			Path:  file,
			Label: sshconfig.CollapseTildePath(file),
		})
	}
	return result, nil
}

func toHostEntry(t TunnelConfig) sshconfig.HostEntry {
	e := sshconfig.HostEntry{
		Alias:        t.Name,
		HostName:     t.Host,
		Port:         t.Port,
		User:         t.User,
		IdentityFile: t.KeyPath,
		ProxyCommand: t.ProxyCommand,
		ProxyJump:    t.ProxyJump,
		Color:        t.Color,
		Group:        t.Group,
		AutoConnect:  t.AutoConnect,
		Pinned:       t.Pinned,
		SourceFile:   t.SourceFile,
	}
	for _, pf := range t.PortForwards {
		e.PortForwards = append(e.PortForwards, sshconfig.PortForwardEntry{
			LocalPort:   pf.LocalPort,
			RemoteHost:  pf.RemoteHost,
			RemotePort:  pf.RemotePort,
			Description: pf.Description,
			Portless:    pf.Portless,
			Domain:      pf.Domain,
			ExposePort:  pf.ExposePort,
		})
	}
	return e
}

func fromHostEntry(e sshconfig.HostEntry) TunnelConfig {
	t := TunnelConfig{
		ID:              e.Alias,
		Name:            e.Alias,
		Host:            e.HostName,
		Port:            e.Port,
		User:            e.User,
		KeyPath:         e.IdentityFile,
		ProxyCommand:    e.ProxyCommand,
		ProxyJump:       e.ProxyJump,
		Color:           e.Color,
		Group:           e.Group,
		AutoConnect:     e.AutoConnect,
		Pinned:          e.Pinned,
		SourceFile:      e.SourceFile,
		SourceFileLabel: sshconfig.CollapseTildePath(e.SourceFile),
	}
	for _, pf := range e.PortForwards {
		t.PortForwards = append(t.PortForwards, PortForward{
			LocalPort:   pf.LocalPort,
			RemoteHost:  pf.RemoteHost,
			RemotePort:  pf.RemotePort,
			Description: pf.Description,
			Portless:    pf.Portless,
			Domain:      pf.Domain,
			ExposePort:  pf.ExposePort,
		})
	}
	return t
}
