//go:build windows

package database

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// Native Windows execution must prove the protected DACL rather than treating
// Unix mode bits as a privacy assertion. This helper is exercised by the named
// Windows CI/runtime gate; cross-compilation only proves the code path builds.
func verifyPrivateRestoreArtifactForTest(path string) error {
	current, err := currentWindowsPrincipal()
	if err != nil {
		return err
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return err
	}
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return err
	}
	if err := verifyWindowsPrivateACL(path,
		windowsACLPrincipal{sid: current, trusteeType: windows.TRUSTEE_IS_USER},
		windowsACLPrincipal{sid: system, trusteeType: windows.TRUSTEE_IS_USER},
		windowsACLPrincipal{sid: admins, trusteeType: windows.TRUSTEE_IS_GROUP},
	); err != nil {
		return fmt.Errorf("protected DACL verification failed: %w", err)
	}
	return nil
}
