//go:build !windows

package ssh

import "os/exec"

// applySysProcAttr is a no-op on non-Windows platforms.
func applySysProcAttr(cmd *exec.Cmd) {}
