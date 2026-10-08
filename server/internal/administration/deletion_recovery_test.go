package administration

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
)

func deletionFixture(t *testing.T, days int) (*Service, *sql.DB, string, string, []string) {
	t.Helper()
	s, db, root := newService(t)
	library, item, files := seedLibrary(t, db, root)
	doc, err := s.LibrarySettingsFor(context.Background(), allow, library)
	if err != nil {
		t.Fatal(err)
	}
	doc.Settings.AllowMediaDeletion = true
	doc.Settings.TrashRetentionDays = days
	if _, err = s.SaveLibrarySettings(context.Background(), allow, library, Change[LibrarySettings]{ExpectedRevision: doc.Revision, Settings: doc.Settings, OperationID: "test-policy-key-1"}); err != nil {
		t.Fatal(err)
	}
	return s, db, root, item, files
}
func deleteItem(t *testing.T, s *Service, item, operation string) (DeleteResult, error) {
	t.Helper()
	p, err := s.PreviewDelete(context.Background(), allow, []string{item})
	if err != nil {
		return DeleteResult{}, err
	}
	return s.Delete(context.Background(), allow, "test-owner", DeleteRequest{ItemIDs: []string{item}, DeleteFiles: true, Confirmation: p.Confirmation, ExpectedRevision: p.Revision, OperationID: operation})
}
func TestTrashRetainsMetadataAndRestoresOriginalIdentity(t *testing.T) {
	s, db, _, item, _ := deletionFixture(t, 7)
	id := entityID(t, db, item)
	if _, err := db.Exec(`INSERT INTO metadata_details VALUES(?,'owner','custom-id','','2026-09-22');`, id); err != nil {
		t.Fatal(err)
	}
	result, err := deleteItem(t, s, item, "durable-trash")
	if err != nil {
		t.Fatal(err)
	}
	var count, available int
	if err = db.QueryRow(`SELECT count(*) FROM metadata_details WHERE item_id=?`, id).Scan(&count); err != nil || count != 1 {
		t.Fatalf("metadata lost: %d %v", count, err)
	}
	if err = db.QueryRow(`SELECT max(available) FROM inventory_item_availability WHERE item_id=?`, id).Scan(&available); err != nil || available != 0 {
		t.Fatalf("held item available: %d %v", available, err)
	}
	restored, err := s.RestoreFromTrash(context.Background(), allow, result.Receipts[0].TrashEntryID, "restore-original")
	if err != nil || restored.Reindex || restored.Restored != 2 {
		t.Fatalf("restore %+v %v", restored, err)
	}
	if err = db.QueryRow(`SELECT max(available) FROM inventory_item_availability WHERE item_id=?`, id).Scan(&available); err != nil || available != 1 {
		t.Fatalf("restored identity unavailable: %d %v", available, err)
	}
	if err = db.QueryRow(`SELECT count(*) FROM metadata_details WHERE item_id=?`, id).Scan(&count); err != nil || count != 1 {
		t.Fatalf("metadata changed after restore: %v", err)
	}
}
func TestDeleteReceiptFailureRollsBackBytesEvenWithZeroRetention(t *testing.T) {
	for _, days := range []int{0, 7} {
		t.Run(string(rune('0'+days)), func(t *testing.T) {
			s, db, _, item, files := deletionFixture(t, days)
			if _, err := db.Exec(`CREATE TRIGGER inject_receipt_failure BEFORE INSERT ON admin_receipts WHEN NEW.scope='media-delete' BEGIN SELECT RAISE(ABORT,'injected failure'); END;`); err != nil {
				t.Fatal(err)
			}
			if _, err := deleteItem(t, s, item, "failed-delete"); err == nil {
				t.Fatal("expected injected error")
			}
			for _, file := range files {
				raw, err := os.ReadFile(file)
				if err != nil || len(raw) != 11 {
					t.Fatalf("media not rolled back: %s %v", file, err)
				}
			}
			var count int
			if err := db.QueryRow(`SELECT count(*) FROM catalog_entities WHERE public_id=pid_blob(?)`, item).Scan(&count); err != nil || count != 1 {
				t.Fatal("catalog rollback failed", err)
			}
			if err := s.RecoverFileOperations(context.Background()); err != nil {
				t.Fatal(err)
			}
		})
	}
}
func TestRestartRecoversUncommittedMoveAndFinishesCommittedPurge(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "rollback", true: "commit"}[committed], func(t *testing.T) {
			s, db, root, _, files := deletionFixture(t, 7)
			ctx := context.Background()
			j := s.fileJournal("media-delete", "crashed-operation", "digest")
			trash := filepath.Join(root, "trash", "crash-fixture")
			to := filepath.Join(trash, "file.mkv")
			if err := j.move(ctx, files[0], to); err != nil {
				t.Fatal(err)
			}
			if committed {
				if err := j.removeAfterCommit(trash); err != nil {
					t.Fatal(err)
				}
				if err := dbwork.WithWriteTx(ctx, db, dbwork.ClassInteractive, func(tx *sql.Tx) error {
					return saveReceipt(ctx, tx, j.Scope, j.OperationID, j.Digest, DeleteResult{}, 1)
				}); err != nil {
					t.Fatal(err)
				}
			}
			restarted := NewAt(db, root)
			if err := restarted.RecoverFileOperations(ctx); err != nil {
				t.Fatal(err)
			}
			_, originalErr := os.Stat(files[0])
			_, trashErr := os.Stat(to)
			if !errors.Is(trashErr, os.ErrNotExist) || committed && !errors.Is(originalErr, os.ErrNotExist) || !committed && originalErr != nil {
				t.Fatalf("wrong recovery direction: original=%v trash=%v", originalErr, trashErr)
			}
			if err := restarted.RecoverFileOperations(ctx); err != nil {
				t.Fatal("replay", err)
			}
		})
	}
}
func TestTrashPurgeFailureKeepsBytesAndMetadata(t *testing.T) {
	s, db, root, item, _ := deletionFixture(t, 7)
	result, err := deleteItem(t, s, item, "purge-test-delete")
	if err != nil {
		t.Fatal(err)
	}
	id := result.Receipts[0].TrashEntryID
	if _, err = db.Exec(`CREATE TRIGGER prevent_item_purge BEFORE DELETE ON catalog_entities BEGIN SELECT RAISE(ABORT,'purge blocked'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.EmptyTrash(context.Background(), allow, EmptyTrashRequest{Confirmation: "1", OperationID: "purge-blocked"}); err == nil {
		t.Fatal("expected failure")
	}
	entries, err := os.ReadDir(filepath.Join(root, "trash", id))
	if err != nil || len(entries) != 2 {
		t.Fatalf("trash removed before catalog commit: %v", err)
	}
	if _, err = db.Exec(`DROP TRIGGER prevent_item_purge`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.EmptyTrash(context.Background(), allow, EmptyTrashRequest{Confirmation: "1", OperationID: "purge-complete"}); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = db.QueryRow(`SELECT count(*) FROM catalog_entities WHERE public_id=pid_blob(?)`, item).Scan(&count); err != nil || count != 0 {
		t.Fatalf("item not purged: %d %v", count, err)
	}
}
func TestRestoreConflictNeverOverwritesOriginal(t *testing.T) {
	s, _, _, item, files := deletionFixture(t, 7)
	r, err := deleteItem(t, s, item, "conflict-delete")
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(files[1], []byte("new bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RestoreFromTrash(context.Background(), allow, r.Receipts[0].TrashEntryID, "conflict-restore"); !errors.Is(err, ErrConflict) {
		t.Fatalf("want conflict, got %v", err)
	}
	raw, err := os.ReadFile(files[1])
	if err != nil || string(raw) != "new bytes" {
		t.Fatal("overwrote original", err)
	}
	if _, err = os.Stat(files[0]); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("partially restored before conflict")
	}
}

// Seed every restrictive asset edge against the real installed schema, including
// the session that blocks deleting the item one statement before asset cleanup.
func TestPermanentPurgeAllRestrictiveAssetReferences(t *testing.T) {
	s, db, _, item, files := deletionFixture(t, 0)
	itemID := entityID(t, db, item)
	assetTokens := entityAssets(t, db, itemID)
	if len(assetTokens) == 0 {
		t.Fatal("fixture item has no linked assets")
	}
	assetToken := assetTokens[0]
	queries := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO playback_sessions(id,session_hash,account_id,profile_id,item_id,asset_id,generation,state,grant_hash,grant_token,expires_at,request_id,duration) VALUES('session','hash','account','profile',?,?,1,'ended','grant','token','2099-01-01T00:00:00Z','request',60)`, []any{itemID, assetToken}},
		{`INSERT INTO audio_tag_evidence VALUES('lib-1',?,'title','tag','Title')`, []any{assetToken}},
		{`INSERT INTO audio_source_metadata VALUES('lib-1',?,'')`, []any{assetToken}},
		{`INSERT INTO episodic_sources VALUES('lib-1',?,0,'','episode.mkv')`, []any{assetToken}},
		{`INSERT INTO item_extras VALUES(?,'lib-1','/media','trailer','Trailer',?,'part1.mkv')`, []any{itemID, assetToken}},
		{`INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns) SELECT 'object','lib-1',?,incarnation,'part1.mkv','r1','{}',11,0 FROM library_sources WHERE id='lib-1'`, []any{assetToken}},
		{`INSERT INTO lyric_resources VALUES('lyric',?,?,'version','library','local','account','profile',1,0)`, []any{itemID, assetToken}},
		{`INSERT INTO lyric_candidates VALUES('candidate',?,?,'version','actor','target',1,'{}','{}','en','2099-01-01',1)`, []any{itemID, assetToken}},
		{`INSERT INTO subtitle_resources(id,item_id,source_id,scope,owner,current_revision) VALUES('subtitle',?,?,'shared','',1)`, []any{itemID, assetToken}},
		{`INSERT INTO dvr_catalog_provenance(item_id,recording_id,asset_id,source_id,channel_id,programme_id,guide_generation,artifact_digest,captured_start_ms,captured_end_ms,state) VALUES(?,'recording',?,'source','channel','programme','guide','digest',1,2,'completed')`, []any{itemID, assetToken}},
	}
	for _, q := range queries {
		if _, err := db.Exec(q.query, q.args...); err != nil {
			t.Fatalf("seed %s: %v", q.query, err)
		}
	}
	if _, err := deleteItem(t, s, item, "purge-all-edges"); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"catalog_entities", "catalog_assets", "inventory_objects", "audio_tag_evidence", "audio_source_metadata", "episodic_sources", "lyric_resources", "lyric_candidates", "subtitle_resources", "dvr_catalog_provenance", "playback_sessions"} {
		var n int
		if err := db.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("surviving %s: %d %v", table, n, err)
		}
	}
	for _, file := range files {
		if _, err := os.Stat(file); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("file not purged: %v", err)
		}
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign key violation after purge")
	}
}
func TestTrashRetainsSharedArtworkUntilLastItemPurged(t *testing.T) {
	s, db, root, item, _ := deletionFixture(t, 7)
	itemID := entityID(t, db, item)
	other := addCatalogTestEntity(t, db, "lib-1", compactcatalog.ItemKey(root, filepath.Join(root, "other.mkv"), 0), compactcatalog.Movie, "Other")
	otherID := entityID(t, db, other)
	if _, err := db.Exec(`INSERT INTO artwork_objects VALUES('digest','image/jpeg',1,1,1,'2000-01-01T00:00:00Z','ready')`); err != nil {
		t.Fatal(err)
	}
	for _, candidate := range []struct {
		id       string
		entityID int64
	}{
		{id: "art1", entityID: itemID},
		{id: "art2", entityID: otherID},
	} {
		if _, err := db.Exec(`INSERT INTO artwork_candidates(id,kind,entity_id,role,provider,image_id,origin,attribution,source_fence,observed_at) VALUES(?,'item',?,'poster','owner',?,'upload','','fence','2000-01-01')`, candidate.id, candidate.entityID, candidate.id); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`INSERT INTO artwork_selections(kind,entity_id,role,candidate_id,digest,thumbnail_digest,actor,observed_at) VALUES('item',?,'poster',?,'digest','digest','owner','2000-01-01')`, candidate.entityID, candidate.id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := deleteItem(t, s, item, "shared-art-trash"); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM artwork_selections`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("trash released artwork: %d %v", n, err)
	}
	if _, err := s.EmptyTrash(context.Background(), allow, EmptyTrashRequest{Confirmation: "1", OperationID: "shared-art-purge"}); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM artwork_selections WHERE entity_id=? AND digest='digest'`, otherID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("purge deleted another item's artwork: %d %v", n, err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM artwork_candidates WHERE entity_id=?`, itemID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("purge retained dead item reference: %d %v", n, err)
	}
}

func TestDeleteKeepsAssetReferencedOnlyByAnotherItemsExtra(t *testing.T) {
	s, db, root, item, files := deletionFixture(t, 0)
	itemID := entityID(t, db, item)
	assetTokens := entityAssets(t, db, itemID)
	if len(assetTokens) == 0 {
		t.Fatal("fixture item has no linked assets")
	}
	other := addCatalogTestEntity(t, db, "lib-1", compactcatalog.ExtraKey(root, filepath.Join(root, "shared-extra.mkv")), compactcatalog.Extra, "Other")
	otherID := entityID(t, db, other)
	if _, err := db.Exec(`INSERT INTO item_extras(item_id,library_id,parent_path,kind,title,asset_id,relative_path) VALUES(?,'lib-1','','trailer','Shared',?,'shared')`, otherID, assetTokens[0]); err != nil {
		t.Fatal(err)
	}
	var shared string
	if err := db.QueryRow(`SELECT a.path FROM item_extras x JOIN catalog_assets a ON a.token=x.asset_id WHERE x.item_id=?`, otherID).Scan(&shared); err != nil {
		t.Fatal(err)
	}
	result, err := deleteItem(t, s, item, "extra-shared")
	if err != nil {
		t.Fatal(err)
	}
	if result.Receipts[0].FilesKept != 1 {
		t.Fatalf("shared extra not protected: %+v", result)
	}
	if _, err = os.Stat(shared); err != nil {
		t.Fatalf("another item's media removed: %v", err)
	}
	for _, f := range files {
		if f != shared {
			if _, err = os.Stat(f); !os.IsNotExist(err) {
				t.Fatalf("unshared media retained: %v", err)
			}
		}
	}
}

func TestRecoveryRemovesJournaledPartialCrossVolumeCopy(t *testing.T) {
	s, db, root, _, files := deletionFixture(t, 7)
	j := s.fileJournal("media-delete", "partial-copy", "digest")
	to := filepath.Join(root, "trash", "partial", "media")
	temporary := filepath.Join(root, "trash", "partial", ".portico-move-test")
	if err := os.MkdirAll(filepath.Dir(to), 0700); err != nil {
		t.Fatal(err)
	}
	j.Moves = []fileMove{{From: files[0], To: to, Temporary: temporary}}
	if err := j.save(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(temporary, []byte("partial bytes"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := NewAt(db, root).RecoverFileOperations(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(files[0]); err != nil {
		t.Fatal("source lost", err)
	}
	if _, err := os.Stat(temporary); !os.IsNotExist(err) {
		t.Fatal("partial copy leaked", err)
	}
}

func TestFailedMoveNeverDeletesEqualContentDestination(t *testing.T) {
	s, _, root, _, files := deletionFixture(t, 7)
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "concurrent-original")
	if err = os.WriteFile(destination, raw, 0600); err != nil {
		t.Fatal(err)
	}
	j := s.fileJournal("trash-restore", "concurrent-equal", "digest")
	if err = j.move(context.Background(), files[0], destination); err == nil {
		t.Fatal("existing destination accepted")
	}
	if err = s.settleFileOperation(j, nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{files[0], destination} {
		if got, err := os.ReadFile(path); err != nil || string(got) != string(raw) {
			t.Fatalf("unowned bytes removed: %s %v", path, err)
		}
	}
}

func TestRecoveryDoesNotInferDestinationOwnershipFromEqualBytes(t *testing.T) {
	_, _, root, _, files := deletionFixture(t, 7)
	raw, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "unowned-destination")
	if err = os.WriteFile(destination, raw, 0600); err != nil {
		t.Fatal(err)
	}
	sourceID, err := fileIdentity(files[0])
	if err != nil {
		t.Fatal(err)
	}
	move := fileMove{From: files[0], To: destination, DestinationIdentity: sourceID}
	if err = reverseMove(context.Background(), move); !errors.Is(err, ErrConflict) {
		t.Fatalf("unowned destination accepted: %v", err)
	}
	if _, err = os.Stat(destination); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryKeepsBothVersionsWhenOriginalChanged(t *testing.T) {
	s, _, root, _, files := deletionFixture(t, 7)
	destination := filepath.Join(root, "owned-destination")
	j := s.fileJournal("media-delete", "different-original", "digest")
	if err := j.move(context.Background(), files[0], destination); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(files[0], []byte("new user version"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := reverseMove(context.Background(), j.Moves[0]); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed original accepted: %v", err)
	}
	for _, path := range []string{files[0], destination} {
		if _, err := os.Stat(path); err != nil {
			t.Fatal("version lost", err)
		}
	}
}

func TestCrossVolumeMovePreservesSourceFacts(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "original")
	to := filepath.Join(root, "new", "nested", "destination")
	raw := []byte("media snapshot with original facts")
	if err := os.WriteFile(from, raw, 0640); err != nil {
		t.Fatal(err)
	}
	stamp := time.Date(2020, 1, 2, 3, 4, 5, 6000000, time.UTC)
	if err := os.Chtimes(from, stamp, stamp); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(from)
	if err != nil {
		t.Fatal(err)
	}
	if err = moveFileWithLink(context.Background(), from, to, "", nil, nil, func(string, string) error { return syscall.EXDEV }); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(to)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(to)
	if err != nil || string(got) != string(raw) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || before.Mode().Perm() != after.Mode().Perm() {
		t.Fatalf("source facts changed: before=%v after=%v err=%v", before, after, err)
	}
	if _, err = os.Stat(from); !os.IsNotExist(err) {
		t.Fatal("source retained", err)
	}
}

func TestHardLinkMoveFencesConcurrentSourceReplacement(t *testing.T) {
	root := t.TempDir()
	from := filepath.Join(root, "original")
	to := filepath.Join(root, "destination")
	if err := os.WriteFile(from, []byte("original previewed media"), 0600); err != nil {
		t.Fatal(err)
	}
	err := moveFileWithLink(context.Background(), from, to, "", nil, nil, func(a, b string) error {
		if err := os.Rename(a, a+".saved"); err != nil {
			return err
		}
		if err := os.WriteFile(a, []byte("concurrent replacement"), 0600); err != nil {
			return err
		}
		return os.Link(a, b)
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("replacement accepted: %v", err)
	}
	if raw, err := os.ReadFile(from); err != nil || string(raw) != "concurrent replacement" {
		t.Fatalf("replacement removed: %q %v", raw, err)
	}
}
