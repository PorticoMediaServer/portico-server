package database

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

// PreparePrivateDataPaths is the app boundary for creating or validating
// private lifecycle roots before a restore worker writes into them.
func PreparePrivateDataPaths(cfg config.Config) error {
	return preparePrivateDataPaths(cfg)
}

// EnforcePrivateSQLiteArtifacts rechecks existing SQLite and restore sidecars
// after a long-lived process has opened its database.
func EnforcePrivateSQLiteArtifacts(cfg config.Config) error {
	return enforcePrivateSQLiteArtifacts(cfg)
}

// CopyRestrictedFile copies a validated regular file into a no-follow,
// mode/ACL-repaired private target.
func CopyRestrictedFile(sourcePath, targetPath string) error {
	return CopyRestrictedFileSync(sourcePath, targetPath, RestoreMaxDatabaseBytes)
}

// WriteRestrictedFile writes a small private lifecycle marker without
// following a final-component symlink.
func WriteRestrictedFile(path string, body []byte) error {
	return WriteAtomicPrivateFile(path, body)
}

// ValidateRegularNonSymlinkFile is used before SQLite or a restore worker
// opens an operator-selected backup path.
func ValidateRegularNonSymlinkFile(path string) error {
	return requireRegularNonSymlinkFile(path)
}

func ValidateNoSymlinkPath(path string) error {
	return requireNoSymlinkPath(path)
}

// IsAppOwnedPath distinguishes private release state from operator-managed
// media/NAS roots. Automatic backup creation is only allowed in the former.
func IsAppOwnedPath(appData, candidate string) bool {
	return pathWithin(appData, candidate)
}

// ProtectCreatedFile applies the platform privacy primitive after a producer
// such as SQLite has created a file. Callers must remove the artifact if this
// returns an error; a chmod-less or inherited-ACL filesystem is never reported
// as a successful private artifact.
func ProtectCreatedFile(path string) error {
	return enforcePrivateExistingFile(path)
}

// PreparePrivateLogArtifacts repairs the active log and all legacy rotated
// files before the first record is emitted. Rotation calls the corresponding
// enforcement function again after renaming files.
func PreparePrivateLogArtifacts(path string, backups int) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create private log directory: %w", err)
	}
	return EnforcePrivateLogArtifacts(path, backups)
}

func EnforcePrivateLogArtifacts(path string, backups int) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if err := enforcePrivateFileNoParent(path); err != nil {
		return err
	}
	for index := 1; index <= max(0, backups); index++ {
		if err := enforcePrivateExistingFileNoParent(path + "." + strconv.Itoa(index)); err != nil {
			return err
		}
	}
	return nil
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func copyRestrictedFile(sourcePath, targetPath string) error {
	return CopyRestrictedFileSync(sourcePath, targetPath, RestoreMaxDatabaseBytes)
}

func requireNoSymlinkPath(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return errors.New("path is a symlink")
	}
	return nil
}

func requireRegularNonSymlinkFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return errors.New("path is not a regular file")
	}
	return nil
}

func safeBackupName(name string) bool {
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, `/\`) {
		return false
	}
	return strings.HasPrefix(name, "portico-") && strings.HasSuffix(name, ".db") && !strings.Contains(name, ".manifest.")
}
