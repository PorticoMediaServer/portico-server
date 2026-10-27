package catalog

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
)

// Private build/work/scope lifetimes may outlive public authority. Existing
// removal must still finish, without a new refusal, source FK or large cascade.
func TestReadingCompleteLibraryRemovalWithActivatedOverlay(t *testing.T) {
	c := catalogtest.Open(t)
	root := t.TempDir()
	lib := c.Library("path-removal", "Reading", "book", root)
	item := c.Entity(compactcatalog.Entity{Library: lib, Kind: compactcatalog.Ebook, Key: "book", Title: "Book"}, nil)
	asset, token := c.File(item.ID, filepath.Join(root, "Shelf", "Book.epub"), 0)
	var source, inc string
	if err := c.DB.QueryRow(`SELECT id,incarnation FROM library_sources WHERE library_id='path-removal'`).Scan(&source, &inc); err != nil {
		t.Fatal(err)
	}
	c.Exec(`INSERT INTO reading_publications VALUES(?,?,'epub')`, item.ID, asset)
	c.Exec(`INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns) VALUES('removal-object',?,?,?,'Shelf/Book.epub','r','{}',1,1)`, source, token, inc)
	c.Exec(`INSERT INTO reading_path_scope(library_id,source_id,incarnation,root_prefix,directory_key,completed,queued) VALUES(?,?,?,?,'Shelf',1,0)`, lib, source, inc, root+string(filepath.Separator))
	c.Exec(`INSERT OR IGNORE INTO reading_path_inputs VALUES(?,1)`, lib)
	c.Exec(`INSERT INTO reading_path_build(scope_id,desired,input_revision,source_generation,root_identity,job_id,phase,publish_sequence) VALUES(1,1,1,1,'r','job','flatten',1)`)
	c.Exec(`UPDATE reading_path_slot SET build_id=1,overlay_id=1`)
	c.Exec(`INSERT INTO reading_path_stage(build_id,item_id,asset_id,object_id,source_revision,author,author_source) VALUES(1,?,?,'removal-object','r','Shelf','folder')`, item.ID, asset)
	c.Exec(`UPDATE catalog_libraries SET retired=1 WHERE id=?`, lib)
	done := false
	for i := 0; i < 100 && !done; i++ {
		c.Write(func(ctx context.Context, tx *sql.Tx) error {
			var err error
			done, err = removeLibraryBatchTx(ctx, tx, "path-removal")
			return err
		})
	}
	if !done {
		t.Fatal("private Reading work blocked existing removal")
	}
	for i := 0; i < 100; i++ {
		c.Write(func(ctx context.Context, tx *sql.Tx) error {
			_, err := compactcatalog.StepReadingPaths(ctx, tx, 7)
			return err
		})
	}
	for _, table := range []string{"reading_path_head", "reading_path_stage", "reading_path_scope", "reading_path_inputs", "reading_path_build", "reading_path_dirty_work"} {
		var n int
		if err := c.DB.QueryRow(`SELECT count(*) FROM ` + table).Scan(&n); err != nil || n != 0 {
			t.Fatalf("orphan %s=%d err=%v", table, n, err)
		}
	}
}
