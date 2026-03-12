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

// Prefs holds user-configurable application preferences.
type Prefs struct {
	CloseToTray bool `json:"closeToTray"`
}

// defaultPrefs returns a Prefs with sensible defaults.
func defaultPrefs() Prefs {
	return Prefs{
		CloseToTray: true,
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
	return nil
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
