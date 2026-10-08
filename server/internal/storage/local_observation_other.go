//go:build !darwin && !linux

package storage

import "os"

func observedObjectBinding(os.FileInfo) (string, string, error) {
	return "", "", ErrObservedSourceUnsupported
}
