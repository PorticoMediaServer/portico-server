//go:build darwin

package storage

import (
	"fmt"
	"os"
	"syscall"
)

func observedObjectBinding(info os.FileInfo, path string) (string, string, error) {
	s, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", "", ErrObservedSourceUnsupported
	}
	object, err := directoryFilesystemIdentity(info, path, fmt.Sprintf("dev:%d:ino:%d", s.Dev, s.Ino))
	return object, fmt.Sprintf("%d:%d", s.Ctimespec.Sec, s.Ctimespec.Nsec), err
}

func checkLocalRoot(string) error { return nil }
