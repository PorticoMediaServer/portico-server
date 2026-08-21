//go:build windows

package database

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// MoveFileEx with REPLACE_EXISTING is the Windows equivalent of the Unix
// same-volume atomic replacement used for the durable journal. Native Windows
// CI must exercise the first update and replacement while the old target is
// present; os.Rename alone is not a portable replace primitive on Windows.
func replaceFileAtomicallyOnce(source, target string) error {
	sourceName, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	targetName, err := windows.UTF16PtrFromString(target)
	if err != nil {
		return err
	}
	if err := windows.MoveFileEx(sourceName, targetName, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return fmt.Errorf("replace durable restore file: %w", err)
	}
	return nil
}
