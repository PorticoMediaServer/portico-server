//go:build !linux && !darwin

package mounts

func cacheSpaceAvailable(string, int64) bool { return false }
func MountSupported() bool                   { return false }
