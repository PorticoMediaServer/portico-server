//go:build windows

package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func enforcePrivateDirectory(path string) error {
	if err := rejectWindowsReparsePoint(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create private directory %s: %w", path, err)
	}
	return applyAndVerifyWindowsACL(path, true)
}

func enforcePrivateFile(path string) error {
	if err := rejectWindowsReparseComponents(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create private file directory for %s: %w", path, err)
	}
	file, err := openWindowsPrivateFile(path)
	if err != nil {
		return fmt.Errorf("create private file %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close private file %s: %w", path, err)
	}
	return applyAndVerifyWindowsACL(path, false)
}

func enforcePrivateFileNoParent(path string) error {
	if err := validatePrivateFileParent(path); err != nil {
		return err
	}
	return enforcePrivateFile(path)
}

func validatePrivateFileParent(path string) error {
	return rejectWindowsReparseComponents(filepath.Dir(path))
}

func preparePrivateFileForCreate(path string) error {
	if err := validatePrivateFileParent(path); err != nil {
		return err
	}
	if _, err := os.Lstat(path); err == nil {
		return os.ErrExist
	} else if !os.IsNotExist(err) {
		return err
	}
	return nil
}

func enforcePrivateExistingFile(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect private artifact %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
		return fmt.Errorf("private artifact %s is not a regular file", path)
	}
	return applyAndVerifyWindowsACL(path, false)
}

func enforcePrivateExistingFileNoParent(path string) error {
	if err := rejectWindowsReparseComponents(filepath.Dir(path)); err != nil {
		return err
	}
	return enforcePrivateExistingFile(path)
}

func verifyExternalSensitiveArtifact(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect external sensitive artifact %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
		return fmt.Errorf("external sensitive artifact %s is not a regular file", path)
	}
	return nil
}

func prepareExternalDirectory(path string) error {
	if err := rejectWindowsReparseComponents(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o750); err != nil {
		return fmt.Errorf("create external storage directory %s: %w", path, err)
	}
	if err := rejectWindowsReparseComponents(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("external storage path %s is not a directory", path)
	}
	return nil
}

func rejectWindowsReparsePoint(path string) error {
	info, err := os.Lstat(path)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("private path %s is a symlink", path)
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect private path %s: %w", path, err)
	}
	if err != nil {
		return nil
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("encode private path %s: %w", path, err)
	}
	attributes, err := windows.GetFileAttributes(name)
	if err != nil {
		return fmt.Errorf("inspect Windows attributes for %s: %w", path, err)
	}
	if attributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("private path %s is a reparse point", path)
	}
	return nil
}

func rejectWindowsReparseComponents(path string) error {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("resolve private path %s: %w", path, err)
	}
	volume := filepath.VolumeName(absolute)
	remainder := strings.TrimLeft(absolute[len(volume):], `\\/`)
	current := volume + string(filepath.Separator)
	for _, component := range strings.FieldsFunc(remainder, func(r rune) bool { return r == '\\' || r == '/' }) {
		current = filepath.Join(current, component)
		if err := rejectWindowsReparsePoint(current); err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
	}
	return nil
}

func applyAndVerifyWindowsACL(path string, directory bool) error {
	current, err := currentWindowsPrincipal()
	if err != nil {
		return fmt.Errorf("resolve Windows service principal for %s: %w", path, err)
	}
	system, err := windows.CreateWellKnownSid(windows.WinLocalSystemSid)
	if err != nil {
		return fmt.Errorf("resolve Windows SYSTEM principal for %s: %w", path, err)
	}
	admins, err := windows.CreateWellKnownSid(windows.WinBuiltinAdministratorsSid)
	if err != nil {
		return fmt.Errorf("resolve Windows Administrators principal for %s: %w", path, err)
	}
	inheritance := uint32(0)
	if directory {
		inheritance = windows.OBJECT_INHERIT_ACE | windows.CONTAINER_INHERIT_ACE
	}
	principals := uniqueWindowsACLPrincipals(current, system, admins)
	entries := make([]windows.EXPLICIT_ACCESS, 0, len(principals))
	for _, principal := range principals {
		entries = append(entries, privateWindowsAccess(principal.sid, principal.trusteeType, inheritance))
	}
	acl, err := windows.ACLFromEntries(entries, nil)
	if err != nil {
		return fmt.Errorf("build protected Windows ACL for %s: %w", path, err)
	}
	if err := windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil,
		nil,
		acl,
		nil,
	); err != nil {
		return fmt.Errorf("apply protected Windows ACL for %s: %w", path, err)
	}
	if err := verifyWindowsPrivateACL(path, principals...); err != nil {
		return err
	}
	return nil
}

type windowsACLPrincipal struct {
	sid         *windows.SID
	trusteeType windows.TRUSTEE_TYPE
}

// LocalSystem services commonly run with the same token SID as the current
// process. ACL policy is identity-based, not role-label-based: deduplicate the
// explicit entries and validate each unique SID independently so a protected
// DACL never rejects its own SYSTEM service process.
func uniqueWindowsACLPrincipals(current, system, admins *windows.SID) []windowsACLPrincipal {
	principals := make([]windowsACLPrincipal, 0, 3)
	seen := map[string]struct{}{}
	for _, candidate := range []windowsACLPrincipal{
		{sid: current, trusteeType: windows.TRUSTEE_IS_USER},
		{sid: system, trusteeType: windows.TRUSTEE_IS_USER},
		{sid: admins, trusteeType: windows.TRUSTEE_IS_GROUP},
	} {
		if candidate.sid == nil {
			continue
		}
		key := candidate.sid.String()
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		principals = append(principals, candidate)
	}
	return principals
}

func privateWindowsAccess(sid *windows.SID, trusteeType windows.TRUSTEE_TYPE, inheritance uint32) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: windows.GENERIC_ALL,
		AccessMode:        windows.GRANT_ACCESS,
		Inheritance:       inheritance,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeType:  trusteeType,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}

func currentWindowsPrincipal() (*windows.SID, error) {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return nil, err
	}
	defer token.Close()
	user, err := token.GetTokenUser()
	if err != nil {
		return nil, err
	}
	return user.User.Sid, nil
}

func verifyWindowsPrivateACL(path string, principals ...windowsACLPrincipal) error {
	descriptor, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read Windows ACL for %s: %w", path, err)
	}
	dacl, _, err := descriptor.DACL()
	if err != nil || dacl == nil {
		if err != nil {
			return fmt.Errorf("read Windows DACL for %s: %w", path, err)
		}
		return fmt.Errorf("Windows DACL for %s is unavailable", path)
	}
	effectiveAllow := map[string]windows.ACCESS_MASK{}
	for index := uint32(0); index < uint32(dacl.AceCount); index++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, index, &ace); err != nil {
			return fmt.Errorf("read Windows ACE %d for %s: %w", index, path, err)
		}
		// Deny ACEs are not grants. Only explicit ACCESS_ALLOWED ACEs are
		// considered when checking whether a broad principal can read secrets.
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			continue
		}
		entrySID := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		mask := ace.Mask
		if broadWindowsPrincipal(entrySID) && mask != 0 {
			return fmt.Errorf("Windows ACL for %s grants access to broad principal %s", path, entrySID.String())
		}
		key := entrySID.String()
		for _, principal := range principals {
			if entrySID.Equals(principal.sid) {
				effectiveAllow[key] |= mask
			}
		}
	}
	for _, principal := range principals {
		key := principal.sid.String()
		if !windowsMaskHasFullAccess(effectiveAllow[key]) {
			return fmt.Errorf("Windows ACL for %s is missing full access for SID %s", path, key)
		}
	}
	return nil
}

func windowsMaskHasFullAccess(mask windows.ACCESS_MASK) bool {
	required := windows.ACCESS_MASK(windows.FILE_GENERIC_READ | windows.FILE_GENERIC_WRITE | windows.FILE_GENERIC_EXECUTE | windows.DELETE)
	return mask&windows.GENERIC_ALL == windows.GENERIC_ALL || mask&required == required
}

func broadWindowsPrincipal(sid *windows.SID) bool {
	for _, value := range []string{
		"S-1-1-0",      // Everyone
		"S-1-5-7",      // Anonymous Logon
		"S-1-5-11",     // Authenticated Users
		"S-1-5-32-545", // Built-in Users
	} {
		candidate, err := windows.StringToSid(value)
		if err == nil && sid.Equals(candidate) {
			return true
		}
	}
	return false
}

func openRegularFileForRead(path string) (*os.File, error) {
	if err := rejectWindowsReparseComponents(path); err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(handle), path)
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		_ = file.Close()
		return nil, fmt.Errorf("path is not a regular file")
	}
	return file, nil
}

func openPrivateFileForWrite(path string) (*os.File, error) {
	if err := rejectWindowsReparseComponents(path); err != nil {
		return nil, err
	}
	// CREATE_NEW must receive the protected DACL in the CreateFile call. A
	// post-create ACL repair leaves a window in which an install temp or marker
	// beside an operator-managed database inherits that directory's permissions.
	current, err := currentWindowsPrincipal()
	if err != nil {
		return nil, fmt.Errorf("resolve Windows service principal: %w", err)
	}
	securityDescriptor, err := windows.SecurityDescriptorFromString(fmt.Sprintf("D:P(A;;FA;;;%s)(A;;FA;;;SY)(A;;FA;;;BA)", current.String()))
	if err != nil {
		return nil, fmt.Errorf("build private restore DACL: %w", err)
	}
	securityAttributes := &windows.SecurityAttributes{
		Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
		SecurityDescriptor: securityDescriptor,
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		securityAttributes,
		windows.CREATE_NEW,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}

func openWindowsPrivateFile(path string) (*os.File, error) {
	if err := rejectWindowsReparseComponents(path); err != nil {
		return nil, err
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	handle, err := windows.CreateFile(
		name,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_ALWAYS,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_OPEN_REPARSE_POINT,
		0,
	)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(handle), path), nil
}
