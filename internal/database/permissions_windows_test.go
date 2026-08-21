//go:build windows

package database

import (
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsPrivateACLIsAppliedAndVerified(t *testing.T) {
	root := filepath.Join(t.TempDir(), "portico")
	if err := enforcePrivateDirectory(root); err != nil {
		t.Fatalf("apply private directory ACL: %v", err)
	}
	file := filepath.Join(root, "portico.db")
	if err := enforcePrivateFile(file); err != nil {
		t.Fatalf("apply private file ACL: %v", err)
	}
	if info, err := os.Stat(file); err != nil || info.IsDir() {
		t.Fatalf("private file missing after ACL application: info=%v err=%v", info, err)
	}
}

func TestWindowsACLPrincipalSetDeduplicatesLocalSystem(t *testing.T) {
	current, err := windows.StringToSid("S-1-5-18")
	if err != nil {
		t.Fatalf("current SID: %v", err)
	}
	system, err := windows.StringToSid("S-1-5-18")
	if err != nil {
		t.Fatalf("system SID: %v", err)
	}
	admins, err := windows.StringToSid("S-1-5-32-544")
	if err != nil {
		t.Fatalf("administrators SID: %v", err)
	}
	principals := uniqueWindowsACLPrincipals(current, system, admins)
	if len(principals) != 2 {
		t.Fatalf("LocalSystem principal set length=%d want 2", len(principals))
	}
	seen := map[string]bool{}
	for _, principal := range principals {
		key := principal.sid.String()
		if seen[key] {
			t.Fatalf("duplicate SID in ACL principal set: %s", key)
		}
		seen[key] = true
	}
}
