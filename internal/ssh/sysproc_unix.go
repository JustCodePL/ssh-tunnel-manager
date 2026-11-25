//go:build unix

package ssh

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
)

// startProcessTree starts cmd in a process group of its own and returns a
// function that kills the whole group, so helpers the command started (such
// as `aws ssm start-session` or `ssh -W` under `sh -c`) do not outlive it.
// Cancelling the command's context kills the group too.
//
// The kill function must not be called after cmd.Wait has returned: the
// group's ID is the command's PID, which is only reserved until it is reaped.
func startProcessTree(cmd *exec.Cmd) (kill func(), err error) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	// The group is signalled once. Close kills it before Wait; a context
	// cancellation that exec.Cmd runs later, possibly after Wait has reaped
	// the command and released its ID, then does nothing.
	var once sync.Once
	killGroup := func() error {
		err := os.ErrProcessDone
		once.Do(func() { err = signalGroup(cmd.Process.Pid) })
		return err
	}
	cmd.Cancel = killGroup
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return func() { _ = killGroup() }, nil
}

// signalGroup kills every process in the process group pgid.
func signalGroup(pgid int) error {
	// A negative PID signals every process in the group.
	err := syscall.Kill(-pgid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		// exec.Cmd treats this as "already exited", like Process.Kill.
		return os.ErrProcessDone
	}
	if err != nil {
		return fmt.Errorf("killing process group %d: %w", pgid, err)
	}
	return nil
}
