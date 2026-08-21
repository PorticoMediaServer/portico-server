//go:build windows

package app

import "golang.org/x/sys/windows"

func filesystemSpace(path string) (availableBytes int64, totalBytes int64, err error) {
	pathPointer, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, err
	}
	var available uint64
	var total uint64
	var free uint64
	if err := windows.GetDiskFreeSpaceEx(pathPointer, &available, &total, &free); err != nil {
		return 0, 0, err
	}
	return saturatedWindowsBytes(available), saturatedWindowsBytes(total), nil
}

func saturatedWindowsBytes(value uint64) int64 {
	const maxInt64 = int64(^uint64(0) >> 1)
	if value > uint64(maxInt64) {
		return maxInt64
	}
	return int64(value)
}
