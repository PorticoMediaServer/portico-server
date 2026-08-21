//go:build windows

package app

import (
	"errors"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

func filesystemReservationKey(path string) (string, error) {
	input, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	buffer := make([]uint16, windows.MAX_PATH+1)
	if err := windows.GetVolumePathName(input, &buffer[0], uint32(len(buffer))); err != nil {
		return "", err
	}
	end := 0
	for end < len(buffer) && buffer[end] != 0 {
		end++
	}
	root := strings.ToLower(strings.TrimSpace(string(utf16.Decode(buffer[:end]))))
	if root == "" {
		return "", errors.New("filesystem identity is unavailable")
	}
	return "volume:" + root, nil
}
