package mbaccounting_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/mbaccounting"
	"portico.local/server/internal/persistence"
)

func accountingDB(t *testing.T) *sql.DB {
	t.Helper()
	db, e := persistence.OpenFresh(filepath.Join(t.TempDir(), "accounting.sqlite"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { db.Close() })
	return db
}
func writer(t *testing.T, db *sql.DB, fn func(*sql.Tx) error) {
	t.Helper()
	if e := dbwork.WithWriteTx(context.Background(), db, dbwork.ClassBackgroundMedia, fn); e != nil {
		t.Fatal(e)
	}
}
func exec(t *testing.T, tx *sql.Tx, q string, args ...any) {
	t.Helper()
	if _, e := tx.Exec(q, args...); e != nil {
		t.Fatal(e)
	}
}

// This fixture uses the production baseline accounting triggers. Its final
// callback models the domain's one release-reference decrement; actual pin and
// publication-authority integration is verified by the domain tests separately.
func unpinFixture(t *testing.T, db *sql.DB, tracks int) mbaccounting.Change {
	t.Helper()
	writer(t, db, func(tx *sql.Tx) error {
		for _, r := range []string{"a", "b"} {
			exec(t, tx, `INSERT INTO mb_recording_evidence(revision_id,provider_id,digest,title,artist,disambiguation,observed_at,payload) VALUES(?,?,?,'T','A','','now','{}')`, r, r, strings.Repeat(r, 64))
			exec(t, tx, `UPDATE mb_recording_evidence SET sealed=1 WHERE revision_id=?`, r)
		}
		exec(t, tx, `INSERT INTO mb_release_evidence(revision_id,provider_id,digest,title,artist,release_group_id,group_title,primary_type,date,country,barcode,disambiguation,observed_at,payload) VALUES('release','release',?,'T','A','','','','','','','','now','{}')`, strings.Repeat("c", 64))
		// Accounting fixtures need only track PKs and recording memberships. Actual
		// release media uses the same production FK and ready-build definitions.
		exec(t, tx, `INSERT INTO mb_release_media(release_revision,position,title,format) VALUES('release',1,'','CD')`)
		for n := 0; n < tracks; n++ {
			rec := "a"
			if n%2 != 0 {
				rec = "b"
			}
			exec(t, tx, `INSERT INTO mb_release_tracks(release_revision,track_id,medium_position,position,number,title,artist,recording_revision) VALUES('release',?,1,?,'','T','A',?)`, fmt.Sprintf("%05d", n), n+1, rec)
		}
		exec(t, tx, `UPDATE mb_accounting_state SET phase='flip' WHERE singleton=1`)
		exec(t, tx, `UPDATE mb_release_accounting SET direct_refs=1 WHERE revision_id='release'`)
		exec(t, tx, `UPDATE mb_recording_accounting_base SET release_refs=(SELECT count(*) FROM mb_release_tracks WHERE recording_revision=revision_id)`)
		exec(t, tx, `UPDATE mb_accounting_state SET phase='idle' WHERE singleton=1`)
		return nil
	})
	return mbaccounting.Change{Owner: mbaccounting.Owner{Kind: "entity-mutation", Operation: "mutation", Generation: 1}, ID: "transition", Link: "album", Entity: 1, Digest: "pin-digest", OldRelease: "release"}
}
func state(t *testing.T, db *sql.DB) mbaccounting.State {
	t.Helper()
	var s mbaccounting.State
	writer(t, db, func(tx *sql.Tx) error { var e error; s, e = mbaccounting.ReadTx(context.Background(), tx); return e })
	return s
}
func refs(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var n int64
	if e := db.QueryRow(`SELECT sum(release_refs) FROM mb_recording_accounting`).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func prepare(t *testing.T, db *sql.DB, c mbaccounting.Change) {
	t.Helper()
	for n := 0; n < 40; n++ {
		var done bool
		writer(t, db, func(tx *sql.Tx) error {
			var e error
			done, e = mbaccounting.PrepareTx(context.Background(), tx, c)
			return e
		})
		if done {
			return
		}
	}
	t.Fatal("preparation did not converge")
}
func fold(t *testing.T, db *sql.DB) {
	t.Helper()
	for n := 0; n < 40; n++ {
		var done bool
		writer(t, db, func(tx *sql.Tx) error { var e error; done, e = mbaccounting.FoldTx(context.Background(), tx); return e })
		if done {
			return
		}
	}
	t.Fatal("fold did not converge")
}
func TestAccountingPrivateCopyFlipAndFoldHaveExactVisibleCounts(t *testing.T) {
	db := accountingDB(t)
	c := unpinFixture(t, db, 37)
	writer(t, db, func(tx *sql.Tx) error { _, e := mbaccounting.BeginTx(context.Background(), tx, c); return e })
	// More than one raw window, including duplicate recordings. Every private
	// checkpoint leaves the old authoritative values visible.
	writer(t, db, func(tx *sql.Tx) error { _, e := mbaccounting.PrepareTx(context.Background(), tx, c); return e })
	if n := refs(t, db); n != 37 {
		t.Fatal("private copy changed visible facts", n)
	}
	prepare(t, db, c)
	if n := refs(t, db); n != 37 {
		t.Fatal("ready copy changed visible facts", n)
	}
	writer(t, db, func(tx *sql.Tx) error {
		return mbaccounting.FlipTx(context.Background(), tx, c, func() error {
			_, e := tx.Exec(`UPDATE mb_release_accounting SET direct_refs=0 WHERE revision_id='release'`)
			return e
		}, func() error { return nil })
	})
	if n := refs(t, db); n != 0 {
		t.Fatal("published overlay incomplete", n)
	}
	fold(t, db)
	if n := refs(t, db); n != 0 || state(t, db).Phase != "idle" {
		t.Fatal("fold changed published facts", n)
	}
	var records int64
	if e := db.QueryRow(`SELECT recordings FROM mb_evidence_totals WHERE singleton=1`).Scan(&records); e != nil || records != 2 {
		t.Fatal(records, e)
	}
}
func TestAccountingAbortKeepsOldFactsAndReleasesOwner(t *testing.T) {
	db := accountingDB(t)
	c := unpinFixture(t, db, 37)
	writer(t, db, func(tx *sql.Tx) error { _, e := mbaccounting.BeginTx(context.Background(), tx, c); return e })
	writer(t, db, func(tx *sql.Tx) error { _, e := mbaccounting.PrepareTx(context.Background(), tx, c); return e })
	writer(t, db, func(tx *sql.Tx) error {
		if e := mbaccounting.IdleTx(context.Background(), tx); !errors.Is(e, mbaccounting.ErrBusy) {
			t.Fatal("unrelated producer was not paused", e)
		}
		return mbaccounting.AbortTx(context.Background(), tx, c.Owner)
	})
	fold(t, db)
	if n := refs(t, db); n != 37 || state(t, db).Phase != "idle" {
		t.Fatal("abort lost old authority", n)
	}
}
