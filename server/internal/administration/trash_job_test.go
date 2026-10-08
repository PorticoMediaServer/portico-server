package administration

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
)

func TestTrashJobSingleStepAndAtomicRollback(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "held-zero-retention", true: "rollback"}[fail], func(t *testing.T) {
			s, db, _, item, files := deletionFixture(t, 0)
			moves := 0
			ctx := context.WithValue(context.Background(), fileIOObserverKey{}, func() {
				moves++
				if dbwork.WriteGate().Stats().Active {
					t.Fatal("filesystem work holds write gate")
				}
			})
			var before int
			db.QueryRow(`SELECT count(*) FROM admin_receipts`).Scan(&before)
			injected := errors.New("job cursor failed")
			e := s.TrashJobItem(ctx, allow, "owner", "single-step", item, func(tx *sql.Tx) error {
				if fail {
					return injected
				}
				return nil
			})
			if fail && !errors.Is(e, injected) || !fail && e != nil {
				t.Fatal(e)
			}
			if moves != 2 {
				t.Fatal("did not exercise both file moves", moves)
			}
			var held, receipts int
			db.QueryRow(`SELECT count(*) FROM admin_trash WHERE state='held'`).Scan(&held)
			db.QueryRow(`SELECT count(*) FROM admin_receipts`).Scan(&receipts)
			if receipts != before {
				t.Fatal("per-item receipt written")
			}
			if fail {
				if held != 0 {
					t.Fatal("failed effect committed")
				}
				for _, path := range files {
					raw, e := os.ReadFile(path)
					if e != nil || len(raw) != 11 {
						t.Fatal(path, e)
					}
				}
			} else {
				if held != 1 {
					t.Fatal(held)
				}
				var entry string
				var expires int64
				db.QueryRow(`SELECT t.id,t.expires_ms FROM admin_trash t JOIN catalog_entities e ON e.id=t.item_id WHERE e.public_id=pid_blob(?)`, item).Scan(&entry, &expires)
				if expires != 0 {
					t.Fatal("zero retention must remain recoverable", expires)
				}
				for _, path := range files {
					if _, e := os.Stat(path); !errors.Is(e, os.ErrNotExist) {
						t.Fatal(path, e)
					}
				}
				restored, e := s.RestoreFromTrash(context.Background(), allow, entry, "restore-job")
				if e != nil || restored.Restored != 2 || restored.Reindex {
					t.Fatal(restored, e)
				}
			}
		})
	}
}
func TestTrashJobJournalRecoveryAcrossDatabaseRestart(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "uncommitted", true: "committed"}[committed], func(t *testing.T) {
			s, db, root, item, files := deletionFixture(t, 7)
			journal := s.fileJournal("job-trash", "interrupted-job", "digest")
			journal.TrashEntryID = "interrupted-entry"
			target := filepath.Join(root, "trash", journal.TrashEntryID, "file.mkv")
			if e := journal.move(context.Background(), files[0], target); e != nil {
				t.Fatal(e)
			}
			if committed {
				if _, e := db.Exec(`INSERT INTO admin_trash VALUES(?,'lib-1',(SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)),'Movie','movie','[]',1,11,1,0,'held','owner',?)`, journal.TrashEntryID, item, journal.OperationID); e != nil {
					t.Fatal(e)
				}
			}
			db.Close()
			reopened, e := persistence.Open(filepath.Join(root, "server.sqlite"))
			if e != nil {
				t.Fatal(e)
			}
			defer reopened.Close()
			recovered := NewAt(reopened, root)
			ctx := context.WithValue(context.Background(), fileIOObserverKey{}, func() {
				if dbwork.WriteGate().Stats().Active {
					t.Fatal("recovery holds write gate")
				}
			})
			if e = recovered.RecoverFileOperations(ctx); e != nil {
				t.Fatal(e)
			}
			_, original := os.Stat(files[0])
			_, destination := os.Stat(target)
			if committed {
				if !errors.Is(original, os.ErrNotExist) || destination != nil {
					t.Fatal(original, destination)
				}
			} else {
				if original != nil || !errors.Is(destination, os.ErrNotExist) {
					t.Fatal(original, destination)
				}
			}
			if e = recovered.RecoverFileOperations(ctx); e != nil {
				t.Fatal("recovery replay", e)
			}
		})
	}
}

func TestTrashJobRevokedBetweenMoveAndCommitRestoresFiles(t *testing.T) {
	s, db, _, item, files := deletionFixture(t, 0)
	calls := 0
	e := s.TrashJobItem(context.Background(), func(context.Context, *sql.Tx) error {
		calls++
		if calls == 2 {
			return ErrDenied
		}
		return nil
	}, "owner", "revoked-job", item, func(*sql.Tx) error { t.Fatal("committed revoked action"); return nil })
	if !errors.Is(e, ErrDenied) || calls != 2 {
		t.Fatal(e, calls)
	}
	var held int
	if e = db.QueryRow(`SELECT count(*) FROM admin_trash WHERE state='held'`).Scan(&held); e != nil || held != 0 {
		t.Fatal(e, held)
	}
	for _, path := range files {
		if raw, e := os.ReadFile(path); e != nil || len(raw) != 11 {
			t.Fatal(path, e)
		}
	}
}
