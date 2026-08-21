//go:build !windows

package app

import (
	"errors"
	"syscall"
	"testing"
)

func TestStorageErrorClassificationUsesWrappedUnixErrno(t *testing.T) {
	for _, test := range []struct {
		err   error
		class string
	}{
		{err: syscall.ESTALE, class: "stale_handle"},
		{err: syscall.EIO, class: "io"},
		{err: syscall.EPERM, class: "permission"},
	} {
		wrapped := errors.Join(errors.New("storage failure"), test.err)
		if got := storageErrorClass(wrapped); got != test.class {
			t.Errorf("storageErrorClass(%v) = %q, expected %q", test.err, got, test.class)
		}
		if !storageErrorTransient(wrapped) {
			t.Errorf("storageErrorTransient(%v) = false", test.err)
		}
	}
}
