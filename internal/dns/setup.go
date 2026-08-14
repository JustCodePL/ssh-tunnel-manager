package dns

import (
	"context"
	"fmt"
	"log/slog"
	"os"
)

// SetupArg is the CLI flag main.go inspects to determine that the process was
// relaunched with elevated privileges purely to perform Portless system setup.
const SetupArg = "--setup-dns"

// PrivilegedRedirectArg tells the elevated macOS helper to install the narrow
// PF redirect used by Portless public ports below 1024.
const PrivilegedRedirectArg = "--setup-privileged-port-redirect"

// SetupRequirements describes which machine-wide Portless prerequisites a
// tunnel needs before its listeners can start.
type SetupRequirements struct {
	PrivilegedPortRedirect bool
}

// IsSystemConfigured reports whether the OS-level resolver and all requested
// platform prerequisites are ready. Platforms differ in how they record this
// state (files on macOS/Linux, registry/markers on Windows).
func IsSystemConfigured(requirements SetupRequirements) bool {
	return isSystemConfigured() &&
		(!requirements.PrivilegedPortRedirect || isPrivilegedPortRedirectConfigured())
}

// EnsureSystemConfigured runs the platform-specific setup, prompting the user
// for admin privileges (UAC / sudo / pkexec) if necessary. Returns nil if all
// requested prerequisites are already configured or setup completed.
//
// The mechanism on all three platforms is to relaunch the current executable
// with the --setup-dns flag and an elevation wrapper; the elevated copy then
// calls RunSetup with the requested prerequisites and exits.
func EnsureSystemConfigured(ctx context.Context, requirements SetupRequirements) error {
	if IsSystemConfigured(requirements) {
		return nil
	}
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolving current executable: %w", err)
	}
	args := []string{SetupArg}
	if requirements.PrivilegedPortRedirect {
		args = append(args, PrivilegedRedirectArg)
	}
	slog.Info("portless: launching elevated system setup", "exe", exe,
		"privilegedPortRedirect", requirements.PrivilegedPortRedirect)
	if err := runElevatedSetup(ctx, exe, args); err != nil {
		return err
	}
	if !IsSystemConfigured(requirements) {
		return fmt.Errorf("Portless system setup did not persist — admin prompt was likely cancelled")
	}
	return nil
}

// RunSetup performs the actual privileged setup work and is intended to be
// called from main() when the --setup-dns flag is present. The process exits
// after RunSetup returns.
func RunSetup(requirements SetupRequirements) error {
	return doSetup(requirements)
}
