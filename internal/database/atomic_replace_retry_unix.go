//go:build !windows

package database

import (
	"errors"
	"syscall"
)

func atomicReplaceRetryable(err error) bool {
	return err != nil && (errors.Is(err, syscall.EBUSY) || errors.Is(err, syscall.ETXTBSY))
}
