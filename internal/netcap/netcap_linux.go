//go:build linux

package netcap

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/sys/unix"
)

// capNetBindService is the capability number for CAP_NET_BIND_SERVICE as
// defined in <linux/capability.h>. It lives in the first 32-bit capability
// word (caps 0–31).
const capNetBindService = 10

// defaultUnprivilegedStart is the kernel default for
// net.ipv4.ip_unprivileged_port_start, used when the sysctl can't be read.
const defaultUnprivilegedStart = 1024

var (
	unprivStartOnce  sync.Once
	unprivStartValue int
)

// UnprivilegedPortStart returns net.ipv4.ip_unprivileged_port_start — the
// lowest port a process may bind without CAP_NET_BIND_SERVICE. Read once from
// /proc and cached; falls back to the kernel default (1024) on any error.
func UnprivilegedPortStart() int {
	unprivStartOnce.Do(func() {
		unprivStartValue = defaultUnprivilegedStart
		data, err := os.ReadFile("/proc/sys/net/ipv4/ip_unprivileged_port_start")
		if err != nil {
			return
		}
		if n, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && n >= 0 {
			unprivStartValue = n
		}
	})
	return unprivStartValue
}

// IsPrivilegedPort reports whether binding the given port requires
// CAP_NET_BIND_SERVICE on this kernel.
func IsPrivilegedPort(port int) bool {
	return port > 0 && port < UnprivilegedPortStart()
}

// SetcapCommand returns the exact command a user can run to grant
// CAP_NET_BIND_SERVICE to the binary at path.
func SetcapCommand(path string) string {
	return fmt.Sprintf("sudo setcap '%s=+ep' %s", CapName, path)
}

// HasBindServiceCapability reports whether the binary at path carries
// CAP_NET_BIND_SERVICE in its file capabilities. It reads the
// security.capability extended attribute directly (no dependency on the
// getcap binary) and parses the vfs_cap_data structure.
func HasBindServiceCapability(path string) (bool, error) {
	// security.capability is at most 24 bytes (revision 3); 64 is generous.
	buf := make([]byte, 64)
	n, err := unix.Getxattr(path, "security.capability", buf)
	if err != nil {
		if err == unix.ENODATA || err == unix.ENOTSUP {
			return false, nil // no caps set, or filesystem can't store them
		}
		return false, fmt.Errorf("reading file capabilities of %s: %w", path, err)
	}
	// vfs_cap_data: magic_etc (u32) then data[].permitted (u32) at offset 4.
	// We only need the permitted set for the first capability word.
	if n < 8 {
		return false, nil
	}
	permitted := binary.LittleEndian.Uint32(buf[4:8])
	return permitted&(1<<capNetBindService) != 0, nil
}

// Authorize grants CAP_NET_BIND_SERVICE to the binary at path by invoking
// `pkexec setcap`, which presents the system PolicyKit dialog. Returns a
// descriptive error if pkexec is unavailable or the user cancels.
func Authorize(ctx context.Context, path string) error {
	if _, err := exec.LookPath("pkexec"); err != nil {
		return fmt.Errorf("pkexec not found — install polkit, or run manually: %s", SetcapCommand(path))
	}
	if _, err := exec.LookPath("setcap"); err != nil {
		return fmt.Errorf("setcap not found — install libcap2-bin (Debian/Ubuntu) or libcap (Fedora)")
	}
	cmd := exec.CommandContext(ctx, "pkexec", "setcap", CapName+"=+ep", path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(out))
		if cmd.ProcessState != nil && cmd.ProcessState.ExitCode() == 126 {
			return fmt.Errorf("admin prompt cancelled")
		}
		return fmt.Errorf("pkexec setcap: %w (%s)", err, trimmed)
	}
	return nil
}
