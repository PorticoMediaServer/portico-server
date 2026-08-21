//go:build !windows

package database

import (
	"io/fs"
	"os"
	"strings"
	"syscall"
	"testing"
)

func TestPrivateDirectoryFailsClosedWhenModeRepairIsUnsupported(t *testing.T) {
	path := t.TempDir() + "/private"
	err := enforcePrivateDirectoryWith(path, func(string, fs.FileMode) error {
		return syscall.ENOTSUP
	})
	if err == nil || !strings.Contains(err.Error(), "mode enforcement unavailable") {
		t.Fatalf("chmod-unsupported policy error=%v", err)
	}
	if info, statErr := os.Stat(path); statErr != nil || !info.IsDir() {
		t.Fatalf("unsupported-mode test did not leave a deterministic directory: info=%v err=%v", info, statErr)
	}
}

func TestPrivatePathsRejectWritableParentSwapBoundary(t *testing.T) {
	parent := t.TempDir() + "/writable-parent"
	if err := os.Mkdir(parent, 0o777); err != nil {
		t.Fatalf("create writable parent: %v", err)
	}
	if err := os.Chmod(parent, 0o777); err != nil {
		t.Fatalf("make parent writable: %v", err)
	}
	path := parent + "/private"
	err := enforcePrivateDirectory(path)
	if err == nil || !strings.Contains(err.Error(), "writable by group or other") {
		t.Fatalf("writable parent was accepted: %v", err)
	}
	if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
		t.Fatalf("rejected path was created despite trusted-ancestor failure: %v", statErr)
	}
}
