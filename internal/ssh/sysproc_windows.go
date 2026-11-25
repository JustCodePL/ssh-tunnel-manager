//go:build windows

package ssh

import (
	"fmt"
	"log/slog"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// startProcessTree starts cmd without a visible console window and puts it in
// a Job Object, and returns a function that ends every process in the job, so
// helpers the command started (such as `aws ssm start-session` or `ssh -W`)
// do not outlive it. Cancelling the command's context ends the job too. The
// job is created with KILL_ON_JOB_CLOSE, so its processes also end if the app
// exits without closing the connection.
//
// The command is added to the job right after it starts; a process it starts
// before that stays outside the job. If the job cannot be set up, kill ends
// only the command itself, as before.
func startProcessTree(cmd *exec.Cmd) (kill func(), err error) {
	// Without HideWindow, Windows pops up a console window for each
	// subprocess spawned by the GUI app.
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true

	job, jobErr := newKillOnCloseJob()
	if jobErr != nil {
		slog.Warn("could not create a job object for ProxyCommand; only the command itself will be stopped", "error", jobErr)
	}

	var (
		mu     sync.Mutex
		inJob  bool
		closed bool
	)
	killAll := func() error {
		mu.Lock()
		defer mu.Unlock()
		if closed {
			return nil
		}
		closed = true
		var err error
		if inJob {
			err = windows.TerminateJobObject(job, 1)
		} else if cmd.Process != nil {
			err = cmd.Process.Kill()
		}
		if job != 0 {
			_ = windows.CloseHandle(job)
		}
		return err
	}
	cmd.Cancel = killAll

	if err := cmd.Start(); err != nil {
		if job != 0 {
			_ = windows.CloseHandle(job)
		}
		return nil, err
	}
	// Hold the lock so a context cancellation cannot close the job meanwhile.
	mu.Lock()
	if job != 0 && !closed {
		if err := assignToJob(job, cmd.Process.Pid); err != nil {
			slog.Warn("could not add ProxyCommand to a job object; only the command itself will be stopped", "error", err)
		} else {
			inJob = true
		}
	}
	mu.Unlock()
	return func() { _ = killAll() }, nil
}

// newKillOnCloseJob creates a Job Object whose processes end when its last
// handle is closed.
func newKillOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, fmt.Errorf("creating job object: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		_ = windows.CloseHandle(job)
		return 0, fmt.Errorf("setting job object limits: %w", err)
	}
	return job, nil
}

// assignToJob adds the process with the given PID to job.
func assignToJob(job windows.Handle, pid int) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("opening process %d: %w", pid, err)
	}
	defer windows.CloseHandle(process)
	if err := windows.AssignProcessToJobObject(job, process); err != nil {
		return fmt.Errorf("assigning process %d to job object: %w", pid, err)
	}
	return nil
}
