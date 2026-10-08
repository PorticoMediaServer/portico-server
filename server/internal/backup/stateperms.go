package backup

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// errExposedEntry aborts the permissions walk at the first exposed entry.
var errExposedEntry = errors.New("backup: state entry is accessible by other users")

// CheckPermissions reports whether the state folder or any file under it is
// accessible by other users. Symlinks are never followed. A loose tree never
// refuses to start: the console warns and offers FixPermissions instead.
func CheckPermissions(state string) (bool, error) {
	err := filepath.WalkDir(state, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if !info.Mode().IsDir() && !info.Mode().IsRegular() {
			return nil
		}
		if info.Mode().Perm()&0077 != 0 {
			return errExposedEntry
		}
		return nil
	})
	if errors.Is(err, errExposedEntry) {
		return true, nil
	}
	return false, err
}

// FixPermissions removes group and other permission bits from the state
// folder and its files, keeping every owner bit: a 0755 helper keeps its
// owner execute bit, a 0644 file becomes 0600. Only Portico's own state
// folder is ever touched, and only on the owner's explicit action.
func FixPermissions(state string) error {
	return filepath.WalkDir(state, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return nil
		}
		if !info.Mode().IsDir() && !info.Mode().IsRegular() {
			return nil
		}
		if info.Mode().Perm()&0077 == 0 {
			return nil
		}
		return os.Chmod(path, info.Mode().Perm()&^0077)
	})
}
