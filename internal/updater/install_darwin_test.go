//go:build darwin

package updater

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseMountPoint(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name:   "volume name with spaces",
			output: "/dev/disk4\tGUID_partition_scheme\n/dev/disk4s1\tApple_HFS\t/Volumes/SSH Tunnel Manager\n",
			want:   "/Volumes/SSH Tunnel Manager",
		},
		{
			name:   "APFS volume",
			output: "/dev/disk5s1         Apple_APFS                 /Volumes/SSH Tunnel Manager 2\n",
			want:   "/Volumes/SSH Tunnel Manager 2",
		},
		{
			name:   "no mounted volume",
			output: "/dev/disk4\tGUID_partition_scheme\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := parseMountPoint(tt.output); got != tt.want {
				t.Fatalf("parseMountPoint() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppBundleNeedsElevation(t *testing.T) {
	tempDir := t.TempDir()
	appPath := filepath.Join(tempDir, "SSH Tunnel Manager.app")
	contentsPath := filepath.Join(appPath, "Contents")
	if err := os.MkdirAll(contentsPath, 0o755); err != nil {
		t.Fatalf("creating app directory: %v", err)
	}
	if appBundleNeedsElevation(appPath) {
		t.Fatal("user-owned writable bundle unexpectedly requires elevation")
	}

	if err := os.Chmod(contentsPath, 0o555); err != nil {
		t.Fatalf("making nested app directory read-only: %v", err)
	}
	if os.Geteuid() != 0 && !appBundleNeedsElevation(appPath) {
		t.Fatal("bundle with a read-only nested directory did not require elevation")
	}

	if appBundleNeedsElevation(filepath.Join(tempDir, "Not Installed.app")) {
		t.Fatal("missing install target unexpectedly requires elevation")
	}
}

func TestPathNeedsElevation(t *testing.T) {
	path := t.TempDir()
	if pathNeedsElevation(path) {
		t.Fatal("user-owned writable install directory unexpectedly requires elevation")
	}
	if err := os.Chmod(path, 0o555); err != nil {
		t.Fatalf("making install directory read-only: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o755) })
	if os.Geteuid() != 0 && !pathNeedsElevation(path) {
		t.Fatal("read-only install directory did not require elevation")
	}
}

func TestElevatedInstallScriptQuotesEveryPath(t *testing.T) {
	script, err := elevatedInstallScript(
		`/Applications/SSH "Tunnel" Manager.app/Contents/MacOS/ssh-tunnel-manager`,
		[]string{InstallHelperArg, `/Volumes/Beta's Build/SSH Tunnel Manager.app`, `/Applications/Old App.app`, `/Applications/New App.app`},
	)
	if err != nil {
		t.Fatalf("elevatedInstallScript() error: %v", err)
	}
	if got := strings.Count(script, "quoted form of"); got != 5 {
		t.Fatalf("quoted arguments = %d, want 5; script: %s", got, script)
	}
	if !strings.Contains(script, `SSH \"Tunnel\" Manager.app`) {
		t.Fatalf("AppleScript string did not escape embedded quotes: %s", script)
	}
	if !strings.HasSuffix(script, "with administrator privileges") {
		t.Fatalf("script does not request administrator privileges: %s", script)
	}

	compiledPath := filepath.Join(t.TempDir(), "install-helper.scpt")
	if out, err := exec.Command("/usr/bin/osacompile", "-o", compiledPath, "-e", script).CombinedOutput(); err != nil {
		t.Fatalf("generated AppleScript does not compile: %v (%s)\n%s", err, out, script)
	}
}

func TestElevatedInstallScriptRejectsControlCharacters(t *testing.T) {
	if _, err := elevatedInstallScript("/tmp/app\nname", []string{InstallHelperArg}); err == nil {
		t.Fatal("elevatedInstallScript accepted a newline")
	}
}

func TestReplaceAppBundlesStagesVerifiedBundle(t *testing.T) {
	parentDir := t.TempDir()
	sourceApp := filepath.Join(parentDir, "source", "SSH Tunnel Manager.app")
	currentApp := filepath.Join(parentDir, "SSH Tunnel Manager.app")
	createSignedTestApp(t, sourceApp, "new")
	createSignedTestApp(t, currentApp, "old")

	if err := replaceAppBundles(sourceApp, currentApp, currentApp); err != nil {
		t.Fatalf("replaceAppBundles() error: %v", err)
	}
	marker, err := os.ReadFile(filepath.Join(currentApp, "Contents", "Resources", "version"))
	if err != nil {
		t.Fatalf("reading installed marker: %v", err)
	}
	if got := string(marker); got != "new" {
		t.Fatalf("installed marker = %q, want new", got)
	}

	entries, err := os.ReadDir(parentDir)
	if err != nil {
		t.Fatalf("reading install parent: %v", err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".ssh-tunnel-manager-") {
			t.Fatalf("temporary updater directory was not removed: %s", entry.Name())
		}
	}
}

func createSignedTestApp(t *testing.T, appPath, marker string) {
	t.Helper()
	macOSDir := filepath.Join(appPath, "Contents", "MacOS")
	resourcesDir := filepath.Join(appPath, "Contents", "Resources")
	if err := os.MkdirAll(macOSDir, 0o755); err != nil {
		t.Fatalf("creating test app MacOS directory: %v", err)
	}
	if err := os.MkdirAll(resourcesDir, 0o755); err != nil {
		t.Fatalf("creating test app Resources directory: %v", err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locating test executable: %v", err)
	}
	testBinary := filepath.Join(macOSDir, "ssh-tunnel-manager")
	if out, err := exec.Command("/usr/bin/ditto", exe, testBinary).CombinedOutput(); err != nil {
		t.Fatalf("copying test executable: %v (%s)", err, out)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>ssh-tunnel-manager</string>
<key>CFBundleIdentifier</key><string>com.wails.ssh-tunnel-manager.test</string>
<key>CFBundlePackageType</key><string>APPL</string>
</dict></plist>`
	if err := os.WriteFile(filepath.Join(appPath, "Contents", "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatalf("writing test Info.plist: %v", err)
	}
	if err := os.WriteFile(filepath.Join(resourcesDir, "version"), []byte(marker), 0o644); err != nil {
		t.Fatalf("writing test marker: %v", err)
	}
	if out, err := exec.Command("/usr/bin/codesign", "--force", "--deep", "--sign", "-", appPath).CombinedOutput(); err != nil {
		t.Fatalf("signing test app: %v (%s)", err, out)
	}
}

func TestScheduleRelaunchWaitsForOldProcess(t *testing.T) {
	oldProcess := exec.Command("/bin/sh", "-c", "read -r _ || exit 0")
	stdin, err := oldProcess.StdinPipe()
	if err != nil {
		t.Fatalf("creating old process stdin: %v", err)
	}
	if err := oldProcess.Start(); err != nil {
		t.Fatalf("starting old process: %v", err)
	}
	t.Cleanup(func() {
		_ = stdin.Close()
		_ = oldProcess.Process.Kill()
		_ = oldProcess.Wait()
	})

	tempDir := t.TempDir()
	markerPath := filepath.Join(tempDir, "relaunch-marker")
	openerPath := filepath.Join(tempDir, "open")
	opener := "#!/bin/sh\nprintf '%s' \"$1\" > \"$RELAUNCH_MARKER\"\n"
	if err := os.WriteFile(openerPath, []byte(opener), 0o700); err != nil {
		t.Fatalf("writing fake opener: %v", err)
	}
	t.Setenv("RELAUNCH_MARKER", markerPath)

	appPath := "/Applications/SSH Tunnel Manager.app"
	if err := scheduleRelaunchAfterExit(oldProcess.Process.Pid, appPath, openerPath); err != nil {
		t.Fatalf("scheduling relaunch: %v", err)
	}

	time.Sleep(150 * time.Millisecond)
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("replacement opened while old process was still running")
	}

	if err := stdin.Close(); err != nil {
		t.Fatalf("stopping old process: %v", err)
	}
	if err := oldProcess.Wait(); err != nil {
		t.Fatalf("waiting for old process: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(markerPath)
		if err == nil {
			if got := string(data); got != appPath {
				t.Fatalf("opened app path = %q, want %q", got, appPath)
			}
			return
		}
		if !os.IsNotExist(err) {
			t.Fatalf("reading relaunch marker: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("replacement app was not opened after old process exited")
}
