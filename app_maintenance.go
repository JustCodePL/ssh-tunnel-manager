package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	goruntime "runtime"
	"strconv"
	"strings"
)

// logTailBytes bounds how much of the log one view reads.
const logTailBytes = 2 * 1024 * 1024

// LogEntry is one line of the app log.
type LogEntry struct {
	Time    string `json:"time"`
	Level   string `json:"level"`
	Message string `json:"message"`
	Details string `json:"details"`
}

// AppLog is the end of the app log, oldest entry first.
type AppLog struct {
	Path      string     `json:"path"`
	Entries   []LogEntry `json:"entries"`
	Truncated bool       `json:"truncated"`
}

// Diagnostics describes this installation. It names files, not servers, so it
// can go into a bug report as it is.
type Diagnostics struct {
	Version      string `json:"version"`
	System       string `json:"system"`
	Tunnels      int    `json:"tunnels"`
	Connected    int    `json:"connected"`
	SSHConfig    string `json:"sshConfig"`
	SettingsFile string `json:"settingsFile"`
	LogFile      string `json:"logFile"`
}

var logLinePattern = regexp.MustCompile(`^time=(\S+) level=(\S+) msg=("(?:[^"\\]|\\.)*"|\S+)\s*(.*)$`)

// parseLogLine reads a line written by slog's text handler. Anything else, such
// as a crash trace, keeps its text as the message.
func parseLogLine(line string) LogEntry {
	match := logLinePattern.FindStringSubmatch(line)
	if match == nil {
		return LogEntry{Message: line}
	}
	message := match[3]
	if unquoted, err := strconv.Unquote(message); err == nil {
		message = unquoted
	}
	return LogEntry{Time: match[1], Level: match[2], Message: message, Details: match[4]}
}

// isLogNoise is a line about the app serving its own files, one per file per
// window, which would bury everything else.
func isLogNoise(entry LogEntry) bool {
	return strings.HasPrefix(entry.Message, "[AssetFileServerFS]")
}

// readLogEntries reads the end of a log file. A missing file is an empty log.
func readLogEntries(path string, limit int) ([]LogEntry, bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("opening the log: %w", err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, fmt.Errorf("reading the log: %w", err)
	}
	truncated := false
	if info.Size() > logTailBytes {
		if _, err := file.Seek(-logTailBytes, io.SeekEnd); err != nil {
			return nil, false, fmt.Errorf("reading the log: %w", err)
		}
		truncated = true
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return nil, false, fmt.Errorf("reading the log: %w", err)
	}
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if truncated && len(lines) > 0 {
		lines = lines[1:] // the first line starts in the middle
	}
	entries := make([]LogEntry, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		if entry := parseLogLine(line); !isLogNoise(entry) {
			entries = append(entries, entry)
		}
	}
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
		truncated = true
	}
	return entries, truncated, nil
}

// GetAppLog returns the newest entries of the app log.
func (a *App) GetAppLog(limit int) (AppLog, error) {
	if limit <= 0 {
		limit = 2000
	}
	path, err := logFilePath()
	if err != nil {
		return AppLog{}, fmt.Errorf("finding the log: %w", err)
	}
	entries, truncated, err := readLogEntries(path, limit)
	if err != nil {
		return AppLog{}, err
	}
	return AppLog{Path: path, Entries: entries, Truncated: truncated}, nil
}

// GetDiagnostics returns facts about this installation.
func (a *App) GetDiagnostics() Diagnostics {
	statuses := a.manager.GetStatuses()
	connected := 0
	for _, status := range statuses {
		if status.Status == "connected" {
			connected++
		}
	}
	diagnostics := Diagnostics{
		Version:      Version,
		System:       goruntime.GOOS + " " + goruntime.GOARCH,
		Tunnels:      len(a.store.GetTunnels()),
		Connected:    connected,
		SettingsFile: a.prefs.Path(),
	}
	if home, err := os.UserHomeDir(); err == nil {
		diagnostics.SSHConfig = filepath.Join(home, ".ssh", "config")
	}
	if path, err := logFilePath(); err == nil {
		diagnostics.LogFile = path
	}
	return diagnostics
}

// OpenLogFolder shows the folder of the app log in the system's file manager.
func (a *App) OpenLogFolder() error {
	path, err := logFilePath()
	if err != nil {
		return fmt.Errorf("finding the log: %w", err)
	}
	if a.wails == nil {
		return nil
	}
	if err := a.wails.Browser.OpenFile(filepath.Dir(path)); err != nil {
		return fmt.Errorf("opening the log folder: %w", err)
	}
	return nil
}

// ResetWindowPositions forgets where tool windows were, for a window that ended
// up somewhere it cannot be reached, such as a screen no longer attached.
func (a *App) ResetWindowPositions() error {
	p := a.prefs.Get()
	p.WindowBounds = nil
	return a.prefs.Set(p)
}
