//go:build windows

package updater

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"os/exec"
)

const platformAsset = "ssh-tunnel-manager-amd64-installer.exe"

// Install downloads the NSIS installer to the temp directory and launches it
// detached, then returns so the caller can quit the app and let NSIS replace
// the binary.
func Install(ctx context.Context, info *UpdateInfo) error {
	dest := filepath.Join(os.TempDir(), "ssh-tunnel-manager-installer.exe")

	if err := downloadFile(ctx, info.AssetUrl, dest); err != nil {
		return fmt.Errorf("downloading installer: %w", err)
	}
	slog.Info("updater: installer downloaded", "path", dest)

	cmd := exec.Command(dest)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x00000008 | syscall.CREATE_NEW_PROCESS_GROUP,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("launching installer: %w", err)
	}
	slog.Info("updater: installer launched", "pid", cmd.Process.Pid)
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
