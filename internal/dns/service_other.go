//go:build !darwin

package dns

import (
	"context"
	"fmt"
)

func getSystemServiceStatus() SystemServiceStatus {
	return SystemServiceStatus{State: "unsupported", Message: "The Portless system service is only used on macOS."}
}

func installSystemService(context.Context) error {
	return fmt.Errorf("the Portless system service is only available on macOS")
}

func uninstallSystemService(context.Context) error {
	return fmt.Errorf("the Portless system service is only available on macOS")
}

func openSystemServiceSettings() error {
	return fmt.Errorf("the Portless system service is only available on macOS")
}
