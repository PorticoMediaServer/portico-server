//go:build windows

package app

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const managedWindowsCommandAttachTimeout = 5 * time.Second

type managedWindowsCommand struct {
	mu     sync.Mutex
	job    windows.Handle
	closed bool
}

var managedWindowsCommands struct {
	sync.Mutex
	values map[*exec.Cmd]*managedWindowsCommand
}

var managedWindowsNTDLL = windows.NewLazySystemDLL("ntdll.dll")
var managedWindowsNtResumeProcess = managedWindowsNTDLL.NewProc("NtResumeProcess")

func prepareManagedBackgroundCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(
		job,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)),
		uint32(unsafe.Sizeof(limits)),
	); err != nil {
		_ = windows.CloseHandle(job)
		return
	}

	state := &managedWindowsCommand{job: job}
	managedWindowsCommands.Lock()
	if managedWindowsCommands.values == nil {
		managedWindowsCommands.values = make(map[*exec.Cmd]*managedWindowsCommand)
	}
	managedWindowsCommands.values[cmd] = state
	managedWindowsCommands.Unlock()

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	// Keep the process suspended until it is assigned to the job. This closes
	// the race in which a collector can create a child before cancellation
	// cleanup has a chance to attach the root process to the job.
	cmd.SysProcAttr.CreationFlags |= windows.CREATE_SUSPENDED
	go attachManagedWindowsCommand(cmd, state)
}

func attachManagedWindowsCommand(cmd *exec.Cmd, state *managedWindowsCommand) {
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	timeout := time.NewTimer(managedWindowsCommandAttachTimeout)
	defer timeout.Stop()

	for {
		state.mu.Lock()
		closed := state.closed
		state.mu.Unlock()
		if closed {
			return
		}
		if cmd.Process != nil {
			state.mu.Lock()
			if state.closed {
				state.mu.Unlock()
				return
			}
			process := cmd.Process
			var assignErr error
			handleErr := process.WithHandle(func(handle uintptr) {
				if err := windows.AssignProcessToJobObject(state.job, windows.Handle(handle)); err != nil {
					assignErr = err
				}
			})
			if handleErr != nil {
				assignErr = handleErr
			}
			if assignErr != nil {
				state.mu.Unlock()
				terminateManagedWindowsCommand(cmd, state)
				return
			}
			if err := resumeManagedWindowsProcess(process); err != nil {
				state.mu.Unlock()
				terminateManagedWindowsCommand(cmd, state)
				return
			}
			state.mu.Unlock()
			go waitForManagedWindowsCommand(cmd, state, process)
			return
		}

		select {
		case <-ticker.C:
		case <-timeout.C:
			terminateManagedWindowsCommand(cmd, state)
			return
		}
	}
}

func resumeManagedWindowsProcess(process *os.Process) error {
	var status uintptr
	if err := process.WithHandle(func(handle uintptr) {
		status, _, _ = managedWindowsNtResumeProcess.Call(handle)
	}); err != nil {
		return err
	}
	if status != 0 {
		return fmt.Errorf("NtResumeProcess returned NTSTATUS 0x%x", status)
	}
	return nil
}

func waitForManagedWindowsCommand(cmd *exec.Cmd, state *managedWindowsCommand, process *os.Process) {
	_ = process.WithHandle(func(handle uintptr) {
		_, _ = windows.WaitForSingleObject(windows.Handle(handle), windows.INFINITE)
	})
	releaseManagedWindowsCommand(cmd, state)
}

func killManagedBackgroundCommand(cmd *exec.Cmd) {
	state := managedWindowsCommandFor(cmd)
	if state != nil {
		terminateManagedWindowsCommand(cmd, state)
		return
	}
	if cmd == nil || cmd.Process == nil {
		return
	}
	terminateWindowsProcessTree(cmd.Process)
}

func releaseManagedBackgroundCommand(cmd *exec.Cmd) {
	state := managedWindowsCommandFor(cmd)
	if state != nil {
		releaseManagedWindowsCommand(cmd, state)
	}
}

func managedWindowsCommandFor(cmd *exec.Cmd) *managedWindowsCommand {
	managedWindowsCommands.Lock()
	defer managedWindowsCommands.Unlock()
	return managedWindowsCommands.values[cmd]
}

func terminateManagedWindowsCommand(cmd *exec.Cmd, state *managedWindowsCommand) {
	state.mu.Lock()
	if state.closed {
		state.mu.Unlock()
		return
	}
	state.closed = true
	job := state.job
	state.job = 0
	state.mu.Unlock()

	if job != 0 {
		_ = windows.TerminateJobObject(job, 1)
		_ = windows.CloseHandle(job)
	}
	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
	removeManagedWindowsCommand(cmd, state)
}

func releaseManagedWindowsCommand(cmd *exec.Cmd, state *managedWindowsCommand) {
	state.mu.Lock()
	if state.closed {
		state.mu.Unlock()
		removeManagedWindowsCommand(cmd, state)
		return
	}
	state.closed = true
	job := state.job
	state.job = 0
	state.mu.Unlock()

	// JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE also cleans descendants that outlive
	// the root command's Wait call.
	if job != 0 {
		_ = windows.CloseHandle(job)
	}
	removeManagedWindowsCommand(cmd, state)
}

func removeManagedWindowsCommand(cmd *exec.Cmd, state *managedWindowsCommand) {
	managedWindowsCommands.Lock()
	if managedWindowsCommands.values[cmd] == state {
		delete(managedWindowsCommands.values, cmd)
	}
	managedWindowsCommands.Unlock()
}

func terminateWindowsProcessTree(process *os.Process) {
	if process == nil || process.Pid <= 0 {
		return
	}
	// This is only the fallback for job setup failure. Windows ships taskkill,
	// and /T preserves tree semantics when a job cannot be created or attached.
	_ = exec.Command("taskkill", "/PID", strconv.Itoa(process.Pid), "/T", "/F").Run()
}

func lowerManagedBackgroundPriority(_ *exec.Cmd) {}
