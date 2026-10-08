//go:build !darwin && !linux

package storage

import (
	"os"
	"portico.local/server/internal/mediasource"
)

// Windows and other OS object/revision implementations remain unqualified.
func localVersionEvidence(os.FileInfo, string) (mediasource.Evidence, error) {
	return mediasource.Evidence{}, mediasource.ErrIdentityRequired
}
