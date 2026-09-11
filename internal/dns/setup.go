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

// CleanupArg selects the explicit, one-shot privileged removal path used when
// the user uninstalls the macOS Portless system service from Settings.
const CleanupArg = "--cleanup-dns"

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
		isSetupPersistenceConfigured() &&
		(!requirements.PrivilegedPortRedirect || isPrivilegedPortRedirectConfigured())
}

// EnsureSystemConfigured runs the platform-specific setup, prompting the user
// for admin privileges (UAC / sudo / pkexec) if necessary. Returns nil if all
// requested prerequisites are already configured or setup completed.
//
// Windows, Linux, and legacy macOS builds relaunch the current executable via
// their elevation wrapper. Current signed macOS bundles instead register the
// minimal bundled LaunchDaemon through SMAppService.
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

// RunCleanup removes machine-wide Portless state. It is intentionally exposed
// only through the explicit service-uninstall flow; normal app startup never
// invokes it.
func RunCleanup() error {
	return doCleanup()
}
