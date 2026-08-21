//go:build !windows

package app

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

func filesystemReservationKey(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return "", errors.New("filesystem identity is unavailable")
	}
	return fmt.Sprintf("device:%d", uint64(stat.Dev)), nil
}
