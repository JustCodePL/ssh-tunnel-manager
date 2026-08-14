//go:build !linux

package netcap

import (
	"context"
	"fmt"
)

// UnprivilegedPortStart is a Linux sysctl concept. macOS low ports are handled
// by the Portless PF redirect before the listener is created.
func UnprivilegedPortStart() int { return 0 }

// IsPrivilegedPort reports whether the Linux capability flow applies.
func IsPrivilegedPort(port int) bool { return false }

// SetcapCommand has no meaning off Linux.
func SetcapCommand(path string) string { return "" }

// HasBindServiceCapability reports true off Linux because Linux file
// capabilities do not apply on these platforms.
func HasBindServiceCapability(path string) (bool, error) { return true, nil }

// Authorize is a no-op error off Linux; the capability concept doesn't apply.
func Authorize(ctx context.Context, path string) error {
	return fmt.Errorf("privileged-port capability grant is only required on Linux")
}
