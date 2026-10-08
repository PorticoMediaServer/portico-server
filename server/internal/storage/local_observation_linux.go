//go:build linux

package storage

import (
	"fmt"
	"os"
	"syscall"
)

func observedObjectBinding(info os.FileInfo) (string, string, error) {
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", "", ErrObservedSourceUnsupported
	}
	return fmt.Sprintf("dev:%d:ino:%d", s.Dev, s.Ino), fmt.Sprintf("%d:%d", s.Ctim.Sec, s.Ctim.Nsec), nil
}
