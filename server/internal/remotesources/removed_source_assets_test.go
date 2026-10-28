package remotesources

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/storage"
)

// Removing a source tombstones it at once and leaves its files to a durable,
// bounded walk: the command's transaction touches no file, each background
// turn marks at most one slice unavailable, a restart carries on from the
// saved place, and nothing outside the source's root is touched.
func TestRemovedSourceFilesBecomeUnavailableInBoundedSlices(t *testing.T) {
	ctx := context.Background()
	f, _, identities, owner, _, _ := nativeSaveFixture(t)
	var root string
	if err := f.s.db.QueryRow(`SELECT root FROM remote_sources WHERE id='remote'`).Scan(&root); err != nil {
		t.Fatal(err)
	}
	const files = 3*removedSourceAssetBatch + 5
	paths := []string{root, root + "-sibling" + string(filepath.Separator) + "kept.mkv"}
	for i := 0; i < files; i++ {
		paths = append(paths, filepath.Join(root, fmt.Sprintf("dir%02d", i%7), fmt.Sprintf("f%04d.mkv", i)))
	}
	if err := dbwork.WithWriteTx(ctx, f.s.db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		for _, path := range paths {
			if _, _, err := compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{Path: path, Size: 1, ModifiedNS: 1, Container: "mkv", Duration: 60}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	available := func(where string, args ...any) int {
		t.Helper()
		var n int
		if err := f.s.db.QueryRow(`SELECT count(*) FROM catalog_assets WHERE available=1 AND `+where, args...).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}
	if n := available(`1`); n != len(paths) {
		t.Fatalf("fixture: %d of %d files available", n, len(paths))
	}
	actor, _ := json.Marshal([]string{owner.Authority, owner.AccountID, owner.ProfileID})
	if _, err := f.s.RemoveDAV(ctx, string(actor), "remote", nativeSaveOperation(700), 1, func(tx *sql.Tx) error { return currentLocalOwnerTx(ctx, tx, identities, owner) }); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Stat(ctx, filepath.Join(root, "dir00", "f0000.mkv")); err != storage.ErrRemoteOffline {
		t.Fatalf("the removed source still answers: %v", err)
	}
	if n := available(`1`); n != len(paths) {
		t.Fatalf("the removal command marked %d files itself", len(paths)-n)
	}
	turn := func() bool {
		t.Helper()
		mctx, cost := dbwork.Measure(ctx)
		more, err := f.s.markRemovedSourceAssets(mctx)
		if err != nil {
			t.Fatal(err)
		}
		if steps := cost().VMSteps; steps > 20000 {
			t.Fatalf("one turn read %d steps", steps)
		}
		return more
	}
	if !turn() {
		t.Fatal("the first slice reported nothing left")
	}
	if n := len(paths) - available(`1`); n == 0 || n > removedSourceAssetBatch+1 {
		t.Fatalf("one turn marked %d files", n)
	}
	// A restart keeps the saved place: a fresh service carries on.
	var after string
	if err := f.s.db.QueryRow(`SELECT after_path FROM remote_source_removals WHERE source_id='remote'`).Scan(&after); err != nil || after == "" {
		t.Fatalf("no saved place: %q %v", after, err)
	}
	restarted := &Service{db: f.s.db}
	for i := 0; ; i++ {
		more, err := restarted.markRemovedSourceAssets(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			break
		}
		if i > files {
			t.Fatal("the walk did not finish")
		}
	}
	if n := available(`(path=? OR substr(path,1,length(?)+1)=?||?)`, root, root, root, string(filepath.Separator)); n != 0 {
		t.Fatalf("%d of the removed source's files are still available", n)
	}
	if n := available(`path=?`, paths[1]); n != 1 {
		t.Fatal("a file beside the removed root was marked unavailable")
	}
	var queued int
	if err := f.s.db.QueryRow(`SELECT count(*) FROM remote_source_removals`).Scan(&queued); err != nil || queued != 0 {
		t.Fatalf("the finished walk stayed queued: %d %v", queued, err)
	}
}

// A queued walk whose source is no longer removed ends without touching a file.
func TestRemovedSourceWalkStopsForALiveSource(t *testing.T) {
	ctx := context.Background()
	f, _, _, _, _, _ := nativeSaveFixture(t)
	var root string
	if err := f.s.db.QueryRow(`SELECT root FROM remote_sources WHERE id='remote'`).Scan(&root); err != nil {
		t.Fatal(err)
	}
	if err := dbwork.WithWriteTx(ctx, f.s.db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		if _, _, err := compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{Path: filepath.Join(root, "a.mkv"), Size: 1, ModifiedNS: 1, Container: "mkv", Duration: 60}); err != nil {
			return err
		}
		_, err := tx.Exec(`INSERT INTO remote_source_removals(source_id,root) VALUES('remote',?)`, root)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for {
		more, err := f.s.markRemovedSourceAssets(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !more {
			break
		}
	}
	var available, queued int
	if err := f.s.db.QueryRow(`SELECT (SELECT count(*) FROM catalog_assets WHERE available=1),(SELECT count(*) FROM remote_source_removals)`).Scan(&available, &queued); err != nil {
		t.Fatal(err)
	}
	if available != 1 || queued != 0 {
		t.Fatalf("a live source's walk marked files or stayed queued: available %d, queued %d", available, queued)
	}
}
