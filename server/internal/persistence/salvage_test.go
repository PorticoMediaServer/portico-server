package persistence

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"portico.local/server/internal/dbwork"
)

func copyFileForTest(t *testing.T, from, to string) {
	t.Helper()
	in, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	out, err := os.OpenFile(to, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err = out.Close(); err != nil {
		t.Fatal(err)
	}
}

func countIn(t *testing.T, path, query string) int {
	t.Helper()
	candidate, err := dbwork.OpenCandidate(filepath.Dir(path), filepath.Base(path))
	if err != nil {
		t.Fatal(err)
	}
	defer candidate.Close()
	var n int
	if err = candidate.QueryRowContext(context.Background(), query).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// liveLibraries opens a server database and writes n libraries through the
// gate, leaving them in the live WAL (the writer never checkpoints).
func liveLibraries(t *testing.T, n int) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "server.sqlite")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	for i := 0; i < n; i++ {
		if _, err = dbwork.ExecWrite(context.Background(), db, dbwork.ClassInteractive, `INSERT INTO libraries(id,name,kind,root) VALUES(?,?,'movie',?)`, fmt.Sprint(i), fmt.Sprintf("Library %d", i), fmt.Sprintf("/media/%04d/%s", i, longName)); err != nil {
			t.Fatal(err)
		}
	}
	return db, path
}

const longName = "a-reasonably-long-folder-name-so-rows-span-several-pages-of-the-table-and-its-index"

// JUSTIN-ANSWERS #21: a copy of a running server's folder (database plus its
// live -wal, taken without stopping anything) is restorable. SQLite replays
// every complete transaction in the copied log; a log cut part-way through a
// frame loses only the incomplete tail, never consistency.
func TestRestoreAcceptsACopyOfARunningServer(t *testing.T) {
	_, live := liveLibraries(t, 200)
	if info, err := os.Stat(live + "-wal"); err != nil || info.Size() == 0 {
		t.Fatalf("fixture has no live WAL: %v", err)
	}
	for _, torn := range []bool{false, true} {
		t.Run(fmt.Sprintf("torn=%v", torn), func(t *testing.T) {
			dir := t.TempDir()
			copied := filepath.Join(dir, "server.sqlite")
			copyFileForTest(t, live, copied)
			copyFileForTest(t, live+"-wal", copied+"-wal")
			if torn {
				info, _ := os.Stat(copied + "-wal")
				if err := os.Truncate(copied+"-wal", info.Size()-1000); err != nil {
					t.Fatal(err)
				}
			}
			report, err := InspectCandidate(context.Background(), dir, "server.sqlite")
			if err != nil || report.Integrity != "ok" {
				t.Fatalf("live copy: %+v %v", report, err)
			}
			n := countIn(t, copied, `SELECT count(*) FROM libraries`)
			if !torn && n != 200 || torn && (n == 0 || n > 200) {
				t.Fatalf("live copy holds %d libraries", n)
			}
			if info, err := os.Stat(copied + "-wal"); err == nil && info.Size() != 0 {
				t.Fatal("inspection left the log unfolded")
			}
		})
	}
}

// pageOf returns a page of the named b-tree, from dbstat.
func pageOf(t *testing.T, path, name, pagetype string, nth int) int64 {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var page int64
	if err = db.QueryRow(`SELECT pageno FROM dbstat WHERE name=? AND pagetype=? ORDER BY pageno LIMIT 1 OFFSET ?`, name, pagetype, nth).Scan(&page); err != nil {
		t.Fatalf("no %s page %d of %s: %v", pagetype, nth, name, err)
	}
	return page
}

func folded(t *testing.T, db *sql.DB, path string) {
	t.Helper()
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path + "-wal"); err == nil && info.Size() != 0 {
		t.Fatal("closing did not fold the log")
	}
}

func overwritePage(t *testing.T, path string, page int64, bytes []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err = file.WriteAt(bytes, (page-1)*4096); err != nil {
		t.Fatal(err)
	}
}

func readPage(t *testing.T, path string, page int64) []byte {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	buf := make([]byte, 4096)
	if _, err = file.ReadAt(buf, (page-1)*4096); err != nil {
		t.Fatal(err)
	}
	return buf
}

// An index that no longer matches its table is rebuilt from the rows, and
// the owner is told it was repaired with nothing lost.
func TestRestoreRebuildsADamagedIndex(t *testing.T) {
	db, path := liveLibraries(t, 20)
	folded(t, db, path)
	// An older copy of the index page: the rows added since are missing from it.
	page := pageOf(t, path, indexOn(t, path, "libraries"), "leaf", 0)
	stale := readPage(t, path, page)
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for i := 20; i < 25; i++ {
		if _, err = dbwork.ExecWrite(context.Background(), db, dbwork.ClassInteractive, `INSERT INTO libraries(id,name,kind,root) VALUES(?,?,'show',?)`, fmt.Sprint(i), fmt.Sprint(i), fmt.Sprintf("/x/%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	folded(t, db, path)
	overwritePage(t, path, page, stale)
	report, err := InspectCandidate(context.Background(), filepath.Dir(path), filepath.Base(path))
	if err != nil || report.Integrity != "recovered" || report.Recovery == nil || !report.Recovery.IndexesRebuilt || report.Recovery.Salvaged {
		t.Fatalf("index damage: %+v %+v %v", report, report.Recovery, err)
	}
	if n := countIn(t, path, `SELECT count(*) FROM libraries`); n != 25 {
		t.Fatalf("index repair lost rows: %d", n)
	}
}

// indexOn names one index of table that has leaf pages.
func indexOn(t *testing.T, path, table string) string {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var name string
	if err = db.QueryRow(`SELECT s.name FROM sqlite_schema s WHERE s.type='index' AND s.tbl_name=? AND EXISTS(SELECT 1 FROM dbstat d WHERE d.name=s.name AND d.pagetype='leaf') ORDER BY s.name LIMIT 1`, table).Scan(&name); err != nil {
		t.Fatal(err)
	}
	return name
}

// A table page that cannot be read at all: restore salvages every other row
// into a fresh database, says which table lost rows, and the result passes a
// full integrity check and opens as this baseline.
func TestRestoreSalvagesADamagedTable(t *testing.T) {
	db, path := liveLibraries(t, 600)
	// A setting the owner changed from its seeded default.
	if _, err := dbwork.ExecWrite(context.Background(), db, dbwork.ClassInteractive, `UPDATE maintenance_policy SET background_priority='normal',revision=2`); err != nil {
		t.Fatal(err)
	}
	folded(t, db, path)
	page := pageOf(t, path, "libraries", "leaf", 3)
	garbage := make([]byte, 4096)
	for i := range garbage {
		garbage[i] = 0xA5
	}
	overwritePage(t, path, page, garbage)
	report, err := InspectCandidate(context.Background(), filepath.Dir(path), filepath.Base(path))
	if err != nil || report.Integrity != "recovered" || report.Recovery == nil || !report.Recovery.Salvaged {
		t.Fatalf("table damage: %+v %v", report, err)
	}
	damaged := fmt.Sprint(report.Recovery.DamagedTables)
	if damaged != "[libraries]" {
		t.Fatalf("damaged tables %s", damaged)
	}
	n := countIn(t, path, `SELECT count(*) FROM libraries`)
	if n >= 600 || n < 500 {
		t.Fatalf("salvage kept %d of 600 libraries", n)
	}
	if n := countIn(t, path, `SELECT count(*) FROM configuration`); n == 0 {
		t.Fatal("salvage lost the configuration")
	}
	// Salvage keeps the owner's settings, not the fresh baseline's defaults.
	if n := countIn(t, path, `SELECT count(*) FROM maintenance_policy WHERE background_priority='normal' AND revision=2`); n != 1 {
		t.Fatal("salvage reset a setting to its default")
	}
	// The result is an ordinary database of this baseline.
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("salvaged database does not open: %v", err)
	}
	defer reopened.Close()
	var ok string
	if err = reopened.QueryRow(`PRAGMA integrity_check`).Scan(&ok); err != nil || ok != "ok" {
		t.Fatalf("salvaged integrity %q %v", ok, err)
	}
	var triggers int
	if err = reopened.QueryRow(`SELECT count(*) FROM sqlite_schema WHERE type='trigger'`).Scan(&triggers); err != nil || triggers == 0 {
		t.Fatalf("salvage dropped the triggers: %d %v", triggers, err)
	}
}
