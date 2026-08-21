//go:build !windows

package database

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

func restoreVolumeKey(path string) (string, error) {
	path = filepath.Clean(path)
	for {
		info, err := os.Stat(path)
		if err == nil {
			stat, ok := info.Sys().(*syscall.Stat_t)
			if !ok {
				return "", fmt.Errorf("volume metadata unavailable for %s", path)
			}
			return fmt.Sprintf("unix-dev:%d", stat.Dev), nil
		}
		parent := filepath.Dir(path)
		if parent == path {
			return "", err
		}
		path = parent
	}
}
