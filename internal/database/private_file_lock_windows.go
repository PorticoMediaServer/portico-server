//go:build windows

package database

import (
	"fmt"

	"golang.org/x/sys/windows"
)

func acquirePrivateFileLock(path string) (func(), error) {
	release, acquired, err := privateWindowsFileLock(path, false)
	if err != nil {
		return nil, err
	}
	if !acquired {
		return nil, fmt.Errorf("acquire private lifecycle lock: lock was not acquired")
	}
	return release, nil
}

func tryAcquirePrivateFileLock(path string) (func(), bool, error) {
	return privateWindowsFileLock(path, true)
}

func privateWindowsFileLock(path string, nonblocking bool) (func(), bool, error) {
	if err := rejectWindowsReparseComponents(path); err != nil {
		return nil, false, fmt.Errorf("inspect private lifecycle lock: %w", err)
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, false, fmt.Errorf("encode private lifecycle lock: %w", err)
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_ALWAYS, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, false, fmt.Errorf("open private lifecycle lock: %w", err)
	}
	if err := applyAndVerifyWindowsACL(path, false); err != nil {
		_ = windows.CloseHandle(handle)
		return nil, false, fmt.Errorf("protect private lifecycle lock: %w", err)
	}
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK)
	if nonblocking {
		flags |= windows.LOCKFILE_FAIL_IMMEDIATELY
	}
	overlapped := &windows.Overlapped{}
	if err := windows.LockFileEx(handle, flags, 0, 1, 0, overlapped); err != nil {
		_ = windows.CloseHandle(handle)
		if nonblocking && (err == windows.ERROR_LOCK_VIOLATION || err == windows.ERROR_SHARING_VIOLATION) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("acquire private lifecycle lock: %w", err)
	}
	return func() {
		_ = windows.UnlockFileEx(handle, 0, 1, 0, overlapped)
		_ = windows.CloseHandle(handle)
	}, true, nil
}
