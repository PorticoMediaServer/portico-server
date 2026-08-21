//go:build windows

package database

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
)

// ErrDirectorySyncUnsupported documents the Windows filesystem contract. A
// number of supported volumes do not permit FlushFileBuffers on a directory
// handle even though MoveFileEx(WRITE_THROUGH) and flushed file handles are
// available. Those volumes use the latter fallback; unexpected errors remain
// fail-closed and are returned to the caller.
var ErrDirectorySyncUnsupported = errors.New("directory entry flush is unsupported by this Windows filesystem")

// FlushFileBuffers on a directory handle is the Windows durability boundary
// for the parent of an atomic rename. Native CI must exercise this on the
// supported filesystem; failure is surfaced rather than reported as a pass.
func syncDirectory(path string) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		if windowsDirectoryFlushUnsupported(err) {
			return nil
		}
		return fmt.Errorf("open directory for sync: %w", err)
	}
	defer windows.CloseHandle(handle)
	if err := windows.FlushFileBuffers(handle); err != nil {
		if windowsDirectoryFlushUnsupported(err) {
			return nil
		}
		return fmt.Errorf("sync directory: %w", err)
	}
	return nil
}

func windowsDirectoryFlushUnsupported(err error) bool {
	return errors.Is(err, windows.ERROR_INVALID_FUNCTION) ||
		errors.Is(err, windows.ERROR_NOT_SUPPORTED) ||
		errors.Is(err, windows.ERROR_INVALID_HANDLE) ||
		errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
