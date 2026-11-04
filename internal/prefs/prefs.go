// Package prefs manages application-level preferences stored separately
// from the SSH config (which holds tunnel configs).
package prefs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// HostTools records which optional remote tools were detected for a tunnel, so
// the monitoring buttons can be shown immediately on reconnect without
// re-probing. It is refreshed when a button is clicked and the tool is
// re-verified (host reinstalled, tool removed, etc.).
type HostTools struct {
	Docker bool   `json:"docker"`
	Htop   bool   `json:"htop"`
	RDP    bool   `json:"rdp,omitempty"`
	OS     string `json:"os,omitempty"`
}

// Prefs holds user-configurable application preferences.
type Prefs struct {
	CloseToTray       bool   `json:"closeToTray"`
	StartMinimized    bool   `json:"startMinimized"`
	WindowWidth       int    `json:"windowWidth,omitempty"`
	WindowHeight      int    `json:"windowHeight,omitempty"`
	ShowResourceStats bool   `json:"showResourceStats"`
	UpdateChannel     string `json:"updateChannel"`
	// HostTools is keyed by tunnel ID.
	HostTools map[string]HostTools `json:"hostTools,omitempty"`

	// RDPCertificates pins the SHA-256 fingerprint of each host's RDP server
	// certificate on first use, keyed by tunnel ID.
	RDPCertificates map[string]string `json:"rdpCertificates,omitempty"`
	// SSHHostKeys pins, on first use, the SHA-256 host key fingerprints a tunnel
	// with a password login has seen: tunnel ID, then server address.
	SSHHostKeys map[string]map[string]string `json:"sshHostKeys,omitempty"`

	// Theme is the app's look: "terminal" or "graphite".
	Theme string `json:"theme"`
	// WindowGrouping decides which tool tabs share a window.
	WindowGrouping string `json:"windowGrouping"`
	// AfterDisconnect is what tool windows do when a connection drops.
	AfterDisconnect string `json:"afterDisconnect"`
	// HideToolsWithMain hides tool windows together with the main window.
	HideToolsWithMain bool `json:"hideToolsWithMain"`
	// Refresh holds the default refresh interval per monitor, in seconds. Zero
	// means manual only.
	Refresh map[string]int `json:"refresh,omitempty"`
	// Row holds the default place of each row module.
	Row map[string]string `json:"row,omitempty"`
	// Tunnels holds per-tunnel overrides, keyed by tunnel ID.
	Tunnels map[string]TunnelSettings `json:"tunnels,omitempty"`
	// WindowBounds remembers where tool windows were, by window key.
	WindowBounds map[string]WindowBounds `json:"windowBounds,omitempty"`
	// Groups lists groups created by hand, so one can exist before its first
	// tunnel. Other groups exist because a tunnel names them.
	Groups []string `json:"groups,omitempty"`
	// Credentials lists saved logins. Secrets live in the OS keychain.
	Credentials []Credential `json:"credentials,omitempty"`
	// CredentialBindings maps "<protocol>:<target key>" to a credential ID.
	CredentialBindings map[string]string `json:"credentialBindings,omitempty"`
}

// TunnelSettings overrides the app defaults for one tunnel. Unset fields inherit.
type TunnelSettings struct {
	AfterDisconnect string            `json:"afterDisconnect,omitempty"`
	Refresh         map[string]int    `json:"refresh,omitempty"`
	Row             map[string]string `json:"row,omitempty"`
	Clipboard       string            `json:"clipboard,omitempty"`
	// RDP overrides detection for the host's own remote desktop: "on" offers it
	// without a detected server, "off" hides it, and empty follows detection.
	RDP string `json:"rdp,omitempty"`
}

// WindowBounds is a saved window position and size.
type WindowBounds struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// Credential is a saved login's public part.
type Credential struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Protocols []string `json:"protocols"`
}

// Values of the window and appearance settings.
const (
	ThemeTerminal = "terminal"
	ThemeGraphite = "graphite"

	AfterDisconnectResume = "resume"
	AfterDisconnectKeep   = "keep"
	AfterDisconnectClose  = "close"

	ClipboardBoth = "both"
	ClipboardTo   = "to"
	ClipboardFrom = "from"
	ClipboardOff  = "off"

	RowInRow   = "row"
	RowInPanel = "panel"
	RowOff     = "off"
)

// DefaultRefresh returns the built-in refresh intervals in seconds.
func DefaultRefresh() map[string]int {
	return map[string]int{"stats": 3, "htop": 3, "disk": 5, "docker": 0}
}

// DefaultRow returns the built-in place of every row module.
func DefaultRow() map[string]string {
	return map[string]string{
		"attach":   RowInRow,
		"forwards": RowInRow,
		"stats":    RowInPanel,
		"address":  RowInPanel,
		"files":    RowInPanel,
		"logs":     RowInPanel,
		"disk":     RowInPanel,
		"docker":   RowInPanel,
		"htop":     RowInPanel,
	}
}

// ResolveRefresh merges defaults, app values and tunnel overrides.
func ResolveRefresh(app, tunnel map[string]int) map[string]int {
	out := DefaultRefresh()
	for k, v := range app {
		out[k] = v
	}
	for k, v := range tunnel {
		out[k] = v
	}
	return out
}

// ResolveRow merges defaults, app values and tunnel overrides.
func ResolveRow(app, tunnel map[string]string) map[string]string {
	out := DefaultRow()
	for k, v := range app {
		out[k] = v
	}
	for k, v := range tunnel {
		out[k] = v
	}
	return out
}

// defaultPrefs returns a Prefs with sensible defaults.
func defaultPrefs() Prefs {
	return Prefs{
		CloseToTray:       true,
		StartMinimized:    true,
		WindowWidth:       900,
		WindowHeight:      600,
		ShowResourceStats: true,
		UpdateChannel:     "stable",
		Theme:             ThemeTerminal,
		WindowGrouping:    "tool-target",
		AfterDisconnect:   AfterDisconnectResume,
	}
}

// Store manages reading and writing of application preferences.
type Store struct {
	mu       sync.RWMutex
	prefs    Prefs
	filePath string
}

// NewStore creates a prefs Store backed by the default path
// (~/.config/ssh-tunnel-manager/prefs.json).
func NewStore() (*Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("getting home dir: %w", err)
	}
	path := filepath.Join(home, ".config", "ssh-tunnel-manager", "prefs.json")
	return NewStoreWithPath(path)
}

// NewStoreWithPath creates a prefs Store backed by the given path.
func NewStoreWithPath(path string) (*Store, error) {
	s := &Store{filePath: path, prefs: defaultPrefs()}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// Path returns the file preferences are stored in.
func (s *Store) Path() string {
	return s.filePath
}

// Get returns a copy of the current preferences.
func (s *Store) Get() Prefs {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.prefs
}

// Set replaces preferences and persists them to disk.
func (s *Store) Set(p Prefs) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	normalizePrefs(&p)
	s.prefs = p
	return s.save()
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.filePath)
	if os.IsNotExist(err) {
		// First run — use defaults, no error.
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading prefs: %w", err)
	}
	if err := json.Unmarshal(data, &s.prefs); err != nil {
		return fmt.Errorf("parsing prefs: %w", err)
	}
	normalizePrefs(&s.prefs)
	return nil
}

func normalizePrefs(p *Prefs) {
	if p.UpdateChannel != "stable" && p.UpdateChannel != "beta" {
		p.UpdateChannel = "stable"
	}
	if p.Theme != ThemeTerminal && p.Theme != ThemeGraphite {
		p.Theme = ThemeTerminal
	}
	if p.WindowGrouping != "tool-target" && p.WindowGrouping != "tool" && p.WindowGrouping != "target" {
		p.WindowGrouping = "tool-target"
	}
	switch p.AfterDisconnect {
	case AfterDisconnectResume, AfterDisconnectKeep, AfterDisconnectClose:
	default:
		p.AfterDisconnect = AfterDisconnectResume
	}
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.filePath), 0o700); err != nil {
		return fmt.Errorf("creating prefs dir: %w", err)
	}
	data, err := json.MarshalIndent(s.prefs, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling prefs: %w", err)
	}
	if err := os.WriteFile(s.filePath, data, 0o600); err != nil {
		return fmt.Errorf("writing prefs: %w", err)
	}
	return nil
}
