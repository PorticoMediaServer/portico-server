//go:build !windows

package app

import (
	"errors"
	"syscall"
)

func storageErrnoClass(err error) (string, bool) {
	switch {
	case errors.Is(err, syscall.ESTALE):
		return "stale_handle", true
	case errors.Is(err, syscall.EIO):
		return "io", true
	case errors.Is(err, syscall.EPERM):
		return "permission", true
	default:
		return "", false
	}
}
