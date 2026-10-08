//go:build !darwin && !linux && !windows

package administration

import "os"

func renameExclusive(from, to string) error {
	if err := os.Link(from, to); err != nil {
		return err
	}
	return os.Remove(from)
}
