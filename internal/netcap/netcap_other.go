//go:build !linux

package netcap

import (
	"context"
	"fmt"
)

// UnprivilegedPortStart is meaningless off Linux; macOS (10.14+) and Windows
// impose no privileged-port restriction on user processes.
func UnprivilegedPortStart() int { return 0 }

// IsPrivilegedPort always reports false: only Linux gates low ports behind a
// capability for unprivileged processes.
func IsPrivilegedPort(port int) bool { return false }

// SetcapCommand has no meaning off Linux.
func SetcapCommand(path string) string { return "" }

// HasBindServiceCapability reports true off Linux — there is no capability to
// be missing, so binding privileged ports always works.
func HasBindServiceCapability(path string) (bool, error) { return true, nil }

// Authorize is a no-op error off Linux; the capability concept doesn't apply.
func Authorize(ctx context.Context, path string) error {
	return fmt.Errorf("privileged-port capability grant is only required on Linux")
}
