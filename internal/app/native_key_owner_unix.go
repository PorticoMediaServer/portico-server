//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package app

import (
	"errors"
	"os"
	"syscall"
)

func validateNativeCredentialOwner(info os.FileInfo) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("native credential key ownership is unavailable")
	}
	if int(stat.Uid) != os.Geteuid() {
		return errors.New("native credential key path must be owned by the server user")
	}
	return nil
}
