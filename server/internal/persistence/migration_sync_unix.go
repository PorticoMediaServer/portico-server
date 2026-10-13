//go:build !windows

package persistence

import "os"

func syncMigrationBackupDirectory(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}
