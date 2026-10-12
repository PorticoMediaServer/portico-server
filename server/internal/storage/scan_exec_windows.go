//go:build windows

package storage

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
	"portico.local/server/internal/mediaexec"
)

const descriptorInput = "portico-retained-scan-input"
const scanCPUSeconds = 300

// A decoder that must open by name gets a read-data handle denying writes and
// deletion, plus non-reparse directory handles denying ancestor replacement.
// Only after matching the admitted handle do we expose this locked physical
// path. Protocol and format bounds stay identical to descriptor-fed Unix jobs.
func lockWindowsMediaInput(source *os.File) (*os.File, string, func(), error) {
	var buffer [32768]uint16
	n, err := windows.GetFinalPathNameByHandle(windows.Handle(source.Fd()), &buffer[0], uint32(len(buffer)), 0)
	if err != nil || n == 0 || n >= uint32(len(buffer)) {
		return nil, "", nil, ErrObservedSourceLost
	}
	path := windows.UTF16ToString(buffer[:n])
	expected, token, err := windowsFileBinding(source)
	if err != nil {
		return nil, "", nil, err
	}
	var held []*os.File
	closeInput := func() {
		for i := len(held) - 1; i >= 0; i-- {
			held[i].Close()
		}
	}
	open := func(name string) (*os.File, error) {
		wide, err := windows.UTF16PtrFromString(name)
		if err != nil {
			return nil, err
		}
		handle, err := windows.CreateFile(wide, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			return nil, err
		}
		file := os.NewFile(uintptr(handle), name)
		held = append(held, file)
		var info windows.ByHandleFileInformation
		if err := windows.GetFileInformationByHandle(handle, &info); err != nil {
			return nil, err
		}
		if err := rejectWindowsNameSurrogate(handle, info.FileAttributes); err != nil {
			return nil, err
		}
		return file, nil
	}
	locked, err := open(path)
	if err != nil {
		closeInput()
		return nil, "", nil, err
	}
	for parent := filepath.Dir(path); ; parent = filepath.Dir(parent) {
		directory, err := open(parent)
		if err != nil {
			closeInput()
			return nil, "", nil, err
		}
		info, err := directory.Stat()
		if err != nil || !info.IsDir() {
			closeInput()
			return nil, "", nil, ErrObservedSourceLost
		}
		if filepath.Dir(parent) == parent {
			break
		}
	}
	// Ancestor locks now stabilize the namespace. Reopen that exact name once
	// more and compare the full object/revision identity, not size or a path.
	current, err := os.Open(path)
	if err != nil {
		closeInput()
		return nil, "", nil, err
	}
	actual, revision, err := windowsFileBinding(current)
	current.Close()
	if err != nil || actual != expected || revision != token {
		closeInput()
		return nil, "", nil, ErrObservedSourceLost
	}
	actual, revision, err = windowsFileBinding(locked)
	if err != nil || actual != expected || revision != token {
		closeInput()
		return nil, "", nil, ErrObservedSourceLost
	}
	info, err := locked.Stat()
	if err != nil || !info.Mode().IsRegular() {
		closeInput()
		return nil, "", nil, ErrObservedSourceLost
	}
	if !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n") {
		closeInput()
		return nil, "", nil, ErrObservedSourceLost
	}
	return locked, path, closeInput, nil
}

var probeJob struct {
	sync.Once
	handle windows.Handle
	err    error
}

// Windows cannot exec the tool in place. A mandatory nested job owned by this
// helper ends every descendant when it exits or is killed by its supervisor.
// Do not allow silent breakaway, and do not rely on the server's optional job.
func ensureWindowsProbeJob() error {
	probeJob.Do(func() {
		job, err := windows.CreateJobObject(nil, nil)
		if err != nil {
			probeJob.err = err
			return
		}
		limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
		limits.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE | windows.JOB_OBJECT_LIMIT_JOB_TIME | windows.JOB_OBJECT_LIMIT_PRIORITY_CLASS
		limits.BasicLimitInformation.PriorityClass = windows.BELOW_NORMAL_PRIORITY_CLASS
		limits.BasicLimitInformation.PerJobUserTimeLimit = int64(scanCPUSeconds) * 10000000
		if _, err = windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err == nil {
			err = windows.AssignProcessToJobObject(job, windows.CurrentProcess())
		}
		if err != nil {
			windows.CloseHandle(job)
			probeJob.err = err
			return
		}
		// Held to process exit; closing a job containing this helper kills it.
		probeJob.handle = job
	})
	return probeJob.err
}

func inventoryCommandHelper(r request) error {
	if err := validateInventoryCommand(r); err != nil {
		return err
	}
	if err := ensureWindowsProbeJob(); err != nil {
		return err
	}
	source, err := openInventoryContent(*r.Inventory)
	if err != nil {
		return err
	}
	defer source.Close()
	_, path, closeInput, err := lockWindowsMediaInput(source)
	if err != nil {
		return err
	}
	defer closeInput()
	argv, err := replaceScanInput(r.Argv, descriptorInput, path)
	if err != nil {
		return err
	}
	cmd, err := mediaexec.CommandArgv(context.Background(), argv)
	if err != nil {
		return err
	}
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	return cmd.Run()
}
