//go:build linux || darwin

package mounts

import (
	"golang.org/x/sys/unix"
	"os"
	"os/exec"
	"runtime"
)

func cacheSpaceAvailable(path string, floor int64) bool {
	var stat unix.Statfs_t
	return unix.Statfs(path, &stat) == nil && uint64(stat.Bavail)*uint64(stat.Bsize) >= uint64(floor)
}
func MountSupported() bool {
	if runtime.GOOS == "linux" {
		if _, e := os.Stat("/dev/fuse"); e != nil {
			return false
		}
		if _, e := exec.LookPath("fusermount3"); e == nil {
			return true
		}
		_, e := exec.LookPath("fusermount")
		return e == nil
	}
	if runtime.GOOS == "darwin" {
		_, e := os.Stat("/Library/Filesystems/macfuse.fs")
		return e == nil
	}
	return false
}
