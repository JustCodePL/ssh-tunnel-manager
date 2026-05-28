//go:build darwin

package dns

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

const resolverPath = "/etc/resolver/ssh-local"

func isSystemConfigured() bool {
	data, err := os.ReadFile(resolverPath)
	if err != nil {
		return false
	}
	content := string(data)
	return strings.Contains(content, "nameserver 127.0.0.1") &&
		strings.Contains(content, fmt.Sprintf("port %d", ListenPort))
}

// doSetup writes /etc/resolver/ssh-local pointing the macOS resolver at
// 127.0.0.1:5354 for the .ssh-local TLD. Must be called from a process that
// already has root (i.e. via osascript "do shell script ... with administrator
// privileges").
func doSetup() error {
	if err := os.MkdirAll("/etc/resolver", 0o755); err != nil {
		return fmt.Errorf("creating /etc/resolver: %w", err)
	}
	content := fmt.Sprintf("nameserver 127.0.0.1\nport %d\n", ListenPort)
	if err := os.WriteFile(resolverPath, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", resolverPath, err)
	}
	slog.Info("portless: resolver file installed", "path", resolverPath)
	return nil
}

func runElevatedSetup(ctx context.Context, exe string) error {
	// AppleScript escaping: the path is wrapped in single quotes inside the
	// "do shell script" string. Escape embedded quotes defensively.
	safeExe := strings.ReplaceAll(exe, `"`, `\"`)
	script := fmt.Sprintf(
		`do shell script "\"%s\" %s" with administrator privileges`,
		safeExe, SetupArg,
	)
	cmd := exec.CommandContext(ctx, "osascript", "-e", script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if strings.Contains(trimmed, "User canceled") || strings.Contains(trimmed, "(-128)") {
			return fmt.Errorf("admin prompt cancelled")
		}
		return fmt.Errorf("osascript: %w (%s)", err, trimmed)
	}
	return nil
}
