package downloads

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/identity"
)

type bookHarness struct {
	*harness
	book     catalogtest.Item
	parts    []catalogtest.Item
	personal string
}

// newBookHarness adds one audiobook with five parts to the ordinary harness.
func newBookHarness(t *testing.T) *bookHarness {
	t.Helper()
	h := newHarness(t)
	c := catalogtest.New(t, h.db)
	root := filepath.Join(h.root, "books")
	books := c.Library("books", "Books", "audiobook", root)
	b := &bookHarness{harness: h, book: c.Book(books, "Dune", "Frank Herbert"), personal: identity.PersonalKey(h.viewer.Viewer)}
	for part := 1; part <= 5; part++ {
		b.parts = append(b.parts, c.BookFile(b.book, part, filepath.Join(root, "Dune", "0"+string(rune('0'+part))+".m4b")))
	}
	settleDownloadsProjection(t, h.db)
	return b
}

// pointer reads where the server would resume the book: which part, and where
// in it.
func (b *bookHarness) pointer(t *testing.T) (int64, int64) {
	t.Helper()
	var item, position int64
	if e := b.db.QueryRow(`SELECT item_id,position FROM book_resume WHERE profile_id=? AND book_id=?`, b.personal, b.book.ID).Scan(&item, &position); e != nil {
		t.Fatal(e)
	}
	return item, position
}

// listenedOnline records what an online session leaves behind for one part: its
// progress, its activity, and the book's pointer naming it.
func (b *bookHarness) listenedOnline(t *testing.T, part catalogtest.Item, seconds int64, at time.Time) {
	t.Helper()
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`INSERT INTO progress(profile_id,item_id,position,unit,playback_id) VALUES(?,?,?,0,'online') ON CONFLICT(profile_id,item_id) DO UPDATE SET position=excluded.position,playback_id=excluded.playback_id`, []any{b.personal, part.ID, seconds * 1000}},
		{`INSERT INTO progress_activity(profile_id,library_id,item_id,updated_at,state) VALUES(?,'books',?,?,'paused') ON CONFLICT(profile_id,item_id) DO UPDATE SET updated_at=excluded.updated_at`, []any{b.personal, part.ID, at.UTC().Format(time.RFC3339)}},
		{`INSERT INTO book_resume(profile_id,book_id,item_id,playback_id,position,unit) VALUES(?,?,?,'online',?,0) ON CONFLICT(profile_id,book_id) DO UPDATE SET item_id=excluded.item_id,playback_id=excluded.playback_id,position=excluded.position`, []any{b.personal, b.book.ID, part.ID, seconds * 1000}},
	} {
		if _, e := b.db.Exec(statement.query, statement.args...); e != nil {
			t.Fatal(e)
		}
	}
}

func (b *bookHarness) sync(t *testing.T, operation string, entries ...ProgressEntry) ProgressReceipt {
	t.Helper()
	out, e := b.service.SyncProgress(context.Background(), b.viewer, operation, entries)
	if e != nil {
		t.Fatal(e)
	}
	return out
}

// Listening offline moves through a book exactly as listening online does: the
// book resumes in the part the listener was last in, not in the part they were
// in when they went offline. The order the observations arrive in does not
// matter, only when each was made.
func TestOfflineListeningMovesTheBookToTheLaterPart(t *testing.T) {
	for _, order := range [][]int{{2, 3, 4}, {4, 3, 2}} {
		b := newBookHarness(t)
		b.listenedOnline(t, b.parts[1], 120, b.now.Add(-6*time.Hour))
		var entries []ProgressEntry
		for _, index := range order {
			entries = append(entries, ProgressEntry{ItemID: b.parts[index].Public, PositionSeconds: float64(100 * index), ObservedAt: b.now.Add(time.Duration(index-5) * time.Hour).Format(time.RFC3339)})
		}
		if out := b.sync(t, "flight", entries...); out.Applied != 3 {
			t.Fatalf("order %v: %+v", order, out)
		}
		if item, position := b.pointer(t); item != b.parts[4].ID || position != 400_000 {
			t.Fatalf("order %v: the book resumes in part id %d at %d ms, want part five (%d) at 400000", order, item, position, b.parts[4].ID)
		}
	}
}

// A book nobody has listened to online has no pointer yet; listening offline
// gives it one.
func TestOfflineListeningStartsABook(t *testing.T) {
	b := newBookHarness(t)
	if out := b.sync(t, "first-listen", ProgressEntry{ItemID: b.parts[0].Public, PositionSeconds: 30, ObservedAt: b.now.Add(-time.Hour).Format(time.RFC3339)}); out.Applied != 1 {
		t.Fatalf("%+v", out)
	}
	if item, position := b.pointer(t); item != b.parts[0].ID || position != 30_000 {
		t.Fatalf("the book resumes in part id %d at %d ms", item, position)
	}
}

// The pointer is ordered the way progress is: an offline observation of one
// part does not pull the book back from a part the listener reached online
// afterwards.
func TestOfflineListeningDoesNotPullTheBookBackFromLaterOnlineListening(t *testing.T) {
	b := newBookHarness(t)
	b.listenedOnline(t, b.parts[4], 900, b.now.Add(-time.Hour))
	out := b.sync(t, "late-sync", ProgressEntry{ItemID: b.parts[2].Public, PositionSeconds: 300, ObservedAt: b.now.Add(-3 * time.Hour).Format(time.RFC3339)})
	// The part's own position is still recorded: nothing online superseded it.
	if out.Applied != 1 {
		t.Fatalf("%+v", out)
	}
	if item, position := b.pointer(t); item != b.parts[4].ID || position != 900_000 {
		t.Fatalf("an older offline observation moved the book to part id %d at %d ms", item, position)
	}
	// Listening further in the part the book already names still moves within it.
	b.sync(t, "same-part", ProgressEntry{ItemID: b.parts[4].Public, PositionSeconds: 1200, ObservedAt: b.now.Format(time.RFC3339)})
	if item, position := b.pointer(t); item != b.parts[4].ID || position != 1_200_000 {
		t.Fatalf("the book did not follow within its part: part id %d at %d ms", item, position)
	}
}

// startedOnline records exactly what starting an online session leaves behind
// before its first progress report: the session row (the schema stamps when it
// was created), and the book's pointer moved to that part and bound to that
// session at the part's stored resume point. It writes no activity, because a
// start does not; the part's activity is whatever an earlier session left, or
// nothing. The creation stamp is then placed on the harness clock, which is
// fixed, where production would have the wall clock.
func (b *bookHarness) startedOnline(t *testing.T, session string, part catalogtest.Item, resumeSeconds int64, at time.Time) {
	t.Helper()
	if _, e := b.db.Exec(`INSERT INTO playback_sessions(id,session_hash,account_id,profile_id,item_id,asset_id,generation,state,grant_hash,grant_token,expires_at,request_id,duration) VALUES(?,'login','account','profile',?,?,1,'playing',?,?,'2099-01-01T00:00:00Z',?,5400)`, session, part.ID, part.Token, "grant-hash-"+session, "grant-"+session, "request-"+session); e != nil {
		t.Fatal(e)
	}
	stamped, e := b.db.Exec(`UPDATE playback_observations SET created_at=? WHERE session_id=? AND created_at IS NOT NULL`, at.UTC().Format("2006-01-02T15:04:05.000Z"), session)
	if e != nil {
		t.Fatal(e)
	}
	if n, _ := stamped.RowsAffected(); n != 1 {
		t.Fatalf("starting a session stamped %d creation rows", n)
	}
	if _, e = b.db.Exec(`INSERT INTO book_resume(profile_id,book_id,item_id,playback_id,position,unit) VALUES(?,?,?,?,?,0) ON CONFLICT(profile_id,book_id) DO UPDATE SET item_id=excluded.item_id,playback_id=excluded.playback_id,position=excluded.position,unit=excluded.unit`, b.personal, b.book.ID, part.ID, session, resumeSeconds*1000); e != nil {
		t.Fatal(e)
	}
}

func (b *bookHarness) writer(t *testing.T) string {
	t.Helper()
	var session string
	if e := b.db.QueryRow(`SELECT playback_id FROM book_resume WHERE profile_id=? AND book_id=?`, b.personal, b.book.ID).Scan(&session); e != nil {
		t.Fatal(e)
	}
	return session
}

// An online session that has just started owns the book's pointer from the
// moment it starts, before it has reported any progress. An offline observation
// of another part made before that start must not take the book back, and must
// not unbind the session, or the session's own reports could never move the
// pointer again. The part's old activity, or its absence, does not date the
// start.
func TestOfflineListeningDoesNotTakeTheBookFromASessionThatJustStarted(t *testing.T) {
	for _, activity := range []string{"none", "from an earlier day"} {
		t.Run(activity, func(t *testing.T) {
			b := newBookHarness(t)
			if activity != "none" {
				b.listenedOnline(t, b.parts[4], 900, b.now.Add(-30*time.Hour))
			}
			b.startedOnline(t, "session-noon", b.parts[4], 900, b.now.Add(-time.Hour))
			out := b.sync(t, "late-sync", ProgressEntry{ItemID: b.parts[2].Public, PositionSeconds: 300, ObservedAt: b.now.Add(-2 * time.Hour).Format(time.RFC3339)})
			// The part's own position is still recorded.
			if out.Applied != 1 {
				t.Fatalf("%+v", out)
			}
			if item, position := b.pointer(t); item != b.parts[4].ID || position != 900_000 {
				t.Fatalf("an observation older than the session's start moved the book to part id %d at %d ms", item, position)
			}
			if session := b.writer(t); session != "session-noon" {
				t.Fatalf("the started session lost the pointer: writer %q", session)
			}
			// An observation made after the start is later evidence, and does move it.
			b.sync(t, "after-start", ProgressEntry{ItemID: b.parts[3].Public, PositionSeconds: 60, ObservedAt: b.now.Add(-30 * time.Minute).Format(time.RFC3339)})
			if item, position := b.pointer(t); item != b.parts[3].ID || position != 60_000 {
				t.Fatalf("a later observation did not move the book: part id %d at %d ms", item, position)
			}
			if session := b.writer(t); session != "" {
				t.Fatalf("the superseded session kept the pointer: writer %q", session)
			}
		})
	}
}

// The session's last report dates it too: a session started long ago that is
// still reporting is newer than an observation made in between.
func TestOfflineListeningDoesNotTakeTheBookFromASessionStillReporting(t *testing.T) {
	b := newBookHarness(t)
	b.startedOnline(t, "session-long", b.parts[4], 900, b.now.Add(-5*time.Hour))
	if _, e := b.db.Exec(`UPDATE playback_observations SET reported_at=?,position=1500 WHERE session_id='session-long'`, b.now.Add(-time.Hour).UTC().Format("2006-01-02T15:04:05.000Z")); e != nil {
		t.Fatal(e)
	}
	b.sync(t, "between", ProgressEntry{ItemID: b.parts[2].Public, PositionSeconds: 300, ObservedAt: b.now.Add(-3 * time.Hour).Format(time.RFC3339)})
	if item, _ := b.pointer(t); item != b.parts[4].ID || b.writer(t) != "session-long" {
		t.Fatalf("an observation older than the session's last report moved the book to part id %d (writer %q)", item, b.writer(t))
	}
}

// A film is not a part of a book, and gets no pointer.
func TestOfflineProgressOnAFilmWritesNoBookPointer(t *testing.T) {
	b := newBookHarness(t)
	b.sync(t, "film", ProgressEntry{ItemID: b.item, PositionSeconds: 10, ObservedAt: b.now.Format(time.RFC3339)})
	var rows int
	if e := b.db.QueryRow(`SELECT count(*) FROM book_resume`).Scan(&rows); e != nil || rows != 0 {
		t.Fatalf("book pointers %d %v", rows, e)
	}
}
