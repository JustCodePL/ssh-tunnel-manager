//go:build darwin

package dns

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const (
	portlessServiceHelper = "ssh-tunnel-manager-portless"
	serviceMarkerPath     = "/Library/Application Support/SSH Tunnel Manager/portless-helper.sha256"
)

var (
	currentDigestOnce sync.Once
	currentDigest     string
	currentDigestErr  error
)

func executableDigest(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func portlessServiceExecutable() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if filepath.Base(exe) == portlessServiceHelper {
		return exe, nil
	}
	contentsDir := filepath.Dir(filepath.Dir(exe))
	return filepath.Join(contentsDir, "Library", "HelperTools", portlessServiceHelper), nil
}

func currentExecutableDigest() (string, error) {
	currentDigestOnce.Do(func() {
		exe, err := portlessServiceExecutable()
		if err != nil {
			currentDigestErr = err
			return
		}
		currentDigest, currentDigestErr = executableDigest(exe)
	})
	return currentDigest, currentDigestErr
}

func serviceMarkerMatchesExecutable() bool {
	want, err := currentExecutableDigest()
	if err != nil {
		return false
	}
	data, err := os.ReadFile(serviceMarkerPath)
	return err == nil && strings.TrimSpace(string(data)) == want
}

func writeServiceVersionMarker() error {
	digest, err := currentExecutableDigest()
	if err != nil {
		return fmt.Errorf("hashing Portless helper executable: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(serviceMarkerPath), 0o755); err != nil {
		return fmt.Errorf("creating Portless helper marker directory: %w", err)
	}
	if err := os.WriteFile(serviceMarkerPath, []byte(digest+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing Portless helper marker: %w", err)
	}
	return nil
}
