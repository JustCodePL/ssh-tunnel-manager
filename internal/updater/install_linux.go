//go:build linux

package updater

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"ssh-tunnel-manager/internal/netcap"
)

const platformAsset = "ssh-tunnel-manager-linux-amd64.tar.gz"

// Install downloads the Linux tar.gz, extracts the binary, and atomically
// replaces the running executable with a rollback on failure.
func Install(ctx context.Context, info *UpdateInfo) error {
	tarPath := filepath.Join(os.TempDir(), "ssh-tunnel-manager-linux-amd64.tar.gz")

	if err := downloadFile(ctx, info.AssetUrl, tarPath); err != nil {
		return fmt.Errorf("downloading update: %w", err)
	}
	slog.Info("updater: archive downloaded", "path", tarPath)

	// Extract into a fresh per-update directory so parallel attempts and
	// leftover files from previous runs don't collide.
	extractDir, err := os.MkdirTemp("", "stm-update-*")
	if err != nil {
		return fmt.Errorf("creating extract dir: %w", err)
	}
	defer os.RemoveAll(extractDir)

	newBinary, err := extractBinary(tarPath, extractDir)
	if err != nil {
		return fmt.Errorf("extracting update: %w", err)
	}

	if err := os.Chmod(newBinary, 0o755); err != nil {
		return fmt.Errorf("chmod new binary: %w", err)
	}

	currentPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating current executable: %w", err)
	}

	// Capabilities are an attribute of the inode, so the freshly extracted
	// binary has empty caps. If the running binary had CAP_NET_BIND_SERVICE
	// (granted for portless mode), portless privileged-port binds would
	// silently break after the swap. Remember whether to restore it.
	hadBindCap, err := netcap.HasBindServiceCapability(currentPath)
	if err != nil {
		slog.Warn("updater: could not read current binary capabilities", "error", err)
	}

	backupPath := currentPath + ".backup"
	if err := os.Rename(currentPath, backupPath); err != nil {
		return fmt.Errorf("backing up current binary: %w", err)
	}

	if err := moveFile(newBinary, currentPath); err != nil {
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

	if hadBindCap {
		restorePrivilegedBindCapability(ctx, currentPath)
	}
	return nil
}

// restorePrivilegedBindCapability re-grants CAP_NET_BIND_SERVICE to the
// freshly installed binary so portless mode keeps working after an update.
// Best-effort: with PolicyKit's auth_admin_keep the user may not even be
// prompted within the same session. If it fails (e.g. headless, no pkexec),
// we log a clear warning — the next failed portless bind surfaces the
// actionable banner with the setcap command.
func restorePrivilegedBindCapability(ctx context.Context, path string) {
	if err := netcap.Authorize(ctx, path); err != nil {
		slog.Warn("updater: could not restore CAP_NET_BIND_SERVICE after update; "+
			"portless privileged-port binds will need re-authorization",
			"path", path, "setcap", netcap.SetcapCommand(path), "error", err)
		return
	}
	slog.Info("updater: restored CAP_NET_BIND_SERVICE on updated binary", "path", path)
}

// extractBinary extracts the ssh-tunnel-manager binary from the tar.gz at
// tarPath into destDir and returns the path to the extracted binary.
func extractBinary(tarPath, destDir string) (string, error) {
	f, err := os.Open(tarPath)
	if err != nil {
		return "", fmt.Errorf("opening archive: %w", err)
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return "", fmt.Errorf("reading gzip: %w", err)
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return "", fmt.Errorf("ssh-tunnel-manager binary not found in archive")
		}
		if err != nil {
			return "", fmt.Errorf("reading tar entry: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != "ssh-tunnel-manager" {
			continue
		}

		outPath := filepath.Join(destDir, "ssh-tunnel-manager")
		out, err := os.OpenFile(outPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
		if err != nil {
			return "", fmt.Errorf("creating %s: %w", outPath, err)
		}
		if _, err := io.Copy(out, tr); err != nil {
			out.Close()
			return "", fmt.Errorf("writing %s: %w", outPath, err)
		}
		if err := out.Close(); err != nil {
			return "", fmt.Errorf("closing %s: %w", outPath, err)
		}
		return outPath, nil
	}
}

// moveFile moves src to dst, falling back to copy+remove if rename fails
// (e.g. because src and dst are on different filesystems).
func moveFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Remove(src)
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
