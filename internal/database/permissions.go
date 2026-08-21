package database

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

// preparePrivateDataPaths establishes the filesystem boundary before SQLite
// or any service worker can create state. The platform implementation applies
// POSIX modes or verifies a Windows ACL; callers never use chmod as a fake
// Windows security primitive.
func preparePrivateDataPaths(cfg config.Config) error {
	appData := strings.TrimSpace(cfg.AppDataDir)
	if appData == "" {
		return fmt.Errorf("app data directory is required")
	}

	appOwnedDirectories := []string{
		appData,
		filepath.Join(appData, "restore"),
		filepath.Join(appData, "keys"),
		filepath.Join(appData, "secrets"),
		filepath.Join(appData, "diagnostics"),
	}
	for _, directory := range uniqueCleanPaths(appOwnedDirectories) {
		if err := enforcePrivateDirectory(directory); err != nil {
			return err
		}
	}

	// Backup and transcode roots may be explicitly mounted or shared by an
	// operator. Do not chmod those external roots. Backup artifacts themselves
	// are still protected when they are app-owned; transcode media is treated as
	// external content and is not rewritten by the privacy boundary.
	for _, externalRoot := range []string{cfg.BackupDir, cfg.TranscodeDir} {
		if strings.TrimSpace(externalRoot) == "" {
			continue
		}
		clean := filepath.Clean(externalRoot)
		if pathWithin(appData, clean) {
			if err := enforcePrivateDirectory(clean); err != nil {
				return err
			}
		} else if err := prepareExternalDirectory(clean); err != nil {
			return err
		}
	}
	if !isSQLiteMemoryPath(cfg.DatabasePath) {
		databaseDirectory := filepath.Dir(cfg.DatabasePath)
		if pathWithin(appData, databaseDirectory) {
			if err := enforcePrivateDirectory(databaseDirectory); err != nil {
				return err
			}
		} else if err := prepareExternalDirectory(databaseDirectory); err != nil {
			return err
		}
	}

	files := []string{}
	if !isSQLiteMemoryPath(cfg.DatabasePath) {
		files = append(files, cfg.DatabasePath)
	}
	if strings.TrimSpace(cfg.ConfigPath) != "" {
		files = append(files, cfg.ConfigPath)
	}
	if strings.TrimSpace(cfg.LogFilePath) != "" {
		files = append(files, cfg.LogFilePath)
	}
	for _, file := range uniqueCleanPaths(files) {
		if pathWithin(appData, file) {
			if err := enforcePrivateFile(file); err != nil {
				return err
			}
		} else if err := enforcePrivateFileNoParent(file); err != nil {
			return err
		}
	}

	// SQLite and supervised restore use sidecars that are created after the
	// initial path preparation. Verify existing artifacts without creating
	// empty WAL/SHM files, then repeat this check after SQLite opens.
	for _, artifact := range sqliteLifecycleArtifacts(cfg.DatabasePath, appData) {
		if err := enforcePrivateExistingFile(artifact); err != nil {
			return err
		}
	}
	// Do not enumerate a backup root during critical startup. External roots
	// may contain millions of operator-managed objects or a remote filesystem;
	// each artifact is validated at create/list/select time instead.
	return nil
}

func enforcePrivateSQLiteArtifacts(cfg config.Config) error {
	for _, artifact := range sqliteLifecycleArtifacts(cfg.DatabasePath, cfg.AppDataDir) {
		if err := enforcePrivateExistingFile(artifact); err != nil {
			return err
		}
	}
	return nil
}

func uniqueCleanPaths(paths []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(paths))
	for _, value := range paths {
		value = strings.TrimSpace(value)
		if value == "" || value == "." {
			continue
		}
		clean := filepath.Clean(value)
		if _, seen := seen[clean]; seen {
			continue
		}
		seen[clean] = struct{}{}
		result = append(result, clean)
	}
	return result
}

func pathWithin(root, candidate string) bool {
	rootAbs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return false
	}
	candidateAbs, err := filepath.Abs(filepath.Clean(candidate))
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)))
}

func sqliteLifecycleArtifacts(databasePath, appData string) []string {
	if isSQLiteMemoryPath(databasePath) {
		return []string{}
	}
	artifacts := []string{
		databasePath + "-wal",
		databasePath + "-shm",
		databasePath + "-journal",
		databasePath + ".restore-tmp",
		filepath.Join(appData, "restore-pending.db"),
		filepath.Join(appData, "restore-pending.json"),
	}
	restoreDir := filepath.Join(appData, "restore")
	entries, err := os.ReadDir(restoreDir)
	if err == nil {
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".pre-restore.db") || strings.Contains(entry.Name(), ".pre-restore.db.retry-") || strings.Contains(entry.Name(), ".pre-restore.db-retry-") {
				artifacts = append(artifacts, filepath.Join(restoreDir, entry.Name()))
			}
		}
	}
	return uniqueCleanPaths(artifacts)
}

func isSQLiteMemoryPath(path string) bool {
	trimmed := strings.TrimSpace(path)
	return trimmed == ":memory:" || strings.HasPrefix(trimmed, "file::memory:")
}
