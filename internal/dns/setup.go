package dns

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

// SetupArg is the CLI flag main.go inspects to determine that the process
// was relaunched with elevated privileges purely to perform DNS setup.
const SetupArg = "--setup-dns"

// IsSystemConfigured reports whether the OS-level resolver is already wired
// to forward *.ssh-local queries to our embedded DNS server. Platforms differ
// in how they record this (file on macOS/Linux, registry on Windows).
func IsSystemConfigured() bool {
	return isSystemConfigured()
}

// EnsureSystemConfigured runs the platform-specific one-time setup, prompting
// the user for admin privileges (UAC / sudo / pkexec) if necessary. Returns
// nil if the system is already configured or the setup completed.
//
// The mechanism on all three platforms is to relaunch the current executable
// with the --setup-dns flag and an elevation wrapper; the elevated copy then
// calls RunSetup and exits.
func EnsureSystemConfigured(ctx context.Context) error {
	if isSystemConfigured() {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving current executable: %w", err)
	}
	slog.Info("portless: launching elevated DNS setup", "exe", exe)
	if err := runElevatedSetup(ctx, exe); err != nil {
		return err
	}
	if !isSystemConfigured() {
		return fmt.Errorf("DNS setup did not persist — admin prompt was likely cancelled")
	}
	return nil
}

// RunSetup performs the actual privileged setup work and is intended to be
// called from main() when the --setup-dns flag is present. The process is
// expected to exit after RunSetup returns.
func RunSetup() error {
	return doSetup()
}
