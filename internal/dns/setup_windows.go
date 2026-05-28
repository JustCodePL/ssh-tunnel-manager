//go:build windows

package dns

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const nrptComment = "ssh-tunnel-manager portless"

// markerPath is the file dropped after a successful elevated setup so that
// subsequent runs can skip the slow Get-DnsClientNrptRule check (PowerShell
// cold start can be tens of seconds on Windows). The user can delete the
// file to force a re-check. We use ProgramData (machine-wide) so the file
// is visible whether the elevated process runs as the same user or a
// different admin account.
//
// The "-v2" suffix is bumped whenever the install layout changes (e.g.
// switching the bind IP); existing users go through UAC once to upgrade.
func markerPath() string {
	dir := os.Getenv("ProgramData")
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "ssh-tunnel-manager", "portless-setup-done-v2")
}

// isSystemConfigured returns true if the per-user marker file is present.
// We deliberately do NOT poll PowerShell here — that proved to hang the UI
// thread on cold-start Windows boxes. The marker is written by doSetup once
// the NRPT rule has been installed in an elevated process.
func isSystemConfigured() bool {
	_, err := os.Stat(markerPath())
	return err == nil
}

// doSetup installs an NRPT rule routing *.ssh-local queries to BindIP. Must
// be invoked from an elevated process. Also writes the marker file so future
// isSystemConfigured checks can return true without invoking PowerShell.
// Removes any pre-existing rules tagged with our comment first so re-running
// after an IP change always converges on exactly one rule.
func doSetup() error {
	script := fmt.Sprintf(
		`Get-DnsClientNrptRule | Where-Object { $_.Comment -eq "%[2]s" } | Remove-DnsClientNrptRule -Force -ErrorAction SilentlyContinue; `+
			`Add-DnsClientNrptRule -Namespace ".%[1]s" -NameServers "%[3]s" -Comment "%[2]s"`,
		TLD, nrptComment, BindIP,
	)
	if _, err := powershellOutput(script); err != nil {
		return fmt.Errorf("installing NRPT rule: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(markerPath()), 0o755); err != nil {
		return fmt.Errorf("creating marker dir: %w", err)
	}
	if err := os.WriteFile(markerPath(), []byte(BindIP+"\n"), 0o644); err != nil {
		return fmt.Errorf("writing marker file: %w", err)
	}
	slog.Info("portless: NRPT rule installed", "bindIP", BindIP)
	return nil
}

// runElevatedSetup relaunches the given executable with --setup-dns through
// the Windows "runas" verb, which triggers the UAC prompt. Blocks until the
// child process exits or the timeout elapses.
//
// ShellExecuteEx requires COM apartment-threaded init on the calling thread.
// Without that, the UAC dialog may silently fail to appear on some Windows
// configurations.
func runElevatedSetup(ctx context.Context, exe string) error {
	runtimeLockOSThread()
	defer runtimeUnlockOSThread()

	if err := coInitializeEx(); err != nil {
		slog.Warn("portless: CoInitializeEx failed; trying ShellExecuteEx anyway", "error", err)
	} else {
		defer coUninitialize()
	}

	verb, _ := syscall.UTF16PtrFromString("runas")
	file, _ := syscall.UTF16PtrFromString(exe)
	args, _ := syscall.UTF16PtrFromString(SetupArg)

	var info shellExecuteInfo
	info.cbSize = uint32(unsafe.Sizeof(info))
	info.fMask = seeMaskNoCloseProcess
	info.lpVerb = verb
	info.lpFile = file
	info.lpParameters = args
	info.nShow = swShowNormal

	slog.Info("portless: invoking ShellExecuteEx runas", "exe", exe, "args", SetupArg)
	if err := shellExecuteEx(&info); err != nil {
		return fmt.Errorf("ShellExecuteEx failed (admin prompt cancelled or blocked): %w", err)
	}
	if info.hProcess == 0 {
		// No handle means "verb completed without spawning a process" — for
		// runas that usually means UAC was declined.
		return fmt.Errorf("admin prompt was not granted")
	}
	defer windows.CloseHandle(windows.Handle(info.hProcess))
	slog.Info("portless: elevated helper started, waiting for exit")

	// Cap the wait so we never hang indefinitely if the helper deadlocks.
	const helperTimeout = 90 * time.Second
	waitCtx, cancel := context.WithTimeout(ctx, helperTimeout)
	defer cancel()

	done := make(chan uint32, 1)
	go func() {
		event, err := windows.WaitForSingleObject(windows.Handle(info.hProcess), windows.INFINITE)
		if err != nil {
			done <- 1
			return
		}
		done <- event
	}()
	select {
	case <-waitCtx.Done():
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return fmt.Errorf("timed out after %s waiting for admin DNS setup helper", helperTimeout)
	case <-done:
	}

	var code uint32
	if err := windows.GetExitCodeProcess(windows.Handle(info.hProcess), &code); err != nil {
		return fmt.Errorf("reading child exit code: %w", err)
	}
	if code != 0 {
		return fmt.Errorf("elevated DNS setup helper exited with code %d", code)
	}
	slog.Info("portless: elevated helper exited cleanly")
	return nil
}

var (
	modOle32           = windows.NewLazySystemDLL("ole32.dll")
	procCoInitializeEx = modOle32.NewProc("CoInitializeEx")
	procCoUninitialize = modOle32.NewProc("CoUninitialize")
)

const coinitApartmentThreaded = 0x2

func coInitializeEx() error {
	r1, _, _ := procCoInitializeEx.Call(0, uintptr(coinitApartmentThreaded))
	// S_OK (0) or S_FALSE (1) both mean COM is initialized.
	if r1 == 0 || r1 == 1 {
		return nil
	}
	return fmt.Errorf("CoInitializeEx returned 0x%x", r1)
}

func coUninitialize() {
	procCoUninitialize.Call()
}

func runtimeLockOSThread()   { runtime.LockOSThread() }
func runtimeUnlockOSThread() { runtime.UnlockOSThread() }

func powershellOutput(script string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return string(out), fmt.Errorf("powershell timed out after 30s")
	}
	if err != nil {
		return string(out), fmt.Errorf("powershell %q: %w (%s)", script, err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// --- ShellExecuteEx binding ---

const (
	seeMaskNoCloseProcess = 0x00000040
	swShowNormal          = 1
)

type shellExecuteInfo struct {
	cbSize         uint32
	fMask          uint32
	hwnd           windows.Handle
	lpVerb         *uint16
	lpFile         *uint16
	lpParameters   *uint16
	lpDirectory    *uint16
	nShow          int32
	hInstApp       windows.Handle
	lpIDList       uintptr
	lpClass        *uint16
	hkeyClass      windows.Handle
	dwHotKey       uint32
	hIconOrMonitor windows.Handle
	hProcess       windows.Handle
}

var (
	modShell32         = windows.NewLazySystemDLL("shell32.dll")
	procShellExecuteEx = modShell32.NewProc("ShellExecuteExW")
)

func shellExecuteEx(info *shellExecuteInfo) error {
	r1, _, err := procShellExecuteEx.Call(uintptr(unsafe.Pointer(info)))
	if r1 == 0 {
		return err
	}
	return nil
}
