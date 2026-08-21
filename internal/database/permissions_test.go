package database

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestPrivateDataPathsCreateAndProtectReleaseState(t *testing.T) {
	appData := t.TempDir()
	databasePath := filepath.Join(appData, "nested", "portico.db")
	backupDir := filepath.Join(appData, "backups")
	transcodeDir := filepath.Join(appData, "transcodes")
	cfg := config.Config{
		AppDataDir:   appData,
		DatabasePath: databasePath,
		BackupDir:    backupDir,
		TranscodeDir: transcodeDir,
		ConfigPath:   filepath.Join(appData, "portico.config.json"),
		LogFilePath:  filepath.Join(appData, "logs", "server.log"),
	}
	if err := preparePrivateDataPaths(cfg); err != nil {
		t.Fatalf("prepare private paths: %v", err)
	}
	for _, directory := range []string{appData, filepath.Dir(databasePath), backupDir, transcodeDir, filepath.Join(appData, "restore"), filepath.Join(appData, "keys"), filepath.Join(appData, "secrets"), filepath.Join(appData, "diagnostics")} {
		info, err := os.Stat(directory)
		if err != nil {
			t.Fatalf("stat private directory %s: %v", directory, err)
		}
		if !info.IsDir() {
			t.Fatalf("private path %s is not a directory", directory)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o700 {
			t.Fatalf("private directory %s mode=%#o want 0700", directory, info.Mode().Perm())
		}
	}
	for _, file := range []string{databasePath, cfg.ConfigPath, cfg.LogFilePath} {
		info, err := os.Stat(file)
		if err != nil {
			t.Fatalf("stat private file %s: %v", file, err)
		}
		if info.IsDir() {
			t.Fatalf("private file %s is a directory", file)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("private file %s mode=%#o want 0600", file, info.Mode().Perm())
		}
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(backupDir, 0o755); err != nil {
			t.Fatalf("make directory unsafe for regression check: %v", err)
		}
		if err := enforcePrivateDirectory(backupDir); err != nil {
			t.Fatalf("repair unsafe directory: %v", err)
		}
		info, err := os.Stat(backupDir)
		if err != nil || info.Mode().Perm() != 0o700 {
			t.Fatalf("repaired directory info=%v err=%v", info, err)
		}
	}
}

func TestPrivateDataPathsProtectLifecycleArtifactsAndRejectSymlinks(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX symlink and mode assertions are covered by the Windows-native ACL test")
	}
	appData := t.TempDir()
	externalRoot := t.TempDir()
	externalBackup := filepath.Join(externalRoot, "backups")
	externalTranscode := filepath.Join(externalRoot, "transcodes")
	if err := os.MkdirAll(externalBackup, 0o755); err != nil {
		t.Fatalf("create external backup root: %v", err)
	}
	if err := os.MkdirAll(externalTranscode, 0o755); err != nil {
		t.Fatalf("create external transcode root: %v", err)
	}
	externalArtifact := filepath.Join(externalBackup, "portico-test.db")
	if err := os.WriteFile(externalArtifact, []byte("operator-managed backup"), 0o644); err != nil {
		t.Fatalf("create external backup artifact: %v", err)
	}
	databasePath := filepath.Join(appData, "nested", "portico.db")
	cfg := config.Config{
		AppDataDir:   appData,
		DatabasePath: databasePath,
		BackupDir:    externalBackup,
		TranscodeDir: externalTranscode,
		ConfigPath:   filepath.Join(appData, "portico.config.json"),
		LogFilePath:  filepath.Join(appData, "logs", "server.log"),
	}
	if err := preparePrivateDataPaths(cfg); err != nil {
		t.Fatalf("prepare private paths with external roots: %v", err)
	}
	for _, path := range []string{externalBackup, externalTranscode} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("stat external root %s: %v", path, err)
		}
		if info.Mode().Perm() != 0o755 {
			t.Fatalf("external root %s was rewritten: mode=%#o", path, info.Mode().Perm())
		}
	}
	if info, err := os.Stat(externalArtifact); err != nil {
		t.Fatalf("stat external artifact: %v", err)
	} else if info.Mode().Perm() != 0o644 {
		t.Fatalf("external artifact was chmodded despite operator-managed policy: mode=%#o", info.Mode().Perm())
	}

	preRestore := filepath.Join(appData, "restore", "restore-20260805-010203.pre-restore.db")
	for _, artifact := range []string{
		databasePath + "-wal",
		databasePath + "-shm",
		databasePath + "-journal",
		databasePath + ".restore-tmp",
		filepath.Join(appData, "restore-pending.db"),
		filepath.Join(appData, "restore-pending.json"),
		preRestore,
	} {
		if err := os.WriteFile(artifact, []byte("sensitive lifecycle fixture"), 0o644); err != nil {
			t.Fatalf("create lifecycle artifact %s: %v", artifact, err)
		}
	}
	if err := enforcePrivateSQLiteArtifacts(cfg); err != nil {
		t.Fatalf("enforce lifecycle artifacts: %v", err)
	}
	for _, artifact := range sqliteLifecycleArtifacts(databasePath, appData) {
		info, err := os.Stat(artifact)
		if err != nil {
			t.Fatalf("stat protected lifecycle artifact %s: %v", artifact, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("lifecycle artifact %s mode=%#o want 0600", artifact, info.Mode().Perm())
		}
	}

	target := filepath.Join(t.TempDir(), "outside.db")
	if err := os.WriteFile(target, []byte("must remain untouched"), 0o644); err != nil {
		t.Fatalf("create symlink target: %v", err)
	}
	symlinkRoot := filepath.Join(t.TempDir(), "app-data-link")
	if err := os.Symlink(filepath.Dir(target), symlinkRoot); err != nil {
		t.Fatalf("create app-data symlink: %v", err)
	}
	linkedCfg := cfg
	linkedCfg.AppDataDir = symlinkRoot
	linkedCfg.DatabasePath = filepath.Join(symlinkRoot, "portico.db")
	if err := preparePrivateDataPaths(linkedCfg); err == nil {
		t.Fatal("app-owned symlink root was accepted")
	}
	if info, err := os.Stat(target); err != nil {
		t.Fatalf("stat symlink target after rejection: %v", err)
	} else if info.Mode().Perm() != 0o644 {
		t.Fatalf("symlink target was modified: mode=%#o", info.Mode().Perm())
	}

	linkedDatabase := filepath.Join(appData, "linked.db")
	if err := os.Symlink(target, linkedDatabase); err != nil {
		t.Fatalf("create database symlink: %v", err)
	}
	linkedCfg = cfg
	linkedCfg.DatabasePath = linkedDatabase
	if err := preparePrivateDataPaths(linkedCfg); err == nil {
		t.Fatal("app-owned database symlink was accepted")
	}
}

func TestExternalBackupSymlinkIsRejectedWithoutRewritingRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX symlink policy is covered by the Windows-native ACL test")
	}
	appData := t.TempDir()
	external := t.TempDir()
	target := filepath.Join(t.TempDir(), "backup.db")
	if err := os.WriteFile(target, []byte("outside"), 0o644); err != nil {
		t.Fatalf("create target: %v", err)
	}
	if err := os.Symlink(target, filepath.Join(external, "portico-link.db")); err != nil {
		t.Fatalf("create backup symlink: %v", err)
	}
	rootBefore, err := os.Stat(external)
	if err != nil {
		t.Fatalf("stat external root before policy check: %v", err)
	}
	cfg := config.Config{
		AppDataDir:   appData,
		DatabasePath: filepath.Join(appData, "portico.db"),
		BackupDir:    external,
	}
	if err := preparePrivateDataPaths(cfg); err != nil {
		t.Fatalf("external backup root should remain available without enumeration: %v", err)
	}
	if err := ValidateRegularNonSymlinkFile(filepath.Join(external, "portico-link.db")); err == nil {
		t.Fatal("external backup symlink was accepted when selected")
	}
	if info, err := os.Stat(external); err != nil {
		t.Fatalf("stat external root: %v", err)
	} else if info.Mode().Perm() != rootBefore.Mode().Perm() {
		// The explicit external policy does not rewrite the existing root, so
		// this assertion guards against accidental chmod.
		t.Fatalf("external backup root was rewritten: mode=%#o", info.Mode().Perm())
	}
}
