package config

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"ssh-tunnel-manager/internal/sshconfig"
)

// nameTaken reports whether a host or a remote desktop already uses the name,
// other than the remote desktop being renamed from except. The caller holds
// s.mu.
func (s *Store) nameTaken(name, except string) bool {
	for _, t := range s.tunnels {
		if t.Name == name {
			return true
		}
	}
	for _, d := range s.desktops {
		if d.Name == name && d.Name != except {
			return true
		}
	}
	return false
}

// GetRemoteDesktops returns a copy of all remote desktops.
func (s *Store) GetRemoteDesktops() []RemoteDesktop {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.desktops)
}

// GetRemoteDesktop returns the remote desktop with the given name.
func (s *Store) GetRemoteDesktop(name string) (RemoteDesktop, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, d := range s.desktops {
		if d.Name == name {
			return d, true
		}
	}
	return RemoteDesktop{}, false
}

// AddRemoteDesktop writes a new remote desktop to the section of its source
// file, or of the main config when it has none. Its name must not be used by
// a host or another remote desktop.
func (s *Store) AddRemoteDesktop(d RemoteDesktop) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.nameTaken(d.Name, "") {
		return fmt.Errorf("the name %q is already used by a host or a remote desktop", d.Name)
	}
	if d.SourceFile == "" {
		d.SourceFile = s.filePath
	}
	if err := s.ensureDirFor(d.SourceFile); err != nil {
		return err
	}
	lines, err := sshconfig.ReadLines(d.SourceFile)
	if err != nil {
		return fmt.Errorf("reading ssh config: %w", err)
	}
	for _, existing := range sshconfig.ParseRemoteDesktops(lines) {
		if existing.Name == d.Name {
			// Added by hand since loading: never overwrite it.
			return fmt.Errorf("the name %q is already used in %s", d.Name, sshconfig.CollapseTildePath(d.SourceFile))
		}
	}
	entry := toDesktopEntry(d)
	if err := s.rewriteSection(d.SourceFile, d.Name, &entry); err != nil {
		return err
	}
	s.desktops = append(s.desktops, normalizeDesktop(d))
	return nil
}

// UpdateRemoteDesktop replaces the remote desktop called oldName, in the file
// it was read from. A new name must be free.
func (s *Store) UpdateRemoteDesktop(oldName string, d RemoteDesktop) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := slices.IndexFunc(s.desktops, func(x RemoteDesktop) bool { return x.Name == oldName })
	if idx < 0 {
		return fmt.Errorf("remote desktop %q not found", oldName)
	}
	if d.Name != oldName && s.nameTaken(d.Name, oldName) {
		return fmt.Errorf("the name %q is already used by a host or a remote desktop", d.Name)
	}
	d.SourceFile = s.desktops[idx].SourceFile
	entry := toDesktopEntry(d)
	if err := s.rewriteSection(d.SourceFile, oldName, &entry); err != nil {
		return err
	}
	s.desktops[idx] = normalizeDesktop(d)
	return nil
}

// DeleteRemoteDesktop removes a remote desktop from its file.
func (s *Store) DeleteRemoteDesktop(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := slices.IndexFunc(s.desktops, func(x RemoteDesktop) bool { return x.Name == name })
	if idx < 0 {
		return fmt.Errorf("remote desktop %q not found", name)
	}
	if err := s.rewriteSection(s.desktops[idx].SourceFile, name, nil); err != nil {
		return err
	}
	s.desktops = slices.Delete(s.desktops, idx, idx+1)
	return nil
}

// rewriteSection changes one entry of a file's section as the file is on disk
// now, so lines added by hand since loading survive. A nil entry removes it.
// The caller holds s.mu.
func (s *Store) rewriteSection(file, name string, entry *sshconfig.RemoteDesktopEntry) error {
	lines, err := sshconfig.ReadLines(file)
	if err != nil {
		return fmt.Errorf("reading ssh config: %w", err)
	}
	lines = sshconfig.UpdateRemoteDesktopLines(lines, name, entry)
	if len(lines) == 0 {
		lines = []string{}
	}
	if err := sshconfig.WriteLines(file, lines); err != nil {
		return fmt.Errorf("writing ssh config: %w", err)
	}
	return nil
}

func (s *Store) loadRemoteDesktops() error {
	entries, err := sshconfig.ParseFileRemoteDesktops(s.filePath)
	if err != nil {
		return fmt.Errorf("loading remote desktops: %w", err)
	}
	s.desktops = nil
	for _, e := range entries {
		d := FromDesktopEntry(e)
		if err := ValidateRemoteDesktop(d); err != nil {
			slog.Warn("skipping remote desktop", "name", e.Name, "file", e.SourceFile, "error", err)
			continue
		}
		// IDs must be unique: the first entry of a name wins, as for ssh.
		if s.nameTaken(d.Name, "") {
			slog.Warn("skipping remote desktop with a name already in use", "name", e.Name, "file", e.SourceFile)
			continue
		}
		s.desktops = append(s.desktops, d)
	}
	return nil
}

func toDesktopEntry(d RemoteDesktop) sshconfig.RemoteDesktopEntry {
	return sshconfig.RemoteDesktopEntry{
		Name: d.Name, Address: d.Address, Port: d.Port, Protocol: d.Protocol,
		Group: d.Group, Color: d.Color, Pinned: d.Pinned, SourceFile: d.SourceFile,
	}
}

// ToDesktopEntry converts a remote desktop for writing to a config file.
func ToDesktopEntry(d RemoteDesktop) sshconfig.RemoteDesktopEntry { return toDesktopEntry(d) }

// FromDesktopEntry converts a parsed section entry.
func FromDesktopEntry(e sshconfig.RemoteDesktopEntry) RemoteDesktop {
	// An IPv6 address may be written in brackets, as in a URL.
	address := strings.TrimSuffix(strings.TrimPrefix(e.Address, "["), "]")
	return normalizeDesktop(RemoteDesktop{
		Name: e.Name, Address: address, Port: e.Port, Protocol: e.Protocol,
		Group: e.Group, Color: e.Color, Pinned: e.Pinned, SourceFile: e.SourceFile,
	})
}

func normalizeDesktop(d RemoteDesktop) RemoteDesktop {
	d.ID = RemoteDesktopID(d.Name)
	d.SourceFileLabel = sshconfig.CollapseTildePath(d.SourceFile)
	return d
}

// ValidateRemoteDesktop checks what the user typed before it is written.
func ValidateRemoteDesktop(d RemoteDesktop) error {
	if strings.TrimSpace(d.Name) == "" || d.Name != strings.TrimSpace(d.Name) {
		return fmt.Errorf("a remote desktop needs a name without leading or trailing spaces")
	}
	if strings.HasPrefix(d.Name, RemoteDesktopIDPrefix) {
		return fmt.Errorf("a name cannot start with %q", RemoteDesktopIDPrefix)
	}
	if d.Address == "" || strings.ContainsAny(d.Address, " \t\"\\/") {
		return fmt.Errorf("enter the address as a host name or an IP address")
	}
	if strings.Count(d.Address, ":") == 1 {
		return fmt.Errorf("enter the port in its own field, not in the address")
	}
	if d.Protocol != ForwardProtocolRDP && d.Protocol != ForwardProtocolVNC {
		return fmt.Errorf("unknown remote desktop protocol %q", d.Protocol)
	}
	if d.Port < 1 || d.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", d.Port)
	}
	for _, value := range []string{d.Name, d.Address, d.Group, d.Color} {
		if strings.ContainsFunc(value, func(r rune) bool { return r < 0x20 || r == 0x7f }) {
			return fmt.Errorf("names and addresses cannot contain control characters")
		}
	}
	return nil
}
