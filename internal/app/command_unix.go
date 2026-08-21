//go:build unix

package app

import (
	"os/exec"
	"syscall"
)

func prepareManagedBackgroundCommand(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func lowerManagedBackgroundPriority(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Setpriority(syscall.PRIO_PROCESS, cmd.Process.Pid, 10)
}

func killManagedBackgroundCommand(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}

func releaseManagedBackgroundCommand(_ *exec.Cmd) {}
