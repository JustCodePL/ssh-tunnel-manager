//go:build darwin && !portless_helper

package dns

/*
#cgo LDFLAGS: -framework Foundation -framework ServiceManagement
#include <stdlib.h>
#include "service_darwin.h"
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"
)

const (
	portlessServicePlistName = "pl.justcode.ssh-tunnel-manager.portless.plist"

	serviceStatusNotRegistered    = 0
	serviceStatusEnabled          = 1
	serviceStatusRequiresApproval = 2
	serviceStatusNotFound         = 3
)

var errServiceApprovalRequired = errors.New("Portless system service needs approval in System Settings → General → Login Items & Extensions; allow SSH Tunnel Manager in the background, then retry")

func isSetupPersistenceConfigured() bool {
	if !modernServiceApplicable() {
		return true
	}
	return serviceStatus() == serviceStatusEnabled && serviceMarkerMatchesExecutable()
}

func modernServiceApplicable() bool {
	return C.stm_portless_service_supported() == 1 && bundledServicePresent()
}

func bundledServicePresent() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	if filepath.Base(exe) == portlessServiceHelper {
		return true
	}
	contentsDir := filepath.Dir(filepath.Dir(exe))
	plist := filepath.Join(contentsDir, "Library", "LaunchDaemons", portlessServicePlistName)
	helper := filepath.Join(contentsDir, "Library", "HelperTools", portlessServiceHelper)
	if _, err = os.Stat(plist); err != nil {
		return false
	}
	info, err := os.Stat(helper)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0
}

func appInstalledForBoot() bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return strings.HasPrefix(filepath.Clean(exe), "/Applications/")
}

func serviceStatus() int {
	return int(C.stm_portless_service_status())
}

func getSystemServiceStatus() SystemServiceStatus {
	if C.stm_portless_service_supported() != 1 {
		return SystemServiceStatus{State: "unsupported", Message: "Persistent Portless setup requires macOS 13 or newer."}
	}
	if !bundledServicePresent() {
		return SystemServiceStatus{State: "unavailable", Message: "This development build does not contain the signed Portless system service."}
	}

	result := SystemServiceStatus{Available: true}
	switch serviceStatus() {
	case serviceStatusEnabled:
		result.Installed = true
		result.Current = serviceMarkerMatchesExecutable()
		result.State = "enabled"
		if result.Current {
			result.Message = "Portless system service is approved and ready for reboot restoration."
		} else {
			result.Message = "Portless system service needs to be refreshed for this app version."
		}
	case serviceStatusRequiresApproval:
		result.Installed = true
		result.ApprovalRequired = true
		result.State = "approval-required"
		result.Message = errServiceApprovalRequired.Error()
	case serviceStatusNotRegistered, serviceStatusNotFound:
		result.State = "not-installed"
		result.Message = "Portless system service is not installed. It will be requested on first Portless use."
	default:
		result.State = "unavailable"
		result.Message = "Portless system service status is unavailable."
	}
	if !appInstalledForBoot() {
		result.Current = false
		result.Message = "Move SSH Tunnel Manager to /Applications before installing the Portless system service."
	}
	return result
}

func installSystemService(ctx context.Context) error {
	if !modernServiceApplicable() {
		return fmt.Errorf("this app build does not contain the macOS Portless system service")
	}
	return ensurePortlessService(ctx, SetupRequirements{PrivilegedPortRedirect: true})
}

func ensurePortlessService(ctx context.Context, requirements SetupRequirements) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !appInstalledForBoot() {
		return fmt.Errorf("move SSH Tunnel Manager to /Applications before enabling Portless so macOS can restore it during boot")
	}

	status := serviceStatus()
	if status == serviceStatusRequiresApproval {
		return errServiceApprovalRequired
	}

	markerCurrent := serviceMarkerMatchesExecutable()
	prerequisitesReady := systemPrerequisitesConfigured(requirements)
	if status == serviceStatusEnabled && markerCurrent && !prerequisitesReady {
		// At login, the GUI and the boot daemon may start concurrently. Give the
		// already-approved helper time to finish before considering a refresh.
		ready, err := waitForServiceSetup(ctx, requirements, 20*time.Second)
		if err != nil {
			return err
		}
		if ready {
			return nil
		}
	}

	replace := status == serviceStatusEnabled && (!markerCurrent || !prerequisitesReady)
	registeredStatus, err := registerService(replace)
	if err != nil {
		return err
	}
	if registeredStatus == serviceStatusRequiresApproval {
		C.stm_portless_service_open_settings()
		return errServiceApprovalRequired
	}
	if registeredStatus != serviceStatusEnabled {
		return fmt.Errorf("Portless system service was registered but is not enabled (status %d)", registeredStatus)
	}

	ready, err := waitForServiceSetup(ctx, requirements, 30*time.Second)
	if err != nil {
		return err
	}
	if ready {
		return nil
	}
	return fmt.Errorf("Portless system service is approved but did not finish setup; restart the app or review the macOS system log for %s", portlessServicePlistName)
}

func waitForServiceSetup(ctx context.Context, requirements SetupRequirements, timeout time.Duration) (bool, error) {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		if serviceMarkerMatchesExecutable() && systemPrerequisitesConfigured(requirements) {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-deadline.C:
			return false, nil
		case <-ticker.C:
		}
	}
}

func systemPrerequisitesConfigured(requirements SetupRequirements) bool {
	return isSystemConfigured() &&
		(!requirements.PrivilegedPortRedirect || isPrivilegedPortRedirectConfigured())
}

func registerService(replace bool) (int, error) {
	var cError *C.char
	replaceValue := C.int(0)
	if replace {
		replaceValue = 1
	}
	status := int(C.stm_portless_service_register(replaceValue, &cError))
	if cError != nil {
		defer C.free(unsafe.Pointer(cError))
		return status, fmt.Errorf("registering Portless system service: %s", C.GoString(cError))
	}
	if status < 0 {
		return status, fmt.Errorf("registering Portless system service failed with status %d", status)
	}
	return status, nil
}

func uninstallSystemService(ctx context.Context) error {
	if !modernServiceApplicable() {
		return fmt.Errorf("this app build does not contain the macOS Portless system service")
	}
	helper, err := portlessServiceExecutable()
	if err != nil {
		return err
	}
	if err := runLegacyElevatedExecutable(ctx, helper, []string{CleanupArg}); err != nil {
		return fmt.Errorf("removing Portless system configuration: %w", err)
	}

	var cError *C.char
	result := int(C.stm_portless_service_unregister(&cError))
	if cError != nil {
		defer C.free(unsafe.Pointer(cError))
		return fmt.Errorf("unregistering Portless system service: %s", C.GoString(cError))
	}
	if result < 0 {
		return fmt.Errorf("unregistering Portless system service failed with status %d", result)
	}
	return nil
}

func openSystemServiceSettings() error {
	if C.stm_portless_service_supported() != 1 {
		return fmt.Errorf("Portless system service settings require macOS 13 or newer")
	}
	C.stm_portless_service_open_settings()
	return nil
}
