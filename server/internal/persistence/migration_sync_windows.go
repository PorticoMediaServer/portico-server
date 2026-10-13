//go:build windows

package persistence

// Windows flushes the copy before publication and journals renames in NTFS;
// FlushFileBuffers does not support directory handles.
func syncMigrationBackupDirectory(string) error { return nil }
