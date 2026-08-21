//go:build !unix && !windows

package app

import "os/exec"

func prepareManagedBackgroundCommand(cmd *exec.Cmd) {}

func lowerManagedBackgroundPriority(cmd *exec.Cmd) {}

func killManagedBackgroundCommand(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}

func releaseManagedBackgroundCommand(_ *exec.Cmd) {}
