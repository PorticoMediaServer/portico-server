//go:build windows

package backup

// checkFreeSpace skips the free-space check where statfs is unavailable.
func checkFreeSpace(state string, estimate int64) error { return nil }
