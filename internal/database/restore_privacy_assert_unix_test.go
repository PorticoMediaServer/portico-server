//go:build !windows

package database

import (
	"fmt"
	"os"
)

func verifyPrivateRestoreArtifactForTest(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0o600 {
		return fmt.Errorf("mode=%#o, want 0600", info.Mode().Perm())
	}
	return nil
}
