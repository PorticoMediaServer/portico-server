package playback

import (
	"database/sql"
	"path/filepath"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/identity"
	"portico.local/server/internal/persistence"
	"testing"
)

func personalWriterFixture(t *testing.T) (*sql.DB, identity.Principal, string, string, int64, int64) {
	t.Helper()
	db, err := persistence.Open(filepath.Join(t.TempDir(), "writers.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	idA, itemA, _ := catalogFixture(t, db, "library", "/movies", compactcatalog.Movie, "a", "A", compactcatalog.Asset{Path: "/movies/a"})
	idB, itemB, _ := catalogFixture(t, db, "library", "/movies", compactcatalog.Movie, "b", "B", compactcatalog.Asset{Path: "/movies/b"})
	return db, identity.Principal{Hash: "token", Viewer: identity.Viewer{Authority: "local", AccountID: "account", ProfileID: "profile", Role: "owner"}}, itemA, itemB, idA, idB
}
func writerTx(t *testing.T, db *sql.DB, work func(*sql.Tx) error) {
	t.Helper()
	tx, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	if err = work(tx); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
}
func expectWriter(t *testing.T, db *sql.DB, p identity.Principal, id int64, want string) {
	t.Helper()
	var actual string
	if err := db.QueryRow(`SELECT playback_id FROM progress WHERE profile_id=? AND item_id=?`, identity.PersonalKey(p.Viewer), id).Scan(&actual); err != nil || actual != want {
		t.Fatalf("writer %d=%q, want %q: %v", id, actual, want, err)
	}
}

func TestPersonalWriterUsesAcceptanceNotPreparationOrder(t *testing.T) {
	db, p, itemA, _, idA, _ := personalWriterFixture(t)
	var first, second int64
	writerTx(t, db, func(tx *sql.Tx) (err error) { first, err = reservePersonalIntent(tx, "first"); return })
	writerTx(t, db, func(tx *sql.Tx) (err error) {
		second, err = reservePersonalIntent(tx, "second")
		if err != nil {
			return
		}
		return activatePersonalWriter(tx, p, itemA, "fast", 0, second)
	})
	writerTx(t, db, func(tx *sql.Tx) error { return activatePersonalWriter(tx, p, itemA, "slow", 0, first) })
	expectWriter(t, db, p, idA, "fast")
	writerTx(t, db, func(tx *sql.Tx) error {
		again, err := reservePersonalIntent(tx, "first")
		if err == nil && again != first {
			t.Fatal("retry elected a new writer")
		}
		return err
	})
	var retained int
	if err := db.QueryRow(`SELECT count(*) FROM playback_personal_claims`).Scan(&retained); err != nil || retained != 2 {
		t.Fatal("older occurrence attribution lost", retained, err)
	}
}
func TestQueueContinuationInheritsWriterAndCannotOvertakeNewExplicitPlay(t *testing.T) {
	db, p, itemA, itemB, idA, idB := personalWriterFixture(t)
	var root, newer int64
	writerTx(t, db, func(tx *sql.Tx) (err error) {
		root, err = reservePersonalIntent(tx, "queue-root")
		if err != nil {
			return
		}
		return activatePersonalWriter(tx, p, itemA, "old-a", 0, root)
	})
	writerTx(t, db, func(tx *sql.Tx) error {
		ordinal, err := inheritedPersonalIntent(tx, p, "old-a")
		if err != nil {
			return err
		}
		return activatePersonalWriter(tx, p, itemA, "repeat-a", 0, ordinal)
	})
	expectWriter(t, db, p, idA, "repeat-a")
	writerTx(t, db, func(tx *sql.Tx) (err error) {
		newer, err = reservePersonalIntent(tx, "other-device")
		if err != nil {
			return
		}
		return activatePersonalWriter(tx, p, itemB, "explicit-b", 0, newer)
	})
	writerTx(t, db, func(tx *sql.Tx) error { return activatePersonalWriter(tx, p, itemB, "auto-b", 0, root) })
	expectWriter(t, db, p, idB, "explicit-b")
	other := p
	other.Authority = "hosted"
	writerTx(t, db, func(tx *sql.Tx) error { return activatePersonalWriter(tx, other, itemB, "hosted-b", 0, root) })
	expectWriter(t, db, other, idB, "hosted-b")
	expectWriter(t, db, p, idB, "explicit-b")
}
func TestManualFenceCoversPendingAndAutomaticButAllowsLaterExplicit(t *testing.T) {
	db, p, itemA, _, idA, _ := personalWriterFixture(t)
	var pending int64
	writerTx(t, db, func(tx *sql.Tx) (err error) {
		pending, err = reservePersonalIntent(tx, "pending")
		if err != nil {
			return
		}
		if _, err = tx.Exec(`INSERT INTO progress(profile_id,item_id,position,unit,completed,playback_id) VALUES(?,?,25000,0,0,'manual')`, identity.PersonalKey(p.Viewer), idA); err != nil {
			return
		}
		_, err = tx.Exec(`INSERT INTO playback_personal_fences VALUES(?,?,?)`, identity.PersonalKey(p.Viewer), idA, pending)
		return
	})
	writerTx(t, db, func(tx *sql.Tx) error { return activatePersonalWriter(tx, p, itemA, "delayed", 0, pending) })
	expectWriter(t, db, p, idA, "manual")
	writerTx(t, db, func(tx *sql.Tx) error {
		ordinal, err := reservePersonalIntent(tx, "deliberate-new-play")
		if err != nil {
			return err
		}
		return activatePersonalWriter(tx, p, itemA, "new", 25, ordinal)
	})
	expectWriter(t, db, p, idA, "new")
}
