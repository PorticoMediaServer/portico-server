//go:build darwin || linux

package administration

import (
	"fmt"
	"golang.org/x/sys/unix"
)

func fileIdentity(path string) (string, error) {
	var info unix.Stat_t
	if err := unix.Lstat(path, &info); err != nil {
		return "", err
	}
	if info.Mode&unix.S_IFMT != unix.S_IFREG {
		return "", ErrDenied
	}
	return fmt.Sprintf("%d:%d", info.Dev, info.Ino), nil
}
