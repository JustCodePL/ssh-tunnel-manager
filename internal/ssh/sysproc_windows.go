//go:build windows

package ssh

import (
	"os/exec"
	"syscall"
)

// applySysProcAttr configures the command to run without a visible console
// window. Without this, Windows pops up a CMD window for each subprocess
// spawned by the GUI app (e.g. ProxyCommand shells).
func applySysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
