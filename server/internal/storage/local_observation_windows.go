//go:build windows

package storage

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// FileInfo on Windows contains attributes, not the volume/file index. Acquire
// those from one handle, holding writers and replacers out while matching the
// attributes used by the snapshot. Reparse points are never admitted.
func observedObjectBinding(info os.FileInfo, path string) (string, string, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", "", err
	}
	handle, err := windows.CreateFile(name, windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return "", "", err
	}
	file := os.NewFile(uintptr(handle), path)
	defer file.Close()
	current, err := file.Stat()
	if err != nil {
		return "", "", err
	}
	if !os.SameFile(info, current) || info.Size() != current.Size() || !info.ModTime().Equal(current.ModTime()) || info.IsDir() != current.IsDir() {
		return "", "", ErrObservedSourceLost
	}
	var id windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(handle, &id); err != nil {
		return "", "", err
	}
	if id.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return "", "", ErrObservedSourceUnsupported
	}
	// FILE_BASIC_INFO: four LARGE_INTEGER timestamps followed by attributes.
	// ChangeTime, unlike LastWriteTime, also captures metadata changes.
	var basic struct {
		CreationTime, LastAccessTime, LastWriteTime, ChangeTime int64
		FileAttributes                                          uint32
	}
	if err = windows.GetFileInformationByHandleEx(handle, windows.FileBasicInfo, (*byte)(unsafe.Pointer(&basic)), uint32(unsafe.Sizeof(basic))); err != nil {
		return "", "", err
	}
	return fmt.Sprintf("volume:%d:file:%d", id.VolumeSerialNumber, uint64(id.FileIndexHigh)<<32|uint64(id.FileIndexLow)), fmt.Sprintf("%d", basic.ChangeTime), nil
}
