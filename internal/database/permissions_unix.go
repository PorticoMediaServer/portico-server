//go:build !windows

package database

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func enforcePrivateDirectory(path string) error {
	if err := verifyTrustedOwnedAncestor(path); err != nil {
		return err
	}
	if err := rejectSymlinkComponents(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create private directory %s: %w", path, err)
	}
	if err := rejectSymlinkComponents(path); err != nil {
		return err
	}
	if err := verifyTrustedOwnedAncestor(path); err != nil {
		return err
	}
	// The trusted, non-writable ancestor invariant above makes component swaps
	// unavailable to an unprivileged peer. The final descriptor is still opened
	// with O_NOFOLLOW and used for chmod/stat, while mount-specific external-root
	// traversal remains an explicit OS integration-gate responsibility.
	directory, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return fmt.Errorf("open private directory %s without following symlinks: %w", path, err)
	}
	defer directory.Close()
	if err := directory.Chmod(0o700); err != nil {
		return fmt.Errorf("private directory mode enforcement unavailable for %s: %w", path, err)
	}
	info, err := directory.Stat()
	if err != nil {
		return fmt.Errorf("stat private directory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("private directory %s is not a real directory", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("private directory %s is readable by group or other users", path)
	}
	return nil
}

func enforcePrivateDirectoryWith(path string, chmod func(string, fs.FileMode) error) error {
	if err := verifyTrustedOwnedAncestor(path); err != nil {
		return err
	}
	if err := rejectSymlinkComponents(path); err != nil {
		return err
	}
	if err := verifyTrustedOwnedAncestor(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o700); err != nil {
		return fmt.Errorf("create private directory %s: %w", path, err)
	}
	if err := rejectSymlinkComponents(path); err != nil {
		return err
	}
	if err := verifyTrustedOwnedAncestor(path); err != nil {
		return err
	}
	if err := chmod(path, 0o700); err != nil {
		return fmt.Errorf("private directory mode enforcement unavailable for %s: %w", path, err)
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("stat private directory %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("private directory %s is not a real directory", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("private directory %s is readable by group or other users", path)
	}
	return nil
}

func enforcePrivateFile(path string) error {
	// As with directory creation, a portable Go path walk cannot hold every
	// parent descriptor while MkdirAll creates missing components. The final
	// open uses O_NOFOLLOW and all permission repair/stat work is descriptor
	// based, so a component race cannot turn this into chmod of a final
	// symlink target; symlinked parents are rejected before and after creation.
	if err := verifyTrustedOwnedAncestor(filepath.Dir(path)); err != nil {
		return err
	}
	if err := rejectSymlinkComponents(filepath.Dir(path)); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create private file directory for %s: %w", path, err)
	}
	return enforcePrivateFileOpened(path)
}

func enforcePrivateFileNoParent(path string) error {
	if err := validatePrivateFileParent(path); err != nil {
		return err
	}
	return enforcePrivateFileOpened(path)
}

// validatePrivateFileParent checks only the trusted parent and path
// components. It deliberately does not create the final file: O_EXCL callers
// must perform the single creation operation themselves.
func validatePrivateFileParent(path string) error {
	if err := verifyTrustedOwnedAncestor(filepath.Dir(path)); err != nil {
		return err
	}
	if err := rejectSymlinkComponents(filepath.Dir(path)); err != nil {
		return err
	}
	return nil
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

func enforcePrivateFileOpened(path string) error {
	if err := verifyTrustedOwnedAncestor(filepath.Dir(path)); err != nil {
		return err
	}
	if err := rejectSymlinkComponents(path); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0o600)
	if err != nil {
		return fmt.Errorf("create private file %s: %w", path, err)
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("private file mode enforcement unavailable for %s: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat private file %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() {
		return fmt.Errorf("private file %s is not a regular file", path)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("private file %s is readable by group or other users", path)
	}
	return nil
}

func enforcePrivateExistingFile(path string) error {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect private artifact %s: %w", path, err)
	}
	return enforcePrivateFileOpened(path)
}

func enforcePrivateExistingFileNoParent(path string) error {
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return fmt.Errorf("inspect private artifact %s: %w", path, err)
	}
	return enforcePrivateFileNoParent(path)
}

func verifyExternalSensitiveArtifact(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect external sensitive artifact %s: %w", path, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("external sensitive artifact %s is a symlink", path)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("external sensitive artifact %s is not a regular file", path)
	}
	return nil
}

func prepareExternalDirectory(path string) error {
	if err := rejectSymlinkComponents(path); err != nil {
		return err
	}
	if err := os.MkdirAll(path, 0o750); err != nil {
		return fmt.Errorf("create external storage directory %s: %w", path, err)
	}
	if err := rejectSymlinkComponents(path); err != nil {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("external storage path %s is not a directory", path)
	}
	return nil
}

// verifyTrustedOwnedAncestor closes the portable parent-swap gap for every
// app-owned mutation. Once the nearest existing ancestor is non-writable by
// group/other (or a sticky system directory such as /tmp), an unprivileged
// process cannot replace a missing component while MkdirAll creates it. The
// final operation still uses O_NOFOLLOW and descriptor-based stat/chmod.
// External media roots are not rewritten by this boundary and are validated
// only on artifact create/list/select; their mount-specific handle traversal
// remains an explicitly deferred OS integration gate.
func verifyTrustedOwnedAncestor(path string) error {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("resolve trusted private path %s: %w", path, err)
	}
	volume := filepath.VolumeName(absolute)
	remainder := strings.TrimLeft(absolute[len(volume):], string(filepath.Separator))
	current := volume + string(filepath.Separator)
	for _, component := range splitPathComponents(remainder) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect trusted private path %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if !trustedSystemPathAlias(current) {
				return fmt.Errorf("trusted private path %s is a symlink", current)
			}
			info, err = os.Stat(current)
			if err != nil {
				return fmt.Errorf("inspect trusted private alias %s: %w", current, err)
			}
		}
		if !info.IsDir() {
			return fmt.Errorf("trusted private path %s is not a directory", current)
		}
		if info.Mode().Perm()&0o022 != 0 && info.Mode()&os.ModeSticky == 0 {
			return fmt.Errorf("trusted private ancestor %s is writable by group or other users", current)
		}
	}
	return nil
}

func rejectSymlinkComponents(path string) error {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("resolve private path %s: %w", path, err)
	}
	volume := filepath.VolumeName(absolute)
	remainder := absolute[len(volume):]
	current := volume + string(filepath.Separator)
	for _, component := range splitPathComponents(remainder) {
		current = filepath.Join(current, component)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("inspect private path %s: %w", current, err)
		}
		if info.Mode()&os.ModeSymlink != 0 && !trustedSystemPathAlias(current) {
			return fmt.Errorf("private path %s is a symlink", current)
		}
	}
	return nil
}

func trustedSystemPathAlias(path string) bool {
	clean := filepath.Clean(path)
	return clean == "/var" || clean == "/tmp"
}

func splitPathComponents(path string) []string {
	trimmed := strings.TrimLeft(path, string(filepath.Separator))
	if trimmed == "" {
		return nil
	}
	parts := strings.Split(trimmed, string(filepath.Separator))
	result := make([]string, 0, len(parts))
	for _, component := range parts {
		if component != "" && component != "." {
			result = append(result, component)
		}
	}
	return result
}

func openRegularFileForRead(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
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
	return os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY|syscall.O_NOFOLLOW, 0o600)
}
