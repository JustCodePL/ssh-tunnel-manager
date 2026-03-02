//go:build darwin

package updater

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const platformAsset = "ssh-tunnel-manager-darwin-arm64.zip"

// Install downloads the macOS zip, extracts the .app bundle, replaces the
// running app, removes quarantine, then relaunches it.
func Install(ctx context.Context, info *UpdateInfo) error {
	tempDir := os.TempDir()
	zipPath := filepath.Join(tempDir, "ssh-tunnel-manager-darwin-arm64.zip")

	if err := downloadFile(ctx, info.AssetUrl, zipPath); err != nil {
		return fmt.Errorf("downloading update: %w", err)
	}
	slog.Info("updater: archive downloaded", "path", zipPath)

	if err := exec.Command("unzip", "-o", zipPath, "-d", tempDir).Run(); err != nil {
		return fmt.Errorf("extracting update: %w", err)
	}

	extractedApp := filepath.Join(tempDir, "ssh-tunnel-manager-darwin-arm64", "ssh-tunnel-manager.app")

	// Remove quarantine attribute so macOS doesn't block the new binary.
	if err := exec.Command("xattr", "-dr", "com.apple.quarantine", extractedApp).Run(); err != nil {
		slog.Warn("updater: failed to remove quarantine", "error", err)
	}

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating current executable: %w", err)
	}
	// Navigate 3 levels up from Contents/MacOS/<binary> to reach the .app bundle.
	currentAppPath := filepath.Clean(filepath.Join(exePath, "..", "..", ".."))

	if err := os.RemoveAll(currentAppPath); err != nil {
		return fmt.Errorf("removing old app bundle: %w", err)
	}
	if err := os.Rename(extractedApp, currentAppPath); err != nil {
		return fmt.Errorf("installing new app bundle: %w", err)
	}

	if err := exec.Command("open", currentAppPath).Start(); err != nil {
		slog.Warn("updater: failed to relaunch app", "error", err)
	}
	slog.Info("updater: new app installed and launched", "path", currentAppPath)
	return nil
}

// downloadFile downloads src URL to the dst path with a 5-minute timeout.
func downloadFile(ctx context.Context, src, dst string) error {
	dlCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(dlCtx, http.MethodGet, src, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
