package persistence

import (
	"context"
	"errors"
	"fmt"
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

// The rebase is a durable job: a start interrupted part-way resumes the same
// move from its saved position, so a path is never rewritten twice, even when
// the new folder sits inside the old one; and the folder it was moving to is
// recorded only once every column is done.
func TestRebaseResumesAnInterruptedMoveExactlyOnce(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(filepath.Join(dir, "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	oldState := "/srv/portico"
	newState := "/srv/portico/moved" // inside the old folder: a double rewrite would show
	if _, err = db.Exec(`INSERT INTO configuration(key,value) VALUES('state_directory',?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, oldState); err != nil {
		t.Fatal(err)
	}
	const rows = 700 // several windows and change batches
	for i := 0; i < rows; i++ {
		root := fmt.Sprintf("%s/recordings/%d", oldState, i)
		if i%3 == 0 {
			root = fmt.Sprintf("/media/%d", i) // never touched
		}
		if _, err = db.Exec(`INSERT INTO libraries(id,name,kind,root) VALUES(?,?,'recordings',?)`, fmt.Sprint(i), fmt.Sprint(i), root); err != nil {
			t.Fatal(err)
		}
	}
	// Interrupt: the job exists and is half-way through the first column.
	if _, err = db.Exec(`INSERT INTO state_rebase_jobs(singleton,old_prefix,new_prefix,registry_version,phase,after_rowid,started_ms) VALUES(1,?,?,?,0,0,0)`, oldState, newState, rebaseRegistryVersion); err != nil {
		t.Fatal(err)
	}
	var half int64
	if err = db.QueryRow(`SELECT rowid FROM libraries ORDER BY rowid LIMIT 1 OFFSET 350`).Scan(&half); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE libraries SET root=?||substr(root,length(?)+1) WHERE rowid<=? AND (root=? OR root LIKE ?)`, newState, oldState, half, oldState, oldState+"/%"); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE state_rebase_jobs SET after_rowid=?`, half); err != nil {
		t.Fatal(err)
	}
	// The next start finishes it — and then moves on to where the folder is now.
	current := t.TempDir()
	if err = RebaseStatePaths(ctx, db, current); err != nil {
		t.Fatal(err)
	}
	result, err := db.Query(`SELECT id,root FROM libraries`)
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()
	for result.Next() {
		var id, root string
		if err = result.Scan(&id, &root); err != nil {
			t.Fatal(err)
		}
		var n int
		fmt.Sscan(id, &n)
		want := fmt.Sprintf("%s/recordings/%d", current, n)
		if n%3 == 0 {
			want = fmt.Sprintf("/media/%d", n)
		}
		if root != want {
			t.Fatalf("library %d: %q, want %q", n, root, want)
		}
	}
	var jobs int
	var recorded string
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM state_rebase_jobs),(SELECT value FROM configuration WHERE key='state_directory')`).Scan(&jobs, &recorded); err != nil || jobs != 0 || recorded != current {
		t.Fatalf("job left %d, recorded %q: %v", jobs, recorded, err)
	}
}

// A job left by a build that walked a different registry is refused, not
// guessed at.
func TestRebaseRefusesAJobFromAnotherRegistry(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "server.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err = db.Exec(`INSERT INTO state_rebase_jobs(singleton,old_prefix,new_prefix,registry_version,phase,after_rowid,started_ms) VALUES(1,'/a','/b',?,0,0,0)`, rebaseRegistryVersion+1); err != nil {
		t.Fatal(err)
	}
	if err = RebaseStatePaths(context.Background(), db, t.TempDir()); !errors.Is(err, ErrRebaseRegistry) {
		t.Fatalf("foreign job: %v", err)
	}
}
