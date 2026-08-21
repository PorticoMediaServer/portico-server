//go:build !windows

package app

import "syscall"

func filesystemSpace(path string) (availableBytes int64, totalBytes int64, err error) {
	var stats syscall.Statfs_t
	if err := syscall.Statfs(path, &stats); err != nil {
		return 0, 0, err
	}
	blockSize := int64(stats.Bsize)
	return saturatedFilesystemBytes(uint64(stats.Bavail), blockSize), saturatedFilesystemBytes(uint64(stats.Blocks), blockSize), nil
}

func saturatedFilesystemBytes(blocks uint64, blockSize int64) int64 {
	if blockSize <= 0 || blocks == 0 {
		return 0
	}
	const maxInt64 = int64(^uint64(0) >> 1)
	if blocks > uint64(maxInt64/blockSize) {
		return maxInt64
	}
	return int64(blocks) * blockSize
}
