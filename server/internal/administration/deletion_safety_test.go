package administration

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"portico.local/server/internal/compactcatalog"
	"testing"
)

func TestDeleteRejectsParentSymlinkEscape(t *testing.T) {
	s, _, root, item, paths := deletionFixture(t)
	media := filepath.Join(root, "media")
	outside := t.TempDir()
	if e := os.Rename(media, filepath.Join(outside, "outside-media")); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(filepath.Join(outside, "outside-media"), media); e != nil {
		t.Fatal(e)
	}
	out, e := deleteItem(t, s, item, "delete-escaped")
	if e != nil {
		t.Fatal(e)
	}
	victim := filepath.Join(outside, "outside-media", filepath.Base(paths[0]))
	if _, e := os.Stat(victim); os.IsNotExist(e) {
		t.Fatalf("file outside library root deleted: %+v", out)
	}
}

func TestDeleteReportsNonRegularReplacement(t *testing.T) {
	s, db, _, item, paths := deletionFixture(t)
	if e := os.Remove(paths[0]); e != nil {
		t.Fatal(e)
	}
	if e := os.Mkdir(paths[0], 0700); e != nil {
		t.Fatal(e)
	}
	out, e := deleteItem(t, s, item, "delete-directory")
	if e != nil {
		t.Fatal(e)
	}
	if out.Removed != 0 || out.Receipts[0].Code != "file_changed" {
		var n int
		db.QueryRow(`SELECT count(*) FROM catalog_entities WHERE public_id=pid_blob(?)`, item).Scan(&n)
		t.Fatalf("non-regular replacement was treated as gone: %+v; titles remaining=%d", out, n)
	}
}

func TestDeleteDoesNotRemoveNewUnpreviewedVersion(t *testing.T) {
	s, db, root, item, _ := deletionFixture(t)
	p, e := s.PreviewDelete(context.Background(), allow, []string{item})
	if e != nil {
		t.Fatal(e)
	}
	newPath := filepath.Join(root, "media", "new-version.mkv")
	if e := os.WriteFile(newPath, []byte("new version"), 0600); e != nil {
		t.Fatal(e)
	}
	calls := 0
	auth := func(ctx context.Context, tx *sql.Tx) error {
		calls++
		if calls != 2 {
			return nil
		}
		var entity int64
		if e := tx.QueryRow(`SELECT id FROM catalog_entities WHERE public_id=pid_blob(?)`, item).Scan(&entity); e != nil {
			return e
		}
		a, _, e := compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{Path: newPath, Size: 11})
		if e != nil {
			return e
		}
		return compactcatalog.LinkAssetTx(ctx, tx, entity, a, compactcatalog.Link{})
	}
	out, e := s.Delete(context.Background(), auth, "owner", DeleteRequest{ItemIDs: []string{item}, DeleteFiles: true, Confirmation: p.Confirmation, ExpectedRevision: p.Revision, OperationID: "delete-new-version"})
	if e != nil {
		t.Fatal(e)
	}
	var n int
	db.QueryRow(`SELECT count(*) FROM catalog_assets WHERE path=?`, newPath).Scan(&n)
	if n != 1 {
		t.Fatalf("unpreviewed version lost from catalogue while its disk file remains: %+v", out)
	}
}
