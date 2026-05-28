//go:build linux

package dns

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
)

const (
	resolvedDropIn = "/etc/systemd/resolved.conf.d/ssh-local.conf"
)

func isSystemConfigured() bool {
	if !hasSystemdResolved() {
		return false
	}
	data, err := os.ReadFile(resolvedDropIn)
	if err != nil {
		return false
	}
	content := string(data)
	return strings.Contains(content, fmt.Sprintf("DNS=127.0.0.1:%d", ListenPort)) &&
		strings.Contains(content, "Domains=~"+TLD)
}

// doSetup creates a systemd-resolved drop-in that forwards *.ssh-local
// queries to 127.0.0.1:5354 and restarts systemd-resolved. Must be invoked
// from a root-elevated process (via pkexec).
func doSetup() error {
	if !hasSystemdResolved() {
		return fmt.Errorf("portless mode requires systemd-resolved (not detected on this system)")
	}

	if err := os.MkdirAll("/etc/systemd/resolved.conf.d", 0o755); err != nil {
		return fmt.Errorf("creating drop-in dir: %w", err)
	}
	content := fmt.Sprintf(
		"[Resolve]\nDNS=127.0.0.1:%d\nDomains=~%s\n",
		ListenPort, TLD,
	)
	if err := os.WriteFile(resolvedDropIn, []byte(content), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", resolvedDropIn, err)
	}
	if out, err := exec.Command("systemctl", "restart", "systemd-resolved").CombinedOutput(); err != nil {
		return fmt.Errorf("restarting systemd-resolved: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	slog.Info("portless: systemd-resolved drop-in installed", "path", resolvedDropIn)
	return nil
}

func runElevatedSetup(ctx context.Context, exe string) error {
	if !hasSystemdResolved() {
		return fmt.Errorf("portless mode requires systemd-resolved (not detected)")
	}
	if _, err := exec.LookPath("pkexec"); err != nil {
		return fmt.Errorf("pkexec not found — install polkit to enable portless setup")
	}
	cmd := exec.CommandContext(ctx, "pkexec", exe, SetupArg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 126 {
			return fmt.Errorf("admin prompt cancelled")
		}
		return fmt.Errorf("pkexec: %w (%s)", err, trimmed)
	}
	return nil
}

func hasSystemdResolved() bool {
	out, err := exec.Command("systemctl", "is-active", "systemd-resolved").CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) == "active"
}
