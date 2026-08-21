//go:build !windows

package app

import (
	"errors"
	"os/exec"
	"syscall"
)

func ffmpegDiagnosticSignal(err error) string {
	if err == nil {
		return ""
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) || exitErr.ProcessState == nil {
		return ""
	}
	status, ok := exitErr.ProcessState.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() {
		return ""
	}
	return status.Signal().String()
}
