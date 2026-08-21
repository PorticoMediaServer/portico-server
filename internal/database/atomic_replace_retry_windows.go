//go:build windows

package database

import (
	"errors"

	"golang.org/x/sys/windows"
)

func atomicReplaceRetryable(err error) bool {
	return err != nil && (errors.Is(err, windows.ERROR_SHARING_VIOLATION) || errors.Is(err, windows.ERROR_LOCK_VIOLATION))
}
