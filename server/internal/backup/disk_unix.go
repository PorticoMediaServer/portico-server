//go:build !windows

package backup

import (
	"golang.org/x/sys/unix"
)

// checkFreeSpace refuses the backup when free space on the state folder's
// filesystem is below 1.1 times the estimate.
func checkFreeSpace(state string, estimate int64) error {
	var stat unix.Statfs_t
	if err := unix.Statfs(state, &stat); err != nil {
		return err
	}
	free := int64(stat.Bavail) * int64(stat.Bsize)
	need := estimate + estimate/10
	if need < estimate {
		need = estimate
	}
	if free < need {
		return ErrInsufficientDisk
	}
	return nil
}
