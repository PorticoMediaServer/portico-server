package backup

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func TestRestoreProgressSurvivesRequestCancellation(t *testing.T) {
	svc, _, _ := testService(t)
	id, err := svc.Create(context.Background(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	job, err := svc.StartRestore(ctx, Source{BackupID: id}, "restore-operation")
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		progress := svc.RestoreProgress()
		if progress.ErrorCode != "" {
			t.Fatal(progress)
		}
		if progress.Phase == "staged" {
			if progress.JobID != job || progress.Result == nil || !progress.Result.Staged {
				t.Fatal(progress)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("restore did not finish", progress)
		}
		time.Sleep(10 * time.Millisecond)
	}
	replay, err := svc.StartRestore(context.Background(), Source{BackupID: id}, "restore-operation")
	if err != nil || replay != job {
		t.Fatal(replay, err)
	}
}

func TestSeparateBackupDestinationAndRestoreRefusalWithoutLock(t *testing.T) {
	svc, state, _ := testService(t)
	directory := filepath.Join(t.TempDir(), "copies")
	if err := svc.SetDirectory(directory); err != nil {
		t.Fatal(err)
	}
	id, err := svc.Create(context.Background(), KindManual)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := svc.List()
	if err != nil || len(listed) != 1 || listed[0].ID != id || svc.backupRoot() == filepath.Join(state, BackupsDir) {
		t.Fatal(listed, err)
	}
	svc.SetRestoreAvailable(false)
	if _, err = svc.StartRestore(context.Background(), Source{BackupID: id}, "restore-refused"); err != ErrRestorePending {
		t.Fatal(err)
	}
}

func TestCompletedRestoreReceiptReplaysAfterRestart(t *testing.T) {
	service, state, db := testService(t)
	source := Source{BackupID: "2026-10-13T010000Z"}
	marker := restoreMarker{JobID: "completed-job", OperationID: "completed-operation", Source: source, SchemaVersion: service.schemaVersion}
	if err := writeResult(state, "restored", "", marker); err != nil {
		t.Fatal(err)
	}
	restarted := New(state, db, "server", "version", service.schemaVersion)
	id, err := restarted.StartRestore(context.Background(), source, "completed-operation")
	if err != nil || id != "completed-job" || restarted.RestoreProgress().Result == nil {
		t.Fatal(id, err, restarted.RestoreProgress())
	}
	if _, err = restarted.StartRestore(context.Background(), Source{Path: "/different"}, "completed-operation"); err != ErrInvalid {
		t.Fatal("operation accepted changed source", err)
	}
}
