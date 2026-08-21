//go:build !windows

package mediatoolchain

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestValidateBundleRejectsSpecialFilesystemEntry(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"toolchain-manifest.json", "requirements.v1.json"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := syscall.Mkfifo(filepath.Join(root, "unexpected-pipe"), 0o600); err != nil {
		t.Skipf("create FIFO fixture: %v", err)
	}
	if err := validateBundleInventory(root, filepath.Join(root, "requirements.v1.json"), map[string]string{}); err == nil || !strings.Contains(err.Error(), "special file") {
		t.Fatalf("special filesystem entry error = %v", err)
	}
	if _, err := os.Lstat(filepath.Join(root, "unexpected-pipe")); err != nil {
		t.Fatal(err)
	}
}
