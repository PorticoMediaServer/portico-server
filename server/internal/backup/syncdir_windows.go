//go:build windows

package backup

// Windows has no supported FlushFileBuffers operation on a directory handle.
// Files are flushed before publication; NTFS journals the subsequent renames.
func syncDirectory(string) error { return nil }
