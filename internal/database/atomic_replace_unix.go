//go:build !windows

package database

import "os"

// os.Rename is an atomic same-volume replacement on the Unix filesystems
// supported by Portico. The caller fsyncs the file before this operation and
// the parent directory after it.
func replaceFileAtomicallyOnce(source, target string) error {
	return os.Rename(source, target)
}
