package persistence

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// Moving the state folder rebases Portico's own absolute paths on first
// start: a copied folder starts, the DVR library root and asset paths point
// at the new folder, and the recorded folder is stored.
func TestRebaseStatePathsOnMovedState(t *testing.T) {
	ctx := context.Background()
	oldState := t.TempDir()
	db, err := Open(filepath.Join(oldState, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`INSERT INTO libraries(id,name,kind,root) VALUES('dvr','Recorded TV','recordings',?)`, filepath.Join(oldState, "recordings")); err != nil {
		t.Fatal(err)
	}
	if err = RebaseStatePaths(ctx, db, oldState); err != nil {
		t.Fatal(err)
	}
	var recorded string
	if err = db.QueryRowContext(ctx, `SELECT value FROM configuration WHERE key='state_directory'`).Scan(&recorded); err != nil || recorded != oldState {
		t.Fatalf("recorded folder: %q %v", recorded, err)
	}
	// A second start in place rewrites nothing.
	if _, err = db.Exec(`UPDATE libraries SET root=? WHERE id='dvr'`, filepath.Join(oldState, "recordings")); err != nil {
		t.Fatal(err)
	}
	db.Close()

	// Stop, copy the whole folder, point at the copy, start.
	newState := t.TempDir()
	raw, err := os.ReadFile(filepath.Join(oldState, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(newState, "server.sqlite"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	moved, err := Open(filepath.Join(newState, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer moved.Close()
	if err = RebaseStatePaths(ctx, moved, newState); err != nil {
		t.Fatal(err)
	}
	var root string
	if err = moved.QueryRowContext(ctx, `SELECT root FROM libraries WHERE id='dvr'`).Scan(&root); err != nil || root != filepath.Join(newState, "recordings") {
		t.Fatalf("rebased root: %q %v", root, err)
	}
	if err = moved.QueryRowContext(ctx, `SELECT value FROM configuration WHERE key='state_directory'`).Scan(&recorded); err != nil || recorded != newState {
		t.Fatalf("recorded folder: %q %v", recorded, err)
	}
	// Values outside the old folder are never touched.
	if _, err = moved.Exec(`INSERT INTO libraries(id,name,kind,root) VALUES('ext','External','movie','/media/films')`); err != nil {
		t.Fatal(err)
	}
	if err = RebaseStatePaths(ctx, moved, newState); err != nil {
		t.Fatal(err)
	}
	if err = moved.QueryRowContext(ctx, `SELECT root FROM libraries WHERE id='ext'`).Scan(&root); err != nil || root != "/media/films" {
		t.Fatalf("external root rewritten: %q %v", root, err)
	}
}
