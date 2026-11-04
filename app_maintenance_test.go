package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ssh-tunnel-manager/internal/prefs"
)

func TestParseLogLine(t *testing.T) {
	entry := parseLogLine(`time=2026-11-04T07:04:21.242+01:00 level=WARN msg="saving a \"file\" failed" name=a.txt bytes=16`)
	if entry.Time != "2026-11-04T07:04:21.242+01:00" || entry.Level != "WARN" || entry.Message != `saving a "file" failed` || entry.Details != "name=a.txt bytes=16" {
		t.Errorf("entry = %+v", entry)
	}
	if entry := parseLogLine(`time=2026-11-04T07:04:21Z level=INFO msg=started`); entry.Message != "started" || entry.Details != "" {
		t.Errorf("bare message = %+v", entry)
	}
	if entry := parseLogLine("panic: boom"); entry.Message != "panic: boom" || entry.Level != "" {
		t.Errorf("other text = %+v", entry)
	}
}

func TestReadLogEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	if entries, truncated, err := readLogEntries(path, 10); err != nil || entries != nil || truncated {
		t.Fatalf("missing file: %v %v %v", entries, truncated, err)
	}
	log := strings.Join([]string{
		`time=t1 level=INFO msg=one`,
		`time=t2 level=INFO msg="[AssetFileServerFS] Handling request" url=/a.js`,
		`time=t3 level=ERROR msg=two`,
		`time=t4 level=INFO msg=three`,
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, truncated, err := readLogEntries(path, 10)
	if err != nil || truncated || len(entries) != 3 || entries[1].Level != "ERROR" {
		t.Fatalf("entries = %+v truncated=%v err=%v", entries, truncated, err)
	}
	entries, truncated, err = readLogEntries(path, 2)
	if err != nil || !truncated || len(entries) != 2 || entries[0].Message != "two" || entries[1].Message != "three" {
		t.Fatalf("limited = %+v truncated=%v err=%v", entries, truncated, err)
	}
}

func TestReadLogEntriesDropsThePartialFirstLineOfALongLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app.log")
	line := "time=t level=INFO msg=" + strings.Repeat("x", 1000) + "\n"
	if err := os.WriteFile(path, []byte(strings.Repeat(line, logTailBytes/1000+10)), 0o600); err != nil {
		t.Fatal(err)
	}
	entries, truncated, err := readLogEntries(path, 1<<30)
	if err != nil || !truncated || len(entries) == 0 {
		t.Fatalf("truncated=%v err=%v count=%d", truncated, err, len(entries))
	}
	if entries[0].Level != "INFO" {
		t.Errorf("first entry is partial: %.40q", entries[0].Message)
	}
}

func TestResetWindowPositions(t *testing.T) {
	app := newTestApp(t, demoTunnel("host", ""))
	p := app.prefs.Get()
	p.WindowBounds = map[string]prefs.WindowBounds{"terminal:host": {X: 9000, Y: 9000, Width: 100, Height: 100}}
	if err := app.prefs.Set(p); err != nil {
		t.Fatal(err)
	}
	if err := app.ResetWindowPositions(); err != nil {
		t.Fatal(err)
	}
	if len(app.prefs.Get().WindowBounds) != 0 {
		t.Error("window positions stayed")
	}
}

func TestDiagnosticsNameNoServers(t *testing.T) {
	app := newTestApp(t, demoTunnel("secret-host", ""))
	diagnostics := app.GetDiagnostics()
	if diagnostics.Tunnels != 1 || diagnostics.Connected != 0 || diagnostics.System == "" {
		t.Errorf("diagnostics = %+v", diagnostics)
	}
	if strings.Contains(diagnostics.SSHConfig+diagnostics.LogFile+diagnostics.SettingsFile, "secret-host") {
		t.Errorf("a server's name leaked: %+v", diagnostics)
	}
}
