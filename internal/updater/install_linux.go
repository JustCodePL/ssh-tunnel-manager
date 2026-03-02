//go:build linux

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

const platformAsset = "ssh-tunnel-manager-linux-amd64.tar.gz"

// Install downloads the Linux tar.gz, extracts the binary, and atomically
// replaces the running executable with a rollback on failure.
func Install(ctx context.Context, info *UpdateInfo) error {
	tempDir := os.TempDir()
	tarPath := filepath.Join(tempDir, "ssh-tunnel-manager-linux-amd64.tar.gz")

	if err := downloadFile(ctx, info.AssetUrl, tarPath); err != nil {
		return fmt.Errorf("downloading update: %w", err)
	}
	slog.Info("updater: archive downloaded", "path", tarPath)

	if err := exec.Command("tar", "xzf", tarPath, "-C", tempDir).Run(); err != nil {
		return fmt.Errorf("extracting update: %w", err)
	}

	newBinary := filepath.Join(tempDir, "ssh-tunnel-manager")
	if err := os.Chmod(newBinary, 0o755); err != nil {
		return fmt.Errorf("chmod new binary: %w", err)
	}

	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating current executable: %w", err)
	}

	backupPath := currentPath + ".backup"
	if err := os.Rename(currentPath, backupPath); err != nil {
		return fmt.Errorf("backing up current binary: %w", err)
	}

	if err := os.Rename(newBinary, currentPath); err != nil {
		// Restore backup
		if restoreErr := os.Rename(backupPath, currentPath); restoreErr != nil {
			slog.Error("updater: failed to restore backup after install failure",
				"backup", backupPath, "error", restoreErr)
		}
		return fmt.Errorf("installing new binary: %w", err)
	}

	// Clean up backup on success
	defer os.Remove(backupPath)

	slog.Info("updater: binary replaced", "path", currentPath)
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
