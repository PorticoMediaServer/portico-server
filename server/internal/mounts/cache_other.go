//go:build !linux && !darwin

package mounts

func cacheSpaceAvailable(string, int64) bool { return false }
func MountSupported() bool                   { return false }

func cacheSpaceFor(string, int64, int64) bool { return false }
