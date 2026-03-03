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
	"runtime"
	"strings"
	"time"
)

// platformAsset is the release asset name for the current architecture.
var platformAsset = func() string {
	if runtime.GOARCH == "amd64" {
		return "ssh-tunnel-manager-darwin-amd64.dmg"
	}
	return "ssh-tunnel-manager-darwin-arm64.dmg"
}()

// Install downloads the macOS DMG, mounts it, replaces the running .app
// bundle with the new one, removes quarantine, then relaunches it.
func Install(ctx context.Context, info *UpdateInfo) error {
	arch := runtime.GOARCH
	assetName := "ssh-tunnel-manager-darwin-" + arch + ".dmg"
	tempDir := os.TempDir()
	dmgPath := filepath.Join(tempDir, assetName)

	if err := downloadFile(ctx, info.AssetUrl, dmgPath); err != nil {
		return fmt.Errorf("downloading update: %w", err)
	}
	slog.Info("updater: DMG downloaded", "path", dmgPath)

	// Mount the DMG
	out, err := exec.CommandContext(ctx, "hdiutil", "attach", "-nobrowse", "-readonly", dmgPath).Output()
	if err != nil {
		return fmt.Errorf("mounting DMG: %w", err)
	}

	// Parse mount point from hdiutil output (last line contains /Volumes/...)
	mountPoint := parseMountPoint(string(out))
	if mountPoint == "" {
		return fmt.Errorf("could not determine DMG mount point from hdiutil output")
	}
	slog.Info("updater: DMG mounted", "mountPoint", mountPoint)

	defer func() {
		if err := exec.Command("hdiutil", "detach", mountPoint, "-quiet").Run(); err != nil {
			slog.Warn("updater: failed to unmount DMG", "error", err)
		}
	}()

	// Find the .app inside the mounted DMG
	entries, err := os.ReadDir(mountPoint)
	if err != nil {
		return fmt.Errorf("reading DMG contents: %w", err)
	}
	var appName string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".app") {
			appName = e.Name()
			break
		}
	}
	if appName == "" {
		return fmt.Errorf("no .app found in DMG at %s", mountPoint)
	}

	extractedApp := filepath.Join(mountPoint, appName)

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
	installTarget := currentAppPath

	// Copy the new .app over the existing one
	if err := os.RemoveAll(installTarget); err != nil {
		return fmt.Errorf("removing old app bundle: %w", err)
	}
	if err := exec.Command("cp", "-R", extractedApp, installTarget).Run(); err != nil {
		return fmt.Errorf("copying new app bundle: %w", err)
	}

	slog.Info("updater: new app installed", "path", installTarget)

	if err := exec.Command("open", installTarget).Start(); err != nil {
		slog.Warn("updater: failed to relaunch app", "error", err)
	}
	return nil
}

// parseMountPoint extracts the /Volumes/... path from hdiutil attach output.
// hdiutil prints lines like: /dev/disk4s1  Apple_HFS  /Volumes/SSH Tunnel Manager
func parseMountPoint(output string) string {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		fields := strings.Fields(line)
		for _, f := range fields {
			if strings.HasPrefix(f, "/Volumes/") {
				return f
			}
		}
		// /Volumes/ path may contain spaces — join fields after the last tab
		if idx := strings.LastIndex(line, "\t"); idx >= 0 {
			candidate := strings.TrimSpace(line[idx+1:])
			if strings.HasPrefix(candidate, "/Volumes/") {
				return candidate
			}
		}
	}
	return ""
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
