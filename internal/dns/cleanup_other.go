//go:build !darwin

package dns

import "fmt"

func doCleanup() error {
	return fmt.Errorf("Portless system-service cleanup is only available on macOS")
}

func isSetupPersistenceConfigured() bool { return true }
