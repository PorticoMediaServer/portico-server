//go:build !unix

package mounts

import "os"

// openConfigFile has no non-blocking flag on this platform. The caller still
// checks the opened descriptor's mode before reading, which refuses a device or
// directory; a named pipe may open normally here.
func openConfigFile(path string) (*os.File, error) {
	return os.Open(path)
}
