//go:build darwin

package storage

import (
	"fmt"
	"os"
	"portico.local/server/internal/mediasource"
	"syscall"
)

func localVersionEvidence(info os.FileInfo, scope string) (mediasource.Evidence, error) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() {
		return mediasource.Evidence{}, mediasource.ErrIdentityRequired
	}
	return mediasource.Evidence{Kind: mediasource.LocalRevision, Scope: scope, Object: fmt.Sprintf("dev:%d:ino:%d", st.Dev, st.Ino), Revision: fmt.Sprintf("%d:%d:%d", info.ModTime().UnixNano(), st.Ctimespec.Sec, st.Ctimespec.Nsec), Size: info.Size(), NoInPlaceMutation: true}, nil
}
