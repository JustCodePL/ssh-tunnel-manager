//go:build darwin

package updater

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
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

	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locating current executable: %w", err)
	}
	// Navigate 3 levels up from Contents/MacOS/<binary> to reach the .app bundle.
	currentAppPath := filepath.Clean(filepath.Join(exePath, "..", "..", ".."))
	// Use the bundle name shipped in the DMG so an update can migrate older
	// installations from ssh-tunnel-manager.app to SSH Tunnel Manager.app.
	installTarget := filepath.Join(filepath.Dir(currentAppPath), appName)

	// A bundle in /Applications may have been installed by another local
	// administrator. Replacing it then requires macOS authorization even when
	// the current user can create entries in /Applications itself. Keep the
	// ordinary no-prompt path for user-owned bundles and elevate only when one
	// of the bundles that must be replaced is not writable.
	if pathNeedsElevation(filepath.Dir(installTarget)) ||
		appBundleNeedsElevation(currentAppPath) ||
		(currentAppPath != installTarget && appBundleNeedsElevation(installTarget)) {
		if err := runElevatedInstall(ctx, exePath, extractedApp, currentAppPath, installTarget); err != nil {
			return fmt.Errorf("installing update with administrator privileges: %w", err)
		}
	} else if err := replaceAppBundles(extractedApp, currentAppPath, installTarget); err != nil {
		return fmt.Errorf("replacing app bundle: %w", err)
	}

	slog.Info("updater: new app installed", "path", installTarget)

	// The current process is still running from the old bundle. Starting the
	// replacement now would briefly run beta and stable side by side and would
	// also race the single-instance lock. Let a detached helper wait for this
	// process to exit before opening the replacement.
	if err := scheduleRelaunchAfterExit(os.Getpid(), installTarget, "/usr/bin/open"); err != nil {
		return fmt.Errorf("scheduling app relaunch: %w", err)
	}
	return nil
}

func pathNeedsElevation(path string) bool {
	if _, err := os.Stat(path); err != nil {
		return true
	}
	return unix.Access(path, unix.W_OK) != nil
}

func appBundleNeedsElevation(path string) bool {
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return false
	} else if err != nil {
		return true
	}
	// Removing a tree needs write access to every directory that contains an
	// entry, not to the files themselves. Walking directories catches mixed-
	// ownership bundles as well as the common whole-bundle ownership mismatch.
	return filepath.WalkDir(path, func(currentPath string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() && unix.Access(currentPath, unix.W_OK) != nil {
			return os.ErrPermission
		}
		return nil
	}) != nil
}

// replaceAppBundles stages and verifies the new bundle before moving either
// installed bundle out of the way. Both staging and backup directories live
// next to the installation target, so every rename is atomic. A failed final
// rename restores the previous paths before returning.
func replaceAppBundles(extractedApp, currentAppPath, installTarget string) error {
	parentDir := filepath.Dir(installTarget)
	stagingDir, err := os.MkdirTemp(parentDir, ".ssh-tunnel-manager-update-")
	if err != nil {
		return fmt.Errorf("creating staging directory: %w", err)
	}
	defer os.RemoveAll(stagingDir)

	stagedApp := filepath.Join(stagingDir, filepath.Base(installTarget))
	if out, err := exec.Command("/usr/bin/ditto", extractedApp, stagedApp).CombinedOutput(); err != nil {
		return fmt.Errorf("staging new app bundle: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	// Clear quarantine on the writable staged copy rather than on the app
	// inside the read-only mounted DMG.
	if out, err := exec.Command("/usr/bin/xattr", "-dr", "com.apple.quarantine", stagedApp).CombinedOutput(); err != nil {
		slog.Warn("updater: failed to remove quarantine from staged app",
			"path", stagedApp, "error", err, "output", strings.TrimSpace(string(out)))
	}
	if out, err := exec.Command("/usr/bin/codesign", "--verify", "--deep", "--strict", stagedApp).CombinedOutput(); err != nil {
		return fmt.Errorf("verifying staged app bundle: %w (%s)", err, strings.TrimSpace(string(out)))
	}

	backupDir, err := os.MkdirTemp(parentDir, ".ssh-tunnel-manager-backup-")
	if err != nil {
		return fmt.Errorf("creating backup directory: %w", err)
	}

	type movedBundle struct {
		original string
		backup   string
	}
	moved := make([]movedBundle, 0, 2)
	moveToBackup := func(path string) error {
		if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
			return nil
		} else if err != nil {
			return err
		}
		backupPath := filepath.Join(backupDir, strconv.Itoa(len(moved))+"-"+filepath.Base(path))
		if err := os.Rename(path, backupPath); err != nil {
			return err
		}
		moved = append(moved, movedBundle{original: path, backup: backupPath})
		return nil
	}
	restoreBackups := func() error {
		var restoreErr error
		for i := len(moved) - 1; i >= 0; i-- {
			if err := os.Rename(moved[i].backup, moved[i].original); err != nil {
				restoreErr = errors.Join(restoreErr, fmt.Errorf("restoring %s: %w", moved[i].original, err))
			}
		}
		return restoreErr
	}

	if err := moveToBackup(installTarget); err != nil {
		_ = os.RemoveAll(backupDir)
		return fmt.Errorf("backing up install target: %w", err)
	}
	if currentAppPath != installTarget {
		if err := moveToBackup(currentAppPath); err != nil {
			restoreErr := restoreBackups()
			_ = os.RemoveAll(backupDir)
			return errors.Join(fmt.Errorf("backing up current app bundle: %w", err), restoreErr)
		}
	}

	if err := os.Rename(stagedApp, installTarget); err != nil {
		restoreErr := restoreBackups()
		_ = os.RemoveAll(backupDir)
		return errors.Join(fmt.Errorf("installing staged app bundle: %w", err), restoreErr)
	}

	if err := os.RemoveAll(backupDir); err != nil {
		// The replacement is already complete. A cleanup failure must not keep
		// the old process alive and make the successful update look unusable.
		slog.Warn("updater: failed to remove app backup after successful install",
			"path", backupDir, "error", err)
	}
	return nil
}

const relaunchAfterExitScript = `while kill -0 "$1" 2>/dev/null; do
    sleep 0.1
done
exec "$3" "$2"`

func scheduleRelaunchAfterExit(pid int, appPath, opener string) error {
	cmd := exec.Command(
		"/bin/sh",
		"-c",
		relaunchAfterExitScript,
		"ssh-tunnel-manager-relauncher",
		strconv.Itoa(pid),
		appPath,
		opener,
	)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// parseMountPoint extracts the /Volumes/... path from hdiutil attach output.
// hdiutil prints lines like: /dev/disk4s1  Apple_HFS  /Volumes/SSH Tunnel Manager
func parseMountPoint(output string) string {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		// The mount point is the final hdiutil column and may contain spaces.
		// Taking the remainder of the line avoids truncating, for example,
		// "/Volumes/SSH Tunnel Manager" to "/Volumes/SSH".
		if idx := strings.Index(line, "/Volumes/"); idx >= 0 {
			return strings.TrimSpace(line[idx:])
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
