package prefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultUpdateChannelIsStable(t *testing.T) {
	store, err := NewStoreWithPath(filepath.Join(t.TempDir(), "prefs.json"))
	if err != nil {
		t.Fatalf("NewStoreWithPath: %v", err)
	}
	if got := store.Get().UpdateChannel; got != "stable" {
		t.Fatalf("UpdateChannel = %q, want stable", got)
	}
}

func TestUpdateChannelPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	store, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewStoreWithPath: %v", err)
	}
	p := store.Get()
	p.UpdateChannel = "beta"
	if err := store.Set(p); err != nil {
		t.Fatalf("Set: %v", err)
	}

	reloaded, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatalf("reloading store: %v", err)
	}
	if got := reloaded.Get().UpdateChannel; got != "beta" {
		t.Fatalf("UpdateChannel = %q, want beta", got)
	}
}

func TestOldPrefsFileMigratesToStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	if err := os.WriteFile(path, []byte(`{"closeToTray":false,"showResourceStats":true}`), 0o600); err != nil {
		t.Fatalf("writing prefs: %v", err)
	}

	store, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewStoreWithPath: %v", err)
	}
	p := store.Get()
	if p.UpdateChannel != "stable" {
		t.Fatalf("UpdateChannel = %q, want stable", p.UpdateChannel)
	}
	if p.CloseToTray {
		t.Fatal("existing preference was not preserved")
	}
}

func TestInvalidUpdateChannelNormalizesToStable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "prefs.json")
	if err := os.WriteFile(path, []byte(`{"updateChannel":"nightly"}`), 0o600); err != nil {
		t.Fatalf("writing prefs: %v", err)
	}

	store, err := NewStoreWithPath(path)
	if err != nil {
		t.Fatalf("NewStoreWithPath: %v", err)
	}
	if got := store.Get().UpdateChannel; got != "stable" {
		t.Fatalf("UpdateChannel = %q, want stable", got)
	}

	p := store.Get()
	p.UpdateChannel = "canary"
	if err := store.Set(p); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if got := store.Get().UpdateChannel; got != "stable" {
		t.Fatalf("UpdateChannel after Set = %q, want stable", got)
	}
}
