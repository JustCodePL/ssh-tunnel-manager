//go:build darwin

package updater

import (
	"os"
	"os/exec"
	"path/filepath"
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
