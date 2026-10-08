package catalog

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/persistence"
)

func TestFinalInventoryForgetPurgesOnlyExhaustedLogicalItems(t *testing.T) {
	db, err := persistence.Open(filepath.Join(t.TempDir(), "db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	cat := New(db)
	lib, err := cat.Create("Movies", "movie", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	c := catalogtest.New(t, db)
	handle := c.Handle(lib.ID)
	var root string
	if err = db.QueryRow(`SELECT root FROM catalog_libraries WHERE library_id=?`, lib.ID).Scan(&root); err != nil {
		t.Fatal(err)
	}
	source, err := cat.LibrarySource(context.Background(), lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	items := map[string]catalogtest.Item{}
	for _, name := range []string{"exhausted", "versioned", "unknown", "held"} {
		path := filepath.Join(root, name+".mkv")
		items[name] = c.Entity(compactcatalog.Entity{Library: handle, Kind: compactcatalog.Movie, Key: compactcatalog.ItemKey(root, path, 0), Title: "Film", Added: "2026-01-01T00:00:00.000Z"}, nil)
	}
	assetIDs := map[string]int64{}
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		for _, token := range []string{"gone", "alternate", "untracked"} {
			path := filepath.Join(root, token+".mkv")
			assetIDs[token], err = compactcatalog.CreateAssetTx(ctx, tx, token, compactcatalog.Asset{Path: path, Size: 1, ModifiedNS: 1, Container: "mkv", VideoCodec: "h264", Width: 1, Height: 1, Duration: 1}, true)
			if err != nil {
				return err
			}
		}
		for _, item := range items {
			if err = compactcatalog.LinkAssetTx(ctx, tx, item.ID, assetIDs["gone"], compactcatalog.Link{}); err != nil {
				return err
			}
		}
		for _, name := range []string{"versioned", "unknown"} {
			token := "alternate"
			if name == "unknown" {
				token = "untracked"
			}
			if err = compactcatalog.LinkAssetTx(ctx, tx, items[name].ID, assetIDs[token], compactcatalog.Link{Part: 1}); err != nil {
				return err
			}
		}
		for _, item := range items {
			if _, err = tx.ExecContext(ctx, `INSERT INTO metadata_details(item_id,provider,provider_id,source_url,observed_at) VALUES(?,'owner','id','','now')`, item.ID); err != nil {
				return err
			}
		}
		return nil
	})
	for _, row := range []struct {
		token, state string
		retired      int
	}{{"gone", "forgotten", 1}, {"alternate", "trashed", 0}} {
		if _, err = db.Exec(`INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns,state,retired) VALUES(?,?,?,?,?,'r','{}',1,0,?,?)`, row.token, source.ID, row.token, source.Incarnation, row.token, row.state, row.retired); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(`INSERT INTO admin_trash(id,library_id,item_id,title,kind,files_json,file_count,bytes,trashed_ms,expires_ms,state,actor,operation_id) VALUES('hold',?,?,'Held','movie','[]',0,0,1,9999999999999,'held','owner','held-op')`, lib.ID, items["held"].ID); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	purge := func(asset string) {
		t.Helper()
		if err := dbwork.WithWriteTx(ctx, db, dbwork.ClassInteractive, func(tx *sql.Tx) error { return purgeForgottenInventoryItemsTx(ctx, tx, lib.ID, asset) }); err != nil {
			t.Fatal(err)
		}
	}
	purge("gone")
	countEntity := func(id int64) int {
		t.Helper()
		var n int
		if err = db.QueryRow(`SELECT count(*) FROM catalog_entities WHERE id=?`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	countMetadata := func(id int64) int {
		t.Helper()
		var n int
		if err = db.QueryRow(`SELECT count(*) FROM metadata_details WHERE item_id=?`, id).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	countAsset := func(token string) int {
		t.Helper()
		var n int
		if err = db.QueryRow(`SELECT count(*) FROM catalog_assets WHERE token=?`, token).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if countEntity(items["held"].ID) != 1 || countMetadata(items["held"].ID) != 1 {
		t.Fatal("recoverable admin Trash metadata lost")
	}
	if countEntity(items["exhausted"].ID) != 0 || countMetadata(items["exhausted"].ID) != 0 {
		t.Fatal("final forget retained logical metadata")
	}
	if countEntity(items["versioned"].ID) != 1 || countEntity(items["unknown"].ID) != 1 || countAsset("gone") != 1 {
		t.Fatal("recoverable/unknown/shared alternative lost")
	}
	if _, err = db.Exec(`UPDATE inventory_objects SET retired=1,state='forgotten' WHERE id='alternate'`); err != nil {
		t.Fatal(err)
	}
	purge("alternate")
	if countEntity(items["versioned"].ID) != 0 || countAsset("alternate") != 0 || countEntity(items["unknown"].ID) != 1 || countAsset("gone") != 1 {
		t.Fatal("final version purge broke ownership")
	}
}
