//go:build darwin && portless_helper

package dns

import (
	"context"
	"fmt"
)

// The privileged helper never registers or controls itself. These stubs keep
// ServiceManagement and its Objective-C bridge out of the root executable.
func isSetupPersistenceConfigured() bool { return true }
func modernServiceApplicable() bool      { return false }

func ensurePortlessService(context.Context, SetupRequirements) error {
	return fmt.Errorf("the privileged Portless helper cannot register itself")
}

func getSystemServiceStatus() SystemServiceStatus {
	return SystemServiceStatus{State: "helper"}
}

func installSystemService(context.Context) error {
	return fmt.Errorf("the privileged Portless helper cannot install itself")
}

func uninstallSystemService(context.Context) error {
	return fmt.Errorf("the privileged Portless helper cannot uninstall itself")
}

func openSystemServiceSettings() error {
	return fmt.Errorf("the privileged Portless helper has no user interface")
}
