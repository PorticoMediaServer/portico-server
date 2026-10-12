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

// A film is not a part of a book, and gets no pointer.
func TestOfflineProgressOnAFilmWritesNoBookPointer(t *testing.T) {
	b := newBookHarness(t)
	b.sync(t, "film", ProgressEntry{ItemID: b.item, PositionSeconds: 10, ObservedAt: b.now.Format(time.RFC3339)})
	var rows int
	if e := b.db.QueryRow(`SELECT count(*) FROM book_resume`).Scan(&rows); e != nil || rows != 0 {
		t.Fatalf("book pointers %d %v", rows, e)
	}
}
