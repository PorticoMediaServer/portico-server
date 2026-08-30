package database

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestRestoreFileCreationPrimitivesUseOneOExclCreate(t *testing.T) {
	root := t.TempDir()
	appData := filepath.Join(root, "app-data")
	if err := os.MkdirAll(appData, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(appData, "source.db"), []byte("sqlite-source"), 0o600); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(appData, "copy.db")
	if err := CopyRestrictedFileSync(filepath.Join(appData, "source.db"), target, 1024); err != nil {
		t.Fatalf("fresh restricted copy: %v", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "sqlite-source" {
		t.Fatalf("copied bytes=%q err=%v", got, err)
	}
	if err := CopyRestrictedFileSync(filepath.Join(appData, "source.db"), target, 1024); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing copy error=%v, want os.ErrExist", err)
	}

	streamTarget := filepath.Join(appData, "stream.db")
	if _, err := WritePrivateStream(streamTarget, strings.NewReader("streamed"), 1024); err != nil {
		t.Fatalf("fresh private stream: %v", err)
	}
	if _, err := WritePrivateStream(streamTarget, strings.NewReader("replacement"), 1024); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing stream error=%v, want os.ErrExist", err)
	}

	marker := filepath.Join(appData, "marker.json")
	if err := WriteAtomicPrivateFile(marker, []byte(`{"phase":"staged"}`)); err != nil {
		t.Fatalf("fresh atomic marker: %v", err)
	}
	if err := WriteAtomicPrivateFile(marker, []byte(`{"phase":"complete"}`)); err != nil {
		t.Fatalf("atomic replacement of existing marker: %v", err)
	}

	if runtime.GOOS != "windows" {
		linkedParent := filepath.Join(root, "linked-parent")
		if err := os.Symlink(appData, linkedParent); err != nil {
			t.Fatal(err)
		}
		if err := CopyRestrictedFileSync(filepath.Join(appData, "source.db"), filepath.Join(linkedParent, "escape.db"), 1024); err == nil {
			t.Fatal("copy through symlinked parent was accepted")
		}
		outside := filepath.Join(root, "outside.db")
		if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
			t.Fatal(err)
		}
		for _, name := range []string{"copy-final.db", "stream-final.db", "snapshot-final.db"} {
			if err := os.Symlink(outside, filepath.Join(appData, name)); err != nil {
				t.Fatal(err)
			}
		}
		if err := CopyRestrictedFileSync(filepath.Join(appData, "source.db"), filepath.Join(appData, "copy-final.db"), 1024); err == nil {
			t.Fatal("copy to a symlinked final component was accepted")
		}
		if _, err := WritePrivateStream(filepath.Join(appData, "stream-final.db"), strings.NewReader("replacement"), 1024); err == nil {
			t.Fatal("stream to a symlinked final component was accepted")
		}
		if got, err := os.ReadFile(outside); err != nil || string(got) != "outside" {
			t.Fatalf("symlink target changed: %q err=%v", got, err)
		}
	}
}

func TestAtomicReplacementRetryPolicySimulatesWindowsSharingViolations(t *testing.T) {
	newPaths := func(t *testing.T) (string, string) {
		t.Helper()
		dir := t.TempDir()
		return filepath.Join(dir, "source.tmp"), filepath.Join(dir, "target.json")
	}
	transient := errors.New("simulated sharing violation")

	t.Run("transient success", func(t *testing.T) {
		source, target := newPaths(t)
		calls := 0
		restore := SetAtomicReplaceRetryForTest(func(string, string) error {
			calls++
			if calls < 3 {
				return transient
			}
			return nil
		}, func(err error) bool { return errors.Is(err, transient) })
		defer restore()
		if err := ReplaceFileAtomicallyContext(context.Background(), source, target); err != nil {
			t.Fatalf("transient replacement: %v", err)
		}
		if calls != 3 {
			t.Fatalf("transient replacement attempts=%d, want 3", calls)
		}
	})

	t.Run("exhausted retry", func(t *testing.T) {
		source, target := newPaths(t)
		calls := 0
		restore := SetAtomicReplaceRetryForTest(func(string, string) error {
			calls++
			return transient
		}, func(err error) bool { return errors.Is(err, transient) })
		defer restore()
		err := ReplaceFileAtomicallyContext(context.Background(), source, target)
		if !errors.Is(err, transient) {
			t.Fatalf("exhausted replacement error=%v, want transient cause", err)
		}
		if calls != 6 {
			t.Fatalf("exhausted replacement attempts=%d, want bounded 6", calls)
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		source, target := newPaths(t)
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		restore := SetAtomicReplaceRetryForTest(func(string, string) error {
			calls++
			cancel()
			return transient
		}, func(error) bool { return true })
		defer restore()
		err := ReplaceFileAtomicallyContext(ctx, source, target)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled replacement error=%v, want context.Canceled", err)
		}
		if calls != 1 {
			t.Fatalf("canceled replacement attempts=%d, want 1", calls)
		}
	})

	t.Run("permanent error is not retried", func(t *testing.T) {
		source, target := newPaths(t)
		permanent := errors.New("simulated permanent ACL failure")
		calls := 0
		restore := SetAtomicReplaceRetryForTest(func(string, string) error {
			calls++
			return permanent
		}, func(error) bool { return false })
		defer restore()
		err := ReplaceFileAtomicallyContext(context.Background(), source, target)
		if !errors.Is(err, permanent) {
			t.Fatalf("permanent replacement error=%v, want permanent cause", err)
		}
		if calls != 1 {
			t.Fatalf("permanent replacement attempts=%d, want 1", calls)
		}
	})
}

func TestCreateVerifiedDatabaseSnapshotFreshAndExistingTargets(t *testing.T) {
	t.Chdir("../..")
	appData := t.TempDir()
	cfg := config.Config{AppDataDir: appData, DatabasePath: filepath.Join(appData, "portico.db")}
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES ('snapshot-wal-marker', '"committed"', 'now') ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json`); err != nil {
		t.Fatalf("write WAL marker: %v", err)
	}

	target := filepath.Join(appData, "safety.db")
	validation, err := CreateVerifiedDatabaseSnapshot(context.Background(), db, target)
	if err != nil {
		t.Fatalf("fresh logical snapshot: %v", err)
	}
	if validation.SizeBytes <= 0 || validation.ChecksumSHA256 == "" || validation.IntegrityResult != "ok" {
		t.Fatalf("incomplete snapshot validation: %#v", validation)
	}
	if err := db.QueryRow(`SELECT value_json FROM settings WHERE key = 'snapshot-wal-marker'`).Scan(new(string)); err != nil {
		t.Fatalf("source WAL marker missing: %v", err)
	}
	standalone, err := sqlOpenReadOnlyForRestore(target)
	if err != nil {
		t.Fatal(err)
	}
	defer standalone.Close()
	var marker string
	if err := standalone.QueryRow(`SELECT value_json FROM settings WHERE key = 'snapshot-wal-marker'`).Scan(&marker); err != nil || marker != `"committed"` {
		t.Fatalf("standalone snapshot marker=%q err=%v", marker, err)
	}
	if _, err := CreateVerifiedDatabaseSnapshot(context.Background(), db, target); !errors.Is(err, os.ErrExist) {
		t.Fatalf("existing snapshot error=%v, want os.ErrExist", err)
	}
	if runtime.GOOS != "windows" {
		linkedTarget := filepath.Join(appData, "snapshot-linked.db")
		outside := filepath.Join(appData, "snapshot-outside.db")
		if err := os.WriteFile(outside, []byte("not a snapshot"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, linkedTarget); err != nil {
			t.Fatal(err)
		}
		if _, err := CreateVerifiedDatabaseSnapshot(context.Background(), db, linkedTarget); err == nil {
			t.Fatal("snapshot to a symlinked final component was accepted")
		}
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	partialTarget := filepath.Join(appData, "snapshot-canceled.db")
	if _, err := CreateVerifiedDatabaseSnapshot(canceled, db, partialTarget); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled snapshot error=%v, want context.Canceled", err)
	}
	if _, err := os.Lstat(partialTarget); !os.IsNotExist(err) {
		t.Fatalf("canceled snapshot left target: %v", err)
	}
}

func TestSafetySnapshotForExternalDatabaseUsesPrivateRestoreRoot(t *testing.T) {
	t.Chdir("../..")
	root := t.TempDir()
	appData := filepath.Join(root, "app-data")
	externalRoot := filepath.Join(root, "operator-db")
	if err := os.MkdirAll(externalRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{AppDataDir: appData, DatabasePath: filepath.Join(externalRoot, "portico.db")}
	if err := PreparePrivateDataPaths(cfg); err != nil {
		t.Fatalf("prepare external database paths: %v", err)
	}
	db, err := Open(cfg)
	if err != nil {
		t.Fatalf("open external database: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES ('external-snapshot-marker', '"committed"', 'now') ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json`); err != nil {
		t.Fatalf("write external WAL marker: %v", err)
	}

	operationID := "restore-external-root"
	target := CanonicalRestoreSafetyCopyPath(cfg, operationID)
	if filepath.Dir(target) != filepath.Join(appData, "restore") {
		t.Fatalf("safety target escaped private restore root: %s", target)
	}
	if strings.HasPrefix(filepath.Clean(target), filepath.Clean(externalRoot)+string(filepath.Separator)) {
		t.Fatalf("safety target is under external database root: %s", target)
	}
	if _, err := CreateVerifiedDatabaseSnapshot(context.Background(), db, target); err != nil {
		t.Fatalf("external-root safety snapshot: %v", err)
	}
	if err := verifyPrivateRestoreArtifactForTest(target); err != nil {
		t.Fatalf("safety snapshot privacy proof: %v", err)
	}
	if _, err := os.Stat(filepath.Join(externalRoot, filepath.Base(cfg.DatabasePath)+".pre-restore-"+operationID)); !os.IsNotExist(err) {
		t.Fatalf("external database root received a safety artifact: %v", err)
	}
	installTemp := CanonicalRestoreInstallPath(cfg, operationID)
	if err := CopyRestrictedFileSync(target, installTemp, RestoreMaxDatabaseBytes); err != nil {
		t.Fatalf("external-root install temp copy: %v", err)
	}
	if err := verifyPrivateRestoreArtifactForTest(installTemp); err != nil {
		t.Fatalf("external install temp privacy proof: %v", err)
	}
}

func TestRestoreExecutorLockDoesNotReexecuteTerminalOperation(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{AppDataDir: filepath.Join(root, "app-data"), DatabasePath: filepath.Join(root, "portico.db")}
	if err := os.MkdirAll(filepath.Join(cfg.AppDataDir, "restore"), 0o700); err != nil {
		t.Fatalf("create restore root: %v", err)
	}
	operation := RestoreOperation{
		Version: RestoreOperationVersion, OperationID: "restore-terminal-lock-test", BackupName: "backup.db",
		ActivePath: cfg.DatabasePath, StagedPath: CanonicalRestoreStagedPath(cfg, "restore-terminal-lock-test", false),
		SafetyCopyPath: CanonicalRestoreSafetyCopyPath(cfg, "restore-terminal-lock-test"), OldActivePath: CanonicalRestoreOldActivePath(cfg, "restore-terminal-lock-test"),
		InstallPath: CanonicalRestoreInstallPath(cfg, "restore-terminal-lock-test"), Phase: RestorePhaseStaged, State: RestorePhaseStaged,
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano), UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	if err := WriteRestoreOperation(cfg.AppDataDir, operation); err != nil {
		t.Fatalf("write restore marker: %v", err)
	}

	releaseWinner, err := AcquireRestoreExecutorLock(cfg)
	if err != nil {
		t.Fatalf("acquire winner executor lock: %v", err)
	}
	winner, claimed, err := ClaimRestoreOperationWithExecutorLock(cfg, operation.OperationID, "winner")
	if err != nil || !claimed || winner.ExecutorID != "winner" {
		releaseWinner()
		t.Fatalf("winner claim=%#v claimed=%v err=%v", winner, claimed, err)
	}

	secondAcquired := make(chan struct{})
	secondResult := make(chan struct {
		operation RestoreOperation
		claimed   bool
		err       error
	}, 1)
	go func() {
		release, acquireErr := AcquireRestoreExecutorLock(cfg)
		if acquireErr != nil {
			secondResult <- struct {
				operation RestoreOperation
				claimed   bool
				err       error
			}{err: acquireErr}
			return
		}
		close(secondAcquired)
		defer release()
		candidate, candidateClaimed, claimErr := ClaimRestoreOperationWithExecutorLock(cfg, operation.OperationID, "loser")
		secondResult <- struct {
			operation RestoreOperation
			claimed   bool
			err       error
		}{operation: candidate, claimed: candidateClaimed, err: claimErr}
	}()
	select {
	case <-secondAcquired:
		releaseWinner()
		t.Fatal("second executor acquired the lock before the winner released it")
	case <-time.After(50 * time.Millisecond):
	}

	winner.Phase, winner.State, winner.Progress = RestorePhaseComplete, RestorePhaseComplete, 100
	winner.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if err := WriteRestoreOperationOwned(cfg, winner, "winner"); err != nil {
		releaseWinner()
		t.Fatalf("write terminal marker: %v", err)
	}
	releaseWinner()

	result := <-secondResult
	if result.err != nil {
		t.Fatalf("loser claim error: %v", result.err)
	}
	if result.claimed {
		t.Fatalf("loser re-claimed terminal operation: %#v", result.operation)
	}
	if result.operation.Phase != RestorePhaseComplete || result.operation.State != RestorePhaseComplete {
		t.Fatalf("loser changed terminal marker: %#v", result.operation)
	}
}

func TestOptionalRenameUsesCollisionProofDiagnosticSibling(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.db")
	target := filepath.Join(root, "old-active.db")
	if err := os.WriteFile(source, []byte("new diagnostic set"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("pre-existing diagnostic set"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := optionalRenameWithStage("test-collision", source, target); err != nil {
		t.Fatalf("optional rename: %v", err)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "pre-existing diagnostic set" {
		t.Fatalf("pre-existing target changed: %q err=%v", got, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	var sibling string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "old-active.db.retry-") {
			sibling = filepath.Join(root, entry.Name())
			break
		}
	}
	if sibling == "" {
		t.Fatalf("collision sibling was not created; entries=%v", entries)
	}
	if got, err := os.ReadFile(sibling); err != nil || string(got) != "new diagnostic set" {
		t.Fatalf("collision sibling bytes=%q err=%v", got, err)
	}
}

func TestPruneRestoreHistoryRemovesCollisionDiagnosticSiblings(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{AppDataDir: filepath.Join(root, "app-data"), DatabasePath: filepath.Join(root, "portico.db")}
	if err := PreparePrivateDataPaths(cfg); err != nil {
		t.Fatalf("prepare private paths: %v", err)
	}
	now := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	operation := RestoreOperation{
		Version: RestoreOperationVersion, OperationID: "restore-history-collision", BackupName: "old.db",
		ActivePath: cfg.DatabasePath, StagedPath: CanonicalRestoreStagedPath(cfg, "restore-history-collision", false),
		SafetyCopyPath: CanonicalRestoreSafetyCopyPath(cfg, "restore-history-collision"),
		OldActivePath:  CanonicalRestoreOldActivePath(cfg, "restore-history-collision"),
		InstallPath:    CanonicalRestoreInstallPath(cfg, "restore-history-collision"),
		Phase:          RestorePhaseFailed, State: RestorePhaseFailed, Progress: 100,
		CreatedAt: now, UpdatedAt: now, CompletedAt: now,
	}
	if err := ArchiveRestoreOperation(cfg, operation); err != nil {
		t.Fatalf("archive history: %v", err)
	}
	collision := operation.OldActivePath + ".retry-random-000"
	interrupted := operation.SafetyCopyPath + ".restore-interrupted-" + operation.OperationID + "-retry-001"
	for _, path := range []string{collision, interrupted} {
		if err := os.WriteFile(path, []byte("diagnostic debris"), 0o600); err != nil {
			t.Fatalf("write debris %s: %v", path, err)
		}
	}
	removed, err := pruneRestoreHistory(cfg, 1, time.Hour)
	if err != nil {
		t.Fatalf("prune history: %v", err)
	}
	if removed != 1 {
		t.Fatalf("removed history count=%d, want 1", removed)
	}
	for _, path := range []string{collision, interrupted, filepath.Join(cfg.AppDataDir, "restore", "history", operation.OperationID+".json")} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("history cleanup left %s: %v", path, err)
		}
	}
}

func TestRecoveryRequiredMarkerRemainsNonTerminalAcrossRestartReconciliation(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{AppDataDir: filepath.Join(root, "app-data"), DatabasePath: filepath.Join(root, "portico.db")}
	if err := PreparePrivateDataPaths(cfg); err != nil {
		t.Fatalf("prepare private paths: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	operation := RestoreOperation{
		Version: RestoreOperationVersion, OperationID: "restore-recovery-restart", BackupName: "backup.db",
		ActivePath: cfg.DatabasePath, StagedPath: CanonicalRestoreStagedPath(cfg, "restore-recovery-restart", false),
		SafetyCopyPath: CanonicalRestoreSafetyCopyPath(cfg, "restore-recovery-restart"),
		OldActivePath:  CanonicalRestoreOldActivePath(cfg, "restore-recovery-restart"),
		InstallPath:    CanonicalRestoreInstallPath(cfg, "restore-recovery-restart"),
		Phase:          RestorePhaseInstalling, State: RestorePhaseInstalling, Progress: 70,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := WriteRestoreOperation(cfg.AppDataDir, operation); err != nil {
		t.Fatalf("write operation: %v", err)
	}
	if err := MarkRestoreRecoveryRequired(cfg, &operation, "restore_recovery_required"); err != nil {
		t.Fatalf("mark recovery required: %v", err)
	}
	if err := RecoverInterruptedRestoreBeforeOpen(cfg); err != nil {
		t.Fatalf("restart reconciliation: %v", err)
	}
	latest, err := ReadRestoreOperation(cfg.AppDataDir)
	if err != nil {
		t.Fatalf("read restart marker: %v", err)
	}
	if latest.State != RestoreStateRecoveryNeeded || latest.Phase == RestorePhaseComplete || latest.Phase == RestorePhaseFailed {
		t.Fatalf("recovery marker was normalized terminally: %#v", latest)
	}
}

func TestTamperedRestoreMarkerPathsFailClosedBeforeInstallRollbackOrRetention(t *testing.T) {
	root := t.TempDir()
	cfg := config.Config{AppDataDir: filepath.Join(root, "app-data"), DatabasePath: filepath.Join(root, "portico.db")}
	if err := PreparePrivateDataPaths(cfg); err != nil {
		t.Fatalf("prepare private paths: %v", err)
	}
	operationID := "restore-tampered-paths"
	now := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339Nano)
	operation := RestoreOperation{
		Version: RestoreOperationVersion, OperationID: operationID, BackupName: "backup.db",
		ActivePath: cfg.DatabasePath, StagedPath: CanonicalRestoreStagedPath(cfg, operationID, false),
		SafetyCopyPath: CanonicalRestoreSafetyCopyPath(cfg, operationID),
		OldActivePath:  CanonicalRestoreOldActivePath(cfg, operationID),
		InstallPath:    CanonicalRestoreInstallPath(cfg, operationID),
		Phase:          RestorePhaseFailed, State: RestorePhaseFailed, Progress: 100,
		CreatedAt: now, UpdatedAt: now, CompletedAt: now,
	}
	sibling := filepath.Join(root, "unrelated-sibling.db")
	if err := os.WriteFile(sibling, []byte("must remain untouched"), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, mutate := range []func(*RestoreOperation){
		func(value *RestoreOperation) { value.SafetyCopyPath = sibling },
		func(value *RestoreOperation) { value.OldActivePath = sibling },
		func(value *RestoreOperation) { value.InstallPath = sibling },
		func(value *RestoreOperation) { value.StagedPath = sibling },
	} {
		tampered := operation
		mutate(&tampered)
		if err := InstallRestoreOperation(cfg, &tampered); err == nil {
			t.Fatal("install accepted a tampered destructive path")
		}
		if err := RollbackRestoreOperation(cfg, &tampered, "restore_test", errors.New("test")); err == nil {
			t.Fatal("rollback accepted a tampered destructive path")
		}
		if err := ArchiveRestoreOperation(cfg, tampered); err == nil {
			t.Fatal("archive accepted a tampered destructive path")
		}
	}

	historyPath := filepath.Join(cfg.AppDataDir, "restore", "history", operationID+".json")
	if err := os.MkdirAll(filepath.Dir(historyPath), 0o700); err != nil {
		t.Fatalf("create history directory: %v", err)
	}
	tampered := operation
	tampered.SafetyCopyPath = sibling
	body, err := json.Marshal(tampered)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeAtomicPrivateFile(historyPath, body); err != nil {
		t.Fatalf("write tampered history: %v", err)
	}
	if _, err := pruneRestoreHistory(cfg, 0, time.Hour); err != nil {
		t.Fatalf("prune tampered history: %v", err)
	}
	if got, err := os.ReadFile(sibling); err != nil || string(got) != "must remain untouched" {
		t.Fatalf("tampered retention touched sibling: %q err=%v", got, err)
	}
}

func TestInterruptedRestoreDurablePhaseFaultMatrix(t *testing.T) {
	for _, test := range []struct {
		name           string
		phase          string
		mutation       bool
		wantState      string
		wantQuarantine bool
	}{
		{name: "phase marker before snapshot evidence", phase: RestorePhaseSafetyCopy, wantState: RestorePhaseFailed, wantQuarantine: true},
		{name: "install marker before snapshot evidence", phase: RestorePhaseInstalling, wantState: RestorePhaseFailed, wantQuarantine: true},
		{name: "active rename intent without snapshot evidence", phase: RestorePhaseInstalling, mutation: true, wantState: RestoreStateRecoveryNeeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			cfg := config.Config{AppDataDir: filepath.Join(root, "app-data"), DatabasePath: filepath.Join(root, "portico.db")}
			if err := PreparePrivateDataPaths(cfg); err != nil {
				t.Fatal(err)
			}
			operationID := "restore-phase-matrix-" + strings.ReplaceAll(test.name, " ", "-")
			now := time.Now().UTC().Format(time.RFC3339Nano)
			operation := RestoreOperation{
				Version: RestoreOperationVersion, OperationID: operationID, BackupName: "backup.db",
				ActivePath: cfg.DatabasePath, StagedPath: CanonicalRestoreStagedPath(cfg, operationID, false),
				SafetyCopyPath: CanonicalRestoreSafetyCopyPath(cfg, operationID),
				OldActivePath:  CanonicalRestoreOldActivePath(cfg, operationID), InstallPath: CanonicalRestoreInstallPath(cfg, operationID),
				Phase: test.phase, State: test.phase, Progress: 50, ActiveMutationStarted: test.mutation,
				CreatedAt: now, UpdatedAt: now,
			}
			if test.wantQuarantine {
				if err := os.WriteFile(operation.SafetyCopyPath, []byte("partial safety snapshot"), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if err := WriteRestoreOperation(cfg.AppDataDir, operation); err != nil {
				t.Fatal(err)
			}
			if err := RecoverInterruptedRestoreBeforeOpen(cfg); err != nil {
				t.Fatalf("phase recovery: %v", err)
			}
			latest, err := ReadRestoreOperation(cfg.AppDataDir)
			if err != nil {
				t.Fatal(err)
			}
			if latest.State != test.wantState {
				t.Fatalf("phase %s state=%q, want %q; marker=%#v", test.phase, latest.State, test.wantState, latest)
			}
			if test.wantQuarantine {
				if _, err := os.Lstat(operation.SafetyCopyPath); !os.IsNotExist(err) {
					t.Fatalf("partial safety artifact survived classification: %v", err)
				}
			}
		})
	}
}

func TestRestoreManifestAndRawImportValidationFaultMatrix(t *testing.T) {
	checksum := strings.Repeat("a", 64)
	validation := RestoreValidation{
		SizeBytes: 42, ChecksumSHA256: checksum, IntegrityResult: "ok", ForeignKeyErrors: 0, MigrationRows: 7,
		Migration: MigrationIdentity{FormatVersion: currentDatabaseFormatVersion, MigrationHead: expectedMigrationHead, LedgerSHA256: checksum, MinimumReader: "1"},
	}
	base := BackupManifest{
		FormatVersion: RestoreManifestFormatVersion, Release: "2026.08.05", DatabaseFormatVersion: currentDatabaseFormatVersion,
		MigrationHead: validation.Migration.MigrationHead, MigrationLedgerSHA256: checksum, MigrationLedgerRows: 7,
		MinimumReader: "1",
		ArtifactSet:   []BackupArtifact{{Name: "portico-import.db", Kind: "database", SizeBytes: 42, ChecksumSHA256: checksum}},
		CreatedAt:     "2026-08-05T01:02:03.123456789Z", IntegrityResult: "ok", ForeignKeyErrors: 0,
		BackupName: "portico-import.db", DatabaseBytes: 42, ChecksumSHA256: checksum,
	}
	for _, test := range []struct {
		name   string
		mutate func(*BackupManifest)
		code   string
	}{
		{name: "obsolete manifest", mutate: func(value *BackupManifest) { value.FormatVersion = 1 }, code: "restore_corrupt_database"},
		{name: "newer manifest", mutate: func(value *BackupManifest) { value.FormatVersion = RestoreManifestFormatVersion + 1 }, code: "restore_forward_incompatible_database"},
		{name: "mismatched backup name", mutate: func(value *BackupManifest) { value.BackupName = "portico-other.db" }, code: "restore_foreign_database"},
		{name: "mismatched artifact name", mutate: func(value *BackupManifest) { value.ArtifactSet[0].Name = "portico-other.db" }, code: "restore_corrupt_database"},
		{name: "bad size", mutate: func(value *BackupManifest) { value.DatabaseBytes++ }, code: "restore_corrupt_database"},
		{name: "bad reader", mutate: func(value *BackupManifest) { value.MinimumReader = "2" }, code: "restore_forward_incompatible_database"},
		{name: "insufficient provenance", mutate: func(value *BackupManifest) { value.Release = "" }, code: "restore_unidentified_database"},
	} {
		t.Run(test.name, func(t *testing.T) {
			manifest := base
			manifest.ArtifactSet = append([]BackupArtifact(nil), base.ArtifactSet...)
			test.mutate(&manifest)
			err := validateRestoreManifest(manifest, "/private/portico-import.db", validation, "")
			var validationErr *RestoreValidationError
			if !errors.As(err, &validationErr) || validationErr.Code != test.code {
				t.Fatalf("manifest error=%v, want code %s", err, test.code)
			}
		})
	}
	for _, body := range [][]byte{
		[]byte(`{"formatVersion":2,"unknown":true}`),
		[]byte(`{"formatVersion":2} trailing`),
		bytes.Repeat([]byte("x"), 128<<10+1),
	} {
		if _, err := ParseBackupManifestBytes(body); err == nil {
			t.Fatalf("malformed/oversized manifest was accepted: %q", body[:min(len(body), 32)])
		}
	}
}

type restoreFailingReader struct {
	read bool
}

func (reader *restoreFailingReader) Read(p []byte) (int, error) {
	if reader.read {
		return 0, errors.New("injected ENOSPC")
	}
	reader.read = true
	copy(p, []byte("partial restore bytes"))
	return len("partial restore bytes"), errors.New("injected ENOSPC")
}

func TestRestoreUploadCopyAndReadOnlyFaultsLeaveNoPartialArtifact(t *testing.T) {
	root := t.TempDir()
	appData := filepath.Join(root, "app-data")
	if err := os.MkdirAll(filepath.Join(appData, "restore"), 0o700); err != nil {
		t.Fatal(err)
	}
	streamTarget := filepath.Join(appData, "restore", "upload-fault.db")
	if _, err := WritePrivateStream(streamTarget, &restoreFailingReader{}, 1024); err == nil {
		t.Fatal("injected upload ENOSPC unexpectedly succeeded")
	}
	if _, err := os.Lstat(streamTarget); !os.IsNotExist(err) {
		t.Fatalf("failed upload left partial artifact: %v", err)
	}

	source := filepath.Join(appData, "source.db")
	if err := os.WriteFile(source, bytes.Repeat([]byte("x"), 32), 0o600); err != nil {
		t.Fatal(err)
	}
	copyTarget := filepath.Join(appData, "restore", "copy-fault.db")
	if err := CopyRestrictedFileSync(source, copyTarget, 8); err == nil {
		t.Fatal("oversized copy unexpectedly succeeded")
	}
	if _, err := os.Lstat(copyTarget); !os.IsNotExist(err) {
		t.Fatalf("failed bounded copy left partial artifact: %v", err)
	}

	if runtime.GOOS == "windows" {
		t.Skip("native Windows read-only ACL execution is a named CI gate")
	}
	if err := os.Chmod(filepath.Join(appData, "restore"), 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(appData, "restore"), 0o700)
	// Root and some containerized filesystems can still create files in a
	// mode-0500 directory. Do not turn that environment limitation into a
	// false EACCES pass/fail; the non-root POSIX gate supplies the real
	// permission evidence.
	probe, probeErr := os.OpenFile(filepath.Join(appData, "restore", ".permission-probe"), os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0o600)
	if probeErr == nil {
		_ = probe.Close()
		_ = os.Remove(filepath.Join(appData, "restore", ".permission-probe"))
		t.Skip("filesystem user can write mode-0500 directory; non-root POSIX permission gate required")
	}
	if _, err := WritePrivateStream(filepath.Join(appData, "restore", "readonly.db"), strings.NewReader("blocked"), 1024); err == nil {
		t.Fatal("read-only restore directory accepted an upload")
	}
}

func TestRestoreRenamePostRenameFaultMatrixIsRestartIdempotent(t *testing.T) {
	for _, stage := range []string{"sidecar-wal", "sidecar-shm", "sidecar-journal", "install-old-active", "install-new-active", "rollback-failed-active", "rollback-quarantine-install", "rollback-install-safety", "quarantine-interrupted"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			source := filepath.Join(root, "source.db")
			target := filepath.Join(root, "target.db")
			if err := os.WriteFile(source, []byte("durable artifact"), 0o600); err != nil {
				t.Fatal(err)
			}
			restoreHook := SetRestoreRenameForTest(func(observed string) error {
				if observed == stage {
					return errors.New("injected post-rename fault")
				}
				return nil
			})
			err := renameRestoreArtifact(stage, source, target)
			restoreHook()
			if err == nil {
				t.Fatal("post-rename fault did not interrupt rename")
			}
			if _, err := os.Stat(target); err != nil {
				t.Fatalf("durable rename target missing after post-rename fault: %v", err)
			}
			if _, err := os.Stat(source); !os.IsNotExist(err) {
				t.Fatalf("rename source survived post-rename fault: %v", err)
			}
			if err := optionalRenameWithStage(stage, source, target); err != nil {
				t.Fatalf("restart reconciliation did not converge: %v", err)
			}
		})
	}
}

// newRestoreStateMachineFixture creates a real, fully migrated Portico
// database, a logical rollback snapshot, and a distinct valid replacement.
// The fixture deliberately records the same durable evidence that the host
// coordinator records before it closes the old generation.
func newRestoreStateMachineFixture(t *testing.T) (config.Config, RestoreOperation) {
	t.Helper()
	t.Chdir("../..")
	root := t.TempDir()
	cfg := config.Config{AppDataDir: filepath.Join(root, "app-data"), DatabasePath: filepath.Join(root, "active", "portico.db")}
	if err := PreparePrivateDataPaths(cfg); err != nil {
		t.Fatalf("prepare restore fixture: %v", err)
	}
	active, err := Open(cfg)
	if err != nil {
		t.Fatalf("open restore fixture: %v", err)
	}
	if _, err := active.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES ('restore-state-machine-marker', '"original"', 'now') ON CONFLICT(key) DO UPDATE SET value_json=excluded.value_json`); err != nil {
		_ = active.Close()
		t.Fatalf("write original fixture marker: %v", err)
	}
	operationID := "restore-state-" + strings.TrimPrefix(restoreRandomID("fixture"), "fixture_")
	safetyPath := CanonicalRestoreSafetyCopyPath(cfg, operationID)
	safety, err := CreateVerifiedDatabaseSnapshot(context.Background(), active, safetyPath)
	if err != nil {
		_ = active.Close()
		t.Fatalf("create fixture safety snapshot: %v", err)
	}
	if err := active.Close(); err != nil {
		t.Fatalf("close original fixture: %v", err)
	}
	stagedPath := CanonicalRestoreStagedPath(cfg, operationID, false)
	if err := CopyRestrictedFileSync(cfg.DatabasePath, stagedPath, RestoreMaxDatabaseBytes); err != nil {
		t.Fatalf("copy fixture replacement: %v", err)
	}
	staged, err := sql.Open("sqlite", sqliteDSN(stagedPath))
	if err != nil {
		t.Fatalf("open staged fixture: %v", err)
	}
	if _, err := staged.Exec(`UPDATE settings SET value_json='"restored"' WHERE key='restore-state-machine-marker'`); err != nil {
		_ = staged.Close()
		t.Fatalf("write replacement fixture marker: %v", err)
	}
	if err := staged.Close(); err != nil {
		t.Fatalf("close staged fixture: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	operation := RestoreOperation{
		Version: RestoreOperationVersion, OperationID: operationID, BackupName: "fixture.db",
		StagedPath: stagedPath, ActivePath: cfg.DatabasePath, SafetyCopyPath: safetyPath,
		OldActivePath: CanonicalRestoreOldActivePath(cfg, operationID), InstallPath: CanonicalRestoreInstallPath(cfg, operationID),
		SafetyCopySizeBytes: safety.SizeBytes, SafetyCopyChecksumSHA256: safety.ChecksumSHA256, SafetyCopyIdentity: safety.Migration,
		RestoreMaxDatabaseBytes: RestoreMaxDatabaseBytes, Phase: RestorePhaseSafetyCopy, State: RestorePhaseSafetyCopy,
		Progress: 45, CreatedAt: now, UpdatedAt: now,
	}
	if err := WriteRestoreOperation(cfg.AppDataDir, operation); err != nil {
		t.Fatalf("write fixture restore marker: %v", err)
	}
	return cfg, operation
}

func restoreFixtureMarker(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open("sqlite", sqliteDSN(path))
	if err != nil {
		t.Fatalf("open fixture marker database: %v", err)
	}
	defer db.Close()
	var value string
	if err := db.QueryRow(`SELECT value_json FROM settings WHERE key='restore-state-machine-marker'`).Scan(&value); err != nil {
		t.Fatalf("read fixture marker: %v", err)
	}
	return value
}

func assertRestoreFixtureHealth(t *testing.T, cfg config.Config, wantMarker string) {
	t.Helper()
	// This is checked before opening the replacement. Opening a normal Portico
	// generation may legitimately create an empty WAL/SHM pair; the safety
	// property under test is that restart reconciliation left no stale active
	// sidecars at its install/rollback boundary.
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		if _, err := os.Lstat(cfg.DatabasePath + suffix); !os.IsNotExist(err) {
			t.Fatalf("active database sidecar %s survived convergence boundary: %v", suffix, err)
		}
	}
	db, err := sql.Open("sqlite", sqliteDSN(cfg.DatabasePath))
	if err != nil {
		t.Fatalf("open converged restore database: %v", err)
	}
	health, err := ValidateOpenDatabaseHealth(context.Background(), db)
	if err != nil {
		_ = db.Close()
		t.Fatalf("converged restore database health: %v", err)
	}
	if health.IntegrityResult != "ok" || health.ForeignKeyErrors != 0 || health.Migration.MigrationHead == "" || health.Migration.LedgerSHA256 == "" || health.MigrationRows <= 0 {
		_ = db.Close()
		t.Fatalf("converged restore health evidence=%#v, want integrity/FK/identity proof", health)
	}
	var marker string
	if err := db.QueryRow(`SELECT value_json FROM settings WHERE key='restore-state-machine-marker'`).Scan(&marker); err != nil {
		_ = db.Close()
		t.Fatalf("read converged active marker: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close converged restore database: %v", err)
	}
	if marker != wantMarker {
		t.Fatalf("converged active marker=%q, want %q", marker, wantMarker)
	}
	operation, err := ReadRestoreOperation(cfg.AppDataDir)
	if err != nil {
		t.Fatalf("read converged restore operation: %v", err)
	}
	if err := validateVerifiedSafetyCopy(context.Background(), &operation); err != nil {
		t.Fatalf("converged restore safety-copy evidence: %v", err)
	}
	if _, err := os.Lstat(operation.SafetyCopyPath); err != nil {
		t.Fatalf("verified safety-copy authority missing: %v", err)
	}
	if _, err := os.Lstat(operation.InstallPath); !os.IsNotExist(err) {
		t.Fatalf("transient install artifact survived convergence: %v", err)
	}
}

func addRestoreFixtureSidecars(t *testing.T, cfg config.Config, operationID, label string) {
	t.Helper()
	for _, suffix := range []string{"-wal", "-shm", "-journal"} {
		path := cfg.DatabasePath + suffix
		if err := os.WriteFile(path, []byte("fixture-"+label+"-"+suffix), 0o600); err != nil {
			t.Fatalf("write fixture %s sidecar: %v", suffix, err)
		}
	}
	_ = operationID
}

func TestRestoreInstallRollbackRestartStateMachineConvergesAtRealFaultPoints(t *testing.T) {
	installStages := []string{"sidecar-wal", "sidecar-shm", "sidecar-journal", "install-old-active", "install-new-active"}
	for _, stage := range installStages {
		t.Run("install-"+stage, func(t *testing.T) {
			cfg, operation := newRestoreStateMachineFixture(t)
			addRestoreFixtureSidecars(t, cfg, operation.OperationID, stage)
			restoreHook := SetRestoreRenameForTest(func(observed string) error {
				if observed == stage {
					return errors.New("injected post-rename fault after real install rename")
				}
				return nil
			})
			err := InstallRestoreOperationContext(context.Background(), cfg, &operation)
			restoreHook()
			if err == nil {
				t.Fatal("install post-rename fault did not interrupt the real state machine")
			}
			if err := RecoverInterruptedRestoreBeforeOpenContext(context.Background(), cfg); err != nil {
				t.Fatalf("restart reconciliation after %s: %v", stage, err)
			}
			latest, err := ReadRestoreOperation(cfg.AppDataDir)
			if err != nil {
				t.Fatal(err)
			}
			if latest.Phase != RestorePhaseHealthChecking || latest.State != RestorePhaseHealthChecking || !latest.ActiveMutationCompleted {
				t.Fatalf("install restart marker=%#v, want health-checking with completed mutation", latest)
			}
			assertRestoreFixtureHealth(t, cfg, `"restored"`)
			if _, err := os.Lstat(operation.InstallPath); !os.IsNotExist(err) {
				t.Fatalf("install temp survived convergence: %v", err)
			}
		})
	}

	rollbackStages := []string{"sidecar-wal", "sidecar-shm", "sidecar-journal", "rollback-failed-active", "rollback-quarantine-install", "rollback-install-safety"}
	for _, stage := range rollbackStages {
		t.Run("rollback-"+stage, func(t *testing.T) {
			cfg, operation := newRestoreStateMachineFixture(t)
			if err := InstallRestoreOperationContext(context.Background(), cfg, &operation); err != nil {
				t.Fatalf("prepare installed replacement: %v", err)
			}
			latest, err := ReadRestoreOperation(cfg.AppDataDir)
			if err != nil {
				t.Fatal(err)
			}
			latest.Phase, latest.State = RestorePhaseReopening, RestorePhaseReopening
			latest.ActiveMutationStarted, latest.ActiveMutationCompleted = true, true
			latest.RollbackPendingHealth = false
			if err := WriteRestoreOperation(cfg.AppDataDir, latest); err != nil {
				t.Fatal(err)
			}
			if stage == "rollback-quarantine-install" {
				if err := CopyRestrictedFileSync(latest.SafetyCopyPath, latest.InstallPath, RestoreMaxDatabaseBytes); err != nil {
					t.Fatalf("prepare rollback install collision: %v", err)
				}
			}
			if strings.HasPrefix(stage, "sidecar-") {
				addRestoreFixtureSidecars(t, cfg, latest.OperationID, stage)
			}
			restoreHook := SetRestoreRenameForTest(func(observed string) error {
				if observed == stage {
					return errors.New("injected post-rename fault after real rollback rename")
				}
				return nil
			})
			err = RollbackRestoreOperationContext(context.Background(), cfg, &latest, "restore_runtime_replacement_failed", errors.New("injected health failure"))
			restoreHook()
			if err == nil {
				t.Fatal("rollback post-rename fault did not interrupt the real state machine")
			}
			if err := RecoverInterruptedRestoreBeforeOpenContext(context.Background(), cfg); err != nil {
				t.Fatalf("restart rollback reconciliation after %s: %v", stage, err)
			}
			converged, err := ReadRestoreOperation(cfg.AppDataDir)
			if err != nil {
				t.Fatal(err)
			}
			if converged.Phase != RestorePhaseHealthChecking || converged.State != RestorePhaseHealthChecking || !converged.RollbackPendingHealth {
				t.Fatalf("rollback restart marker=%#v, want health-checking rollback pending", converged)
			}
			assertRestoreFixtureHealth(t, cfg, `"original"`)
		})
	}
}

func TestPreOpenReconciliationRollbackHonorsRestoreContextDeadline(t *testing.T) {
	cfg, operation := newRestoreStateMachineFixture(t)
	operation.Phase, operation.State = RestorePhaseHealthChecking, RestorePhaseHealthChecking
	if err := WriteRestoreOperation(cfg.AppDataDir, operation); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(cfg.DatabasePath); err != nil {
		t.Fatalf("remove active database for rollback branch: %v", err)
	}

	entered := make(chan struct{})
	restoreGate := SetRestoreRollbackBeforeValidationForTest(func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	})
	defer restoreGate()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		result <- RecoverInterruptedRestoreBeforeOpenContext(ctx, cfg)
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("pre-open recovery did not enter the missing-active rollback branch")
	}
	cancel()
	err := <-result
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled pre-open rollback error=%v, want context.Canceled", err)
	}
	if _, statErr := os.Lstat(cfg.DatabasePath); !os.IsNotExist(statErr) {
		t.Fatalf("canceled rollback mutated active authority: %v", statErr)
	}
	latest, err := ReadRestoreOperation(cfg.AppDataDir)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Phase != RestorePhaseRollingBack || latest.State != RestorePhaseRollingBack {
		t.Fatalf("canceled rollback did not retain in-flight marker: %#v", latest)
	}
}

const restoreSubprocessHelperEnv = "PORTICO_RESTORE_SUBPROCESS_HELPER"

// TestRestoreSubprocessHelper is run by the parent test binary as a separate
// process. It deliberately exits after a durable phase write or after the
// actual filesystem rename hook, so the parent exercises the same on-disk
// restart reconciler that a real Portico process restart uses. The helper is
// intentionally private to this test package and is inert during ordinary
// package runs.
func TestRestoreSubprocessHelper(t *testing.T) {
	if os.Getenv(restoreSubprocessHelperEnv) != "1" {
		return
	}
	cfg := config.Config{
		AppDataDir:   os.Getenv("PORTICO_RESTORE_SUBPROCESS_APP_DATA"),
		DatabasePath: os.Getenv("PORTICO_RESTORE_SUBPROCESS_DATABASE"),
	}
	if cfg.AppDataDir == "" || cfg.DatabasePath == "" {
		t.Fatal("subprocess restore helper requires durable config paths")
	}
	operation, err := ReadRestoreOperation(cfg.AppDataDir)
	if err != nil {
		t.Fatalf("helper read durable operation: %v", err)
	}
	mode := os.Getenv("PORTICO_RESTORE_SUBPROCESS_MODE")
	if mode == "phase" {
		phase := os.Getenv("PORTICO_RESTORE_SUBPROCESS_PHASE")
		if !restorePhaseAllowed(phase) {
			t.Fatalf("helper phase %q is not durable restore phase", phase)
		}
		operation.Phase, operation.State = phase, phase
		operation.ActiveMutationStarted = os.Getenv("PORTICO_RESTORE_SUBPROCESS_ACTIVE") == "1"
		if os.Getenv("PORTICO_RESTORE_SUBPROCESS_CLEAR_SAFETY") == "1" {
			operation.SafetyCopySizeBytes = 0
			operation.SafetyCopyChecksumSHA256 = ""
			operation.SafetyCopyIdentity = MigrationIdentity{}
			if err := cleanupRestoreArtifactPair(operation.SafetyCopyPath); err != nil {
				t.Fatalf("helper clear safety artifact: %v", err)
			}
		}
		if phase == RestorePhaseComplete {
			operation.CompletedAt = time.Now().UTC().Format(time.RFC3339Nano)
		}
		operation.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
		if err := WriteRestoreOperation(cfg.AppDataDir, operation); err != nil {
			t.Fatalf("helper write durable phase: %v", err)
		}
		os.Exit(97)
	}

	targetStage := os.Getenv("PORTICO_RESTORE_SUBPROCESS_KILL_STAGE")
	restoreHook := SetRestoreRenameForTest(func(stage string) error {
		if stage == targetStage {
			// This is an actual process death, not an error returned through the
			// caller. The parent must reconstruct state solely from disk.
			os.Exit(97)
		}
		return nil
	})
	defer restoreHook()
	switch mode {
	case "install":
		err = InstallRestoreOperationContext(context.Background(), cfg, &operation)
	case "rollback":
		err = RollbackRestoreOperationContext(context.Background(), cfg, &operation, "restore_runtime_replacement_failed", errors.New("subprocess helper rollback"))
	default:
		t.Fatalf("unknown restore subprocess helper mode %q", mode)
	}
	if err != nil {
		t.Fatalf("helper restore operation returned before kill: %v", err)
	}
	t.Fatalf("helper restore operation completed without kill stage %q", targetStage)
}

func runRestoreSubprocess(t *testing.T, cfg config.Config, mode, phase, stage string, active, clearSafety bool) {
	t.Helper()
	command := exec.Command(os.Args[0], "-test.run=^TestRestoreSubprocessHelper$", "-test.v")
	environment := append([]string{}, os.Environ()...)
	environment = append(environment,
		restoreSubprocessHelperEnv+"=1",
		"PORTICO_RESTORE_SUBPROCESS_APP_DATA="+cfg.AppDataDir,
		"PORTICO_RESTORE_SUBPROCESS_DATABASE="+cfg.DatabasePath,
		"PORTICO_RESTORE_SUBPROCESS_MODE="+mode,
		"PORTICO_RESTORE_SUBPROCESS_PHASE="+phase,
		"PORTICO_RESTORE_SUBPROCESS_KILL_STAGE="+stage,
		"PORTICO_RESTORE_SUBPROCESS_ACTIVE="+map[bool]string{true: "1", false: "0"}[active],
		"PORTICO_RESTORE_SUBPROCESS_CLEAR_SAFETY="+map[bool]string{true: "1", false: "0"}[clearSafety],
	)
	command.Env = environment
	output, err := command.CombinedOutput()
	if err == nil {
		t.Fatalf("restore subprocess unexpectedly survived helper kill: output=%s", output)
	}
	var exitError *exec.ExitError
	if !errors.As(err, &exitError) {
		t.Fatalf("restore subprocess was not an exit-status failure: %v output=%s", err, output)
	}
	if exitError.ExitCode() != 97 {
		t.Fatalf("restore subprocess exit=%d, want 97; error=%v output=%s", exitError.ExitCode(), err, output)
	}
}

func TestRestoreSubprocessKillConvergesThroughDurablePhases(t *testing.T) {
	phaseCases := []struct {
		name        string
		phase       string
		active      bool
		clearSafety bool
		prepare     func(*testing.T, config.Config, *RestoreOperation)
		wantState   string
		wantMarker  string
		wantHealth  bool
	}{
		{name: "validating-boundary", phase: RestorePhaseValidating, clearSafety: true, wantState: RestorePhaseValidating, wantMarker: "original"},
		{name: "staged-boundary", phase: RestorePhaseStaged, clearSafety: true, wantState: RestorePhaseStaged, wantMarker: "original"},
		{name: "quiescing-before-safety-evidence", phase: RestorePhaseQuiescing, clearSafety: true, wantState: RestorePhaseFailed, wantMarker: "original"},
		{name: "safety-copy-evidence-boundary", phase: RestorePhaseSafetyCopy, wantState: RestorePhaseHealthChecking, wantMarker: "restored", wantHealth: true},
		{name: "installing-boundary", phase: RestorePhaseInstalling, wantState: RestorePhaseHealthChecking, wantMarker: "restored", wantHealth: true},
		{name: "reopening-boundary", phase: RestorePhaseReopening, prepare: installRestoreFixtureForSubprocess, wantState: RestorePhaseHealthChecking, wantMarker: "restored", wantHealth: true},
		{name: "health-checking-boundary", phase: RestorePhaseHealthChecking, prepare: installRestoreFixtureForSubprocess, wantState: RestorePhaseHealthChecking, wantMarker: "restored", wantHealth: true},
		{name: "rolling-back-boundary", phase: RestorePhaseRollingBack, active: true, prepare: installRestoreFixtureForSubprocess, wantState: RestorePhaseHealthChecking, wantMarker: "original", wantHealth: true},
		{name: "complete-boundary", phase: RestorePhaseComplete, prepare: installRestoreFixtureForSubprocess, wantState: RestorePhaseComplete, wantMarker: "restored", wantHealth: true},
		{name: "failed-boundary", phase: RestorePhaseFailed, wantState: RestorePhaseFailed, wantMarker: "original"},
	}
	for _, test := range phaseCases {
		t.Run(test.name, func(t *testing.T) {
			cfg, operation := newRestoreStateMachineFixture(t)
			if test.prepare != nil {
				test.prepare(t, cfg, &operation)
			}
			runRestoreSubprocess(t, cfg, "phase", test.phase, "", test.active, test.clearSafety)
			if err := RecoverInterruptedRestoreBeforeOpenContext(context.Background(), cfg); err != nil {
				t.Fatalf("restart reconciliation after %s process kill: %v", test.phase, err)
			}
			latest, err := ReadRestoreOperation(cfg.AppDataDir)
			if err != nil {
				t.Fatal(err)
			}
			if latest.State != test.wantState {
				t.Fatalf("phase %s restart state=%q, want %q; marker=%#v", test.phase, latest.State, test.wantState, latest)
			}
			if !test.wantHealth {
				if got := restoreFixtureMarker(t, cfg.DatabasePath); got != `"original"` {
					t.Fatalf("phase %s changed untouched active database to %q", test.phase, got)
				}
			} else {
				assertRestoreFixtureHealth(t, cfg, fmt.Sprintf("%q", test.wantMarker))
			}
		})
	}
}

func installRestoreFixtureForSubprocess(t *testing.T, cfg config.Config, operation *RestoreOperation) {
	t.Helper()
	if err := InstallRestoreOperationContext(context.Background(), cfg, operation); err != nil {
		t.Fatalf("prepare installed subprocess fixture: %v", err)
	}
}

func TestRestoreSubprocessKillConvergesAtActualInstallAndRollbackRenames(t *testing.T) {
	for _, stage := range []string{"sidecar-wal", "sidecar-shm", "sidecar-journal", "install-old-active", "install-new-active"} {
		t.Run("install-"+stage, func(t *testing.T) {
			cfg, operation := newRestoreStateMachineFixture(t)
			addRestoreFixtureSidecars(t, cfg, operation.OperationID, stage)
			runRestoreSubprocess(t, cfg, "install", "", stage, false, false)
			if err := RecoverInterruptedRestoreBeforeOpenContext(context.Background(), cfg); err != nil {
				t.Fatalf("restart install reconciliation after %s process kill: %v", stage, err)
			}
			latest, err := ReadRestoreOperation(cfg.AppDataDir)
			if err != nil {
				t.Fatal(err)
			}
			if latest.Phase != RestorePhaseHealthChecking || !latest.ActiveMutationCompleted {
				t.Fatalf("install %s restart marker=%#v, want health-checking completed mutation", stage, latest)
			}
			assertRestoreFixtureHealth(t, cfg, `"restored"`)
		})
	}

	for _, stage := range []string{"sidecar-wal", "sidecar-shm", "sidecar-journal", "rollback-failed-active", "rollback-quarantine-install", "rollback-install-safety"} {
		t.Run("rollback-"+stage, func(t *testing.T) {
			cfg, operation := newRestoreStateMachineFixture(t)
			if err := InstallRestoreOperationContext(context.Background(), cfg, &operation); err != nil {
				t.Fatalf("prepare installed replacement: %v", err)
			}
			latest, err := ReadRestoreOperation(cfg.AppDataDir)
			if err != nil {
				t.Fatal(err)
			}
			latest.Phase, latest.State = RestorePhaseReopening, RestorePhaseReopening
			latest.ActiveMutationStarted, latest.ActiveMutationCompleted = true, true
			latest.RollbackPendingHealth = false
			if stage == "rollback-quarantine-install" {
				if err := CopyRestrictedFileSync(latest.SafetyCopyPath, latest.InstallPath, RestoreMaxDatabaseBytes); err != nil {
					t.Fatalf("prepare rollback install collision: %v", err)
				}
			}
			if strings.HasPrefix(stage, "sidecar-") {
				addRestoreFixtureSidecars(t, cfg, latest.OperationID, stage)
			}
			if err := WriteRestoreOperation(cfg.AppDataDir, latest); err != nil {
				t.Fatal(err)
			}
			runRestoreSubprocess(t, cfg, "rollback", "", stage, true, false)
			if err := RecoverInterruptedRestoreBeforeOpenContext(context.Background(), cfg); err != nil {
				t.Fatalf("restart rollback reconciliation after %s process kill: %v", stage, err)
			}
			converged, err := ReadRestoreOperation(cfg.AppDataDir)
			if err != nil {
				t.Fatal(err)
			}
			if converged.Phase != RestorePhaseHealthChecking || !converged.RollbackPendingHealth {
				t.Fatalf("rollback %s restart marker=%#v, want rollback health-checking", stage, converged)
			}
			assertRestoreFixtureHealth(t, cfg, `"original"`)
		})
	}
}

func TestPreOpenReconciliationThenRestoredOpenFailureRollsBackVerifiedSafetyCopy(t *testing.T) {
	cfg, operation := newRestoreStateMachineFixture(t)
	if err := InstallRestoreOperationContext(context.Background(), cfg, &operation); err != nil {
		t.Fatalf("install restored pre-open fixture: %v", err)
	}
	operation, err := ReadRestoreOperation(cfg.AppDataDir)
	if err != nil {
		t.Fatal(err)
	}
	operation.Phase, operation.State = RestorePhaseHealthChecking, RestorePhaseHealthChecking
	operation.RollbackPendingHealth = false
	if err := WriteRestoreOperation(cfg.AppDataDir, operation); err != nil {
		t.Fatal(err)
	}

	// This models a restored database that is present at startup but fails the
	// host's first OpenWithReporter call after filesystem reconciliation. The
	// verified logical safety copy remains the only rollback authority.
	if err := os.WriteFile(cfg.DatabasePath, []byte("not a sqlite database"), 0o600); err != nil {
		t.Fatalf("corrupt restored active database: %v", err)
	}
	if err := RecoverInterruptedRestoreBeforeOpenContext(context.Background(), cfg); err != nil {
		t.Fatalf("pre-open reconciliation: %v", err)
	}
	preOpen, err := ReadRestoreOperation(cfg.AppDataDir)
	if err != nil {
		t.Fatal(err)
	}
	if preOpen.Phase != RestorePhaseHealthChecking || preOpen.State != RestorePhaseHealthChecking {
		t.Fatalf("pre-open marker=%#v, want health-checking handoff", preOpen)
	}
	broken, openErr := OpenWithReporterContext(context.Background(), cfg, nil)
	if openErr == nil {
		if broken != nil {
			_ = broken.Close()
		}
		t.Fatal("corrupt restored database unexpectedly opened")
	}
	if broken != nil {
		_ = broken.Close()
		t.Fatal("open failure returned a live database handle")
	}

	if err := RollbackRestoreOperationContext(context.Background(), cfg, &preOpen, "restore_open_failed", openErr); err != nil {
		t.Fatalf("rollback after restored open failure: %v", err)
	}
	rolledBack, err := ReadRestoreOperation(cfg.AppDataDir)
	if err != nil {
		t.Fatal(err)
	}
	if rolledBack.Phase != RestorePhaseHealthChecking || !rolledBack.RollbackPendingHealth {
		t.Fatalf("open-failure rollback marker=%#v, want health-checking rollback pending", rolledBack)
	}
	reopened, err := OpenWithReporterContext(context.Background(), cfg, nil)
	if err != nil {
		t.Fatalf("reopen verified safety database: %v", err)
	}
	health, err := ValidateOpenDatabaseHealth(context.Background(), reopened)
	if err != nil {
		_ = reopened.Close()
		t.Fatalf("reopened safety database health: %v", err)
	}
	if health.IntegrityResult != "ok" || health.ForeignKeyErrors != 0 || health.Migration.MigrationHead == "" {
		_ = reopened.Close()
		t.Fatalf("reopened safety health evidence=%#v", health)
	}
	var marker string
	if err := reopened.QueryRow(`SELECT value_json FROM settings WHERE key='restore-state-machine-marker'`).Scan(&marker); err != nil {
		_ = reopened.Close()
		t.Fatalf("read rollback marker: %v", err)
	}
	if err := reopened.Close(); err != nil {
		t.Fatalf("close reopened safety database: %v", err)
	}
	if marker != `"original"` {
		t.Fatalf("rollback reopened active marker=%q, want original", marker)
	}
	if err := CompleteRestoreRollbackOperation(cfg, &rolledBack); err != nil {
		t.Fatalf("complete rollback after health proof: %v", err)
	}
	terminal, err := ReadRestoreOperation(cfg.AppDataDir)
	if err != nil {
		t.Fatal(err)
	}
	if terminal.State != RestorePhaseFailed || terminal.Phase != RestorePhaseFailed {
		t.Fatalf("rollback terminal marker=%#v, want truthful failed/rolled-back state", terminal)
	}
}

func TestRepeatedRestoreCyclesArchiveTerminalSlotsAndRetainVerifiedHistory(t *testing.T) {
	cfg, first := newRestoreStateMachineFixture(t)
	if err := InstallRestoreOperationContext(context.Background(), cfg, &first); err != nil {
		t.Fatalf("install first restore: %v", err)
	}
	assertRestoreFixtureHealth(t, cfg, `"restored"`)
	first, err := ReadRestoreOperation(cfg.AppDataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := CompleteRestoreOperation(cfg, &first); err != nil {
		t.Fatalf("complete first restore: %v", err)
	}
	first, err = ReadRestoreOperation(cfg.AppDataDir)
	if err != nil {
		t.Fatal(err)
	}
	if first.Phase != RestorePhaseComplete || first.CleanupPending {
		t.Fatalf("first terminal marker=%#v, want complete and cleaned", first)
	}
	if err := ArchiveRestoreOperation(cfg, first); err != nil {
		t.Fatalf("archive first terminal restore: %v", err)
	}
	if _, err := os.Stat(first.SafetyCopyPath); err != nil {
		t.Fatalf("first verified recovery point was not retained: %v", err)
	}

	secondID := "restore-state-second"
	active, err := Open(cfg)
	if err != nil {
		t.Fatalf("open active database for second restore: %v", err)
	}
	secondSafetyPath := CanonicalRestoreSafetyCopyPath(cfg, secondID)
	secondSafety, err := CreateVerifiedDatabaseSnapshot(context.Background(), active, secondSafetyPath)
	if err != nil {
		_ = active.Close()
		t.Fatalf("create second safety snapshot: %v", err)
	}
	if err := active.Close(); err != nil {
		t.Fatalf("close second safety source: %v", err)
	}
	secondStagedPath := CanonicalRestoreStagedPath(cfg, secondID, false)
	if err := CopyRestrictedFileSync(cfg.DatabasePath, secondStagedPath, RestoreMaxDatabaseBytes); err != nil {
		t.Fatalf("stage second restore: %v", err)
	}
	secondStaged, err := sql.Open("sqlite", sqliteDSN(secondStagedPath))
	if err != nil {
		t.Fatalf("open second staged restore: %v", err)
	}
	if _, err := secondStaged.Exec(`UPDATE settings SET value_json='"restored-second"' WHERE key='restore-state-machine-marker'`); err != nil {
		_ = secondStaged.Close()
		t.Fatalf("write second restore marker: %v", err)
	}
	if err := secondStaged.Close(); err != nil {
		t.Fatalf("close second staged restore: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	second := RestoreOperation{
		Version: RestoreOperationVersion, OperationID: secondID, BackupName: "second.db",
		StagedPath: secondStagedPath, ActivePath: cfg.DatabasePath, SafetyCopyPath: secondSafetyPath,
		OldActivePath: CanonicalRestoreOldActivePath(cfg, secondID), InstallPath: CanonicalRestoreInstallPath(cfg, secondID),
		SafetyCopySizeBytes: secondSafety.SizeBytes, SafetyCopyChecksumSHA256: secondSafety.ChecksumSHA256, SafetyCopyIdentity: secondSafety.Migration,
		RestoreMaxDatabaseBytes: RestoreMaxDatabaseBytes, Phase: RestorePhaseSafetyCopy, State: RestorePhaseSafetyCopy,
		Progress: 45, CreatedAt: now, UpdatedAt: now,
	}
	if err := WriteRestoreOperation(cfg.AppDataDir, second); err != nil {
		t.Fatalf("publish second restore marker: %v", err)
	}
	if err := InstallRestoreOperationContext(context.Background(), cfg, &second); err != nil {
		t.Fatalf("install second restore: %v", err)
	}
	assertRestoreFixtureHealth(t, cfg, `"restored-second"`)
	second, err = ReadRestoreOperation(cfg.AppDataDir)
	if err != nil {
		t.Fatal(err)
	}
	if err := CompleteRestoreOperation(cfg, &second); err != nil {
		t.Fatalf("complete second restore: %v", err)
	}
	second, err = ReadRestoreOperation(cfg.AppDataDir)
	if err != nil {
		t.Fatal(err)
	}
	if second.Phase != RestorePhaseComplete || second.CleanupPending {
		t.Fatalf("second terminal marker=%#v, want complete and cleaned", second)
	}
	if err := ArchiveRestoreOperation(cfg, second); err != nil {
		t.Fatalf("archive second terminal restore: %v", err)
	}
	if _, err := PruneRestoreHistory(cfg, 2, 90*24*time.Hour); err != nil {
		t.Fatalf("prune bounded restore history: %v", err)
	}
	assertRestoreFixtureHealth(t, cfg, `"restored-second"`)

	historyEntries, err := os.ReadDir(filepath.Join(cfg.AppDataDir, "restore", "history"))
	if err != nil {
		t.Fatal(err)
	}
	historyCount := 0
	for _, entry := range historyEntries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			historyCount++
		}
	}
	if historyCount < 2 || historyCount > 2 {
		t.Fatalf("repeated restore history count=%d, want bounded two-cycle evidence", historyCount)
	}
	for _, path := range []string{first.StagedPath, first.OldActivePath, first.InstallPath, second.StagedPath, second.OldActivePath, second.InstallPath} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("repeated restore left transient artifact %s: %v", path, err)
		}
	}
}

func TestRestorePreMutationDurablePhaseRestartTruthTable(t *testing.T) {
	for _, phase := range []string{RestorePhaseValidating, RestorePhaseStaged} {
		t.Run("no-mutation-"+phase, func(t *testing.T) {
			cfg, operation := newRestoreStateMachineFixture(t)
			operation.Phase, operation.State = phase, phase
			operation.SafetyCopySizeBytes = 0
			operation.SafetyCopyChecksumSHA256 = ""
			operation.SafetyCopyIdentity = MigrationIdentity{}
			if err := WriteRestoreOperation(cfg.AppDataDir, operation); err != nil {
				t.Fatal(err)
			}
			if err := RecoverInterruptedRestoreBeforeOpenContext(context.Background(), cfg); err != nil {
				t.Fatalf("recover untouched %s marker: %v", phase, err)
			}
			latest, err := ReadRestoreOperation(cfg.AppDataDir)
			if err != nil {
				t.Fatal(err)
			}
			if latest.Phase != phase || latest.State != phase || latest.ActiveMutationStarted {
				t.Fatalf("pre-mutation %s marker changed unexpectedly: %#v", phase, latest)
			}
			if got := restoreFixtureMarker(t, cfg.DatabasePath); got != `"original"` {
				t.Fatalf("pre-mutation %s changed active database to %q", phase, got)
			}
		})
	}

	for _, test := range []struct {
		name                  string
		activeMutationStarted bool
		wantState             string
		wantPhase             string
	}{
		{name: "partial-safety-before-mutation", wantState: RestorePhaseFailed, wantPhase: RestorePhaseFailed},
		{name: "install-marker-without-safety-evidence", activeMutationStarted: true, wantState: RestoreStateRecoveryNeeded, wantPhase: RestorePhaseRollingBack},
	} {
		t.Run(test.name, func(t *testing.T) {
			cfg, operation := newRestoreStateMachineFixture(t)
			operation.Phase, operation.State = RestorePhaseSafetyCopy, RestorePhaseSafetyCopy
			operation.ActiveMutationStarted = test.activeMutationStarted
			operation.SafetyCopySizeBytes = 0
			operation.SafetyCopyChecksumSHA256 = ""
			operation.SafetyCopyIdentity = MigrationIdentity{}
			if err := os.Remove(operation.SafetyCopyPath); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(operation.SafetyCopyPath, []byte("partial safety snapshot"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := WriteRestoreOperation(cfg.AppDataDir, operation); err != nil {
				t.Fatal(err)
			}
			if err := RecoverInterruptedRestoreBeforeOpenContext(context.Background(), cfg); err != nil {
				t.Fatalf("recover %s: %v", test.name, err)
			}
			latest, err := ReadRestoreOperation(cfg.AppDataDir)
			if err != nil {
				t.Fatal(err)
			}
			if latest.State != test.wantState || latest.Phase != test.wantPhase {
				t.Fatalf("%s marker=%#v, want state=%s phase=%s", test.name, latest, test.wantState, test.wantPhase)
			}
			if got := restoreFixtureMarker(t, cfg.DatabasePath); got != `"original"` {
				t.Fatalf("%s changed untouched active database to %q", test.name, got)
			}
		})
	}
}

func TestCompleteRestoreCommitsBeforeCleanupAndRetriesWarning(t *testing.T) {
	cfg, operation := newRestoreStateMachineFixture(t)
	if err := os.Remove(operation.StagedPath); err != nil {
		t.Fatal(err)
	}
	// A directory at the canonical staged path is an invalid transient artifact
	// but cannot be removed by the pair cleanup. This injects a post-commit
	// cleanup failure without changing the active database authority.
	if err := os.Mkdir(operation.StagedPath, 0o700); err != nil {
		t.Fatal(err)
	}
	err := CompleteRestoreOperation(cfg, &operation)
	var postCommit *RestorePostCommitError
	if !errors.As(err, &postCommit) {
		t.Fatalf("complete cleanup error=%v, want post-commit warning", err)
	}
	latest, err := ReadRestoreOperation(cfg.AppDataDir)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Phase != RestorePhaseComplete || latest.State != RestorePhaseComplete || !latest.CleanupPending || latest.WarningCode != "restore_cleanup_pending" {
		t.Fatalf("post-commit marker=%#v, want complete cleanup-pending", latest)
	}
	if got := restoreFixtureMarker(t, cfg.DatabasePath); got != `"original"` {
		t.Fatalf("post-commit cleanup failure changed active database to %q", got)
	}
	if err := os.Remove(operation.StagedPath); err != nil {
		t.Fatal(err)
	}
	if err := RetryCompletedRestoreCleanup(cfg); err != nil {
		t.Fatalf("retry completed restore cleanup: %v", err)
	}
	latest, err = ReadRestoreOperation(cfg.AppDataDir)
	if err != nil {
		t.Fatal(err)
	}
	if latest.Phase != RestorePhaseComplete || latest.CleanupPending || latest.WarningCode != "" {
		t.Fatalf("cleanup retry marker=%#v, want clean complete", latest)
	}
}

func TestRestoreRealDatabaseClassificationMatrixAndRawImportAuthority(t *testing.T) {
	t.Chdir("../..")
	root := t.TempDir()
	cfg := config.Config{AppDataDir: filepath.Join(root, "app-data"), DatabasePath: filepath.Join(root, "active", "portico.db")}
	if err := PreparePrivateDataPaths(cfg); err != nil {
		t.Fatal(err)
	}
	db, err := Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO settings (key, value_json, updated_at) VALUES ('real-classification', '"valid"', 'now')`); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	valid := filepath.Join(root, "valid.db")
	if err := CopyRestrictedFileSync(cfg.DatabasePath, valid, RestoreMaxDatabaseBytes); err != nil {
		t.Fatal(err)
	}
	validValidation, err := InspectRestoreDatabase(context.Background(), valid)
	if err != nil {
		t.Fatalf("inspect real valid database: %v", err)
	}
	if _, err := ValidateRestoreCandidate(context.Background(), valid, nil); err == nil {
		t.Fatal("manifestless real database was accepted by catalog restore authority")
	} else {
		var typed *RestoreValidationError
		if !errors.As(err, &typed) || typed.Code != "restore_unidentified_database" {
			t.Fatalf("manifestless real database error=%v, want restore_unidentified_database", err)
		}
	}
	if !validValidation.Manifestless || validValidation.Migration.MigrationHead == "" {
		t.Fatalf("raw-import inspection lost exact identity: %#v", validValidation)
	}
	cases := []struct {
		name   string
		mutate func(*sql.DB) error
		want   string
	}{
		{name: "corrupt bytes", mutate: nil, want: "restore_corrupt_database"},
		{name: "foreign sqlite", mutate: func(candidate *sql.DB) error {
			_, err := candidate.Exec(`CREATE TABLE foreign_only (id TEXT)`)
			return err
		}, want: "restore_unidentified_database"},
		{name: "noncanonical database format", mutate: func(candidate *sql.DB) error {
			_, err := candidate.Exec(`UPDATE portico_database_identity SET format_version = 3 WHERE id = 1`)
			return err
		}, want: "restore_foreign_database"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			candidate := filepath.Join(root, "candidate-"+strings.ReplaceAll(test.name, " ", "-")+".db")
			if test.name == "corrupt bytes" {
				if err := os.WriteFile(candidate, []byte("SQLite format 3\\x00not-a-database"), 0o600); err != nil {
					t.Fatal(err)
				}
			} else if test.name == "foreign sqlite" {
				foreign, err := sql.Open("sqlite", sqliteDSN(candidate))
				if err != nil {
					t.Fatal(err)
				}
				if _, err := foreign.Exec(`CREATE TABLE foreign_only (id TEXT)`); err != nil {
					_ = foreign.Close()
					t.Fatal(err)
				}
				if err := foreign.Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				if err := CopyRestrictedFileSync(valid, candidate, RestoreMaxDatabaseBytes); err != nil {
					t.Fatal(err)
				}
				mutated, err := sql.Open("sqlite", sqliteDSN(candidate))
				if err != nil {
					t.Fatal(err)
				}
				if err := test.mutate(mutated); err != nil {
					_ = mutated.Close()
					t.Fatal(err)
				}
				if err := mutated.Close(); err != nil {
					t.Fatal(err)
				}
			}
			_, err := InspectRestoreDatabase(context.Background(), candidate)
			if test.name == "corrupt bytes" {
				if err == nil {
					t.Fatal("corrupt real candidate was accepted")
				}
				var typed *RestoreValidationError
				if !errors.As(err, &typed) || typed.Code != test.want {
					t.Fatalf("corrupt candidate error=%v, want %s", err, test.want)
				}
				return
			}
			if err == nil {
				t.Fatalf("%s candidate was accepted", test.name)
			}
			var typed *RestoreValidationError
			if !errors.As(err, &typed) || typed.Code != test.want {
				t.Fatalf("%s candidate error=%v, want %s", test.name, err, test.want)
			}
		})
	}
}

func sqlOpenReadOnlyForRestore(path string) (*sql.DB, error) {
	return sql.Open("sqlite", readOnlySQLiteDSN(path))
}
