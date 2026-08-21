//go:build windows

package database

import (
	"fmt"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

func restoreVolumeKey(path string) (string, error) {
	path = filepath.Clean(path)
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	var volume [windows.MAX_PATH]uint16
	if err := windows.GetVolumePathName(name, &volume[0], uint32(len(volume))); err != nil {
		return "", fmt.Errorf("get volume for %s: %w", path, err)
	}
	return "windows-volume:" + strings.ToLower(windows.UTF16ToString(volume[:])), nil
}
