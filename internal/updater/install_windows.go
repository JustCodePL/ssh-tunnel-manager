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
	"unsafe"
)

const platformAsset = "ssh-tunnel-manager-amd64-installer.exe"

// Install downloads the NSIS installer to the temp directory and launches it
// with UAC elevation (ShellExecuteW + "runas" verb), then returns so the
// caller can quit the app and let NSIS replace the binary.
func Install(ctx context.Context, info *UpdateInfo) error {
	dest := filepath.Join(os.TempDir(), "ssh-tunnel-manager-installer.exe")

	if err := downloadFile(ctx, info.AssetUrl, dest); err != nil {
		return fmt.Errorf("downloading installer: %w", err)
	}
	slog.Info("updater: installer downloaded", "path", dest)

	if err := shellExecuteElevated(dest); err != nil {
		return fmt.Errorf("launching installer: %w", err)
	}
	slog.Info("updater: installer launched with elevation")
	return nil
}

// shellExecuteElevated launches the given executable via ShellExecuteW with
// the "runas" verb, which triggers the UAC elevation prompt on Windows.
func shellExecuteElevated(path string) error {
	shell32 := syscall.NewLazyDLL("shell32.dll")
	shellExecuteW := shell32.NewProc("ShellExecuteW")

	verb, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return fmt.Errorf("encoding verb: %w", err)
	}
	file, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("encoding path: %w", err)
	}

	// ShellExecuteW(hwnd, verb, file, params, dir, showCmd)
	// Returns > 32 on success.
	ret, _, _ := shellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(file)),
		0,
		0,
		1, // SW_NORMAL
	)
	if ret <= 32 {
		return fmt.Errorf("ShellExecuteW returned %d", ret)
	}
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
