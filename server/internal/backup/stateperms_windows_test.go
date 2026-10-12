//go:build windows

package backup

import (
	"errors"
	"testing"
)

func TestStatePermissionModesDoNotClaimToAssessOrRepairWindowsACLs(t *testing.T) {
	state := t.TempDir()
	if exposed, err := CheckPermissions(state); err != nil || exposed {
		t.Fatalf("synthetic modes reported ACL exposure: %t %v", exposed, err)
	}
	if err := FixPermissions(state); !errors.Is(err, ErrStatePermissionsUnsupported) {
		t.Fatalf("ACL repair claimed success: %v", err)
	}
}
