package catalog

import (
	"database/sql"
	"strings"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/identity"
)

func TestHistoryBookAuthorCapturesCanonicalAuthorSeparatelyFromBookTitle(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("books", "Books", "audiobook", "/private/books")
	const title = "The Complete Book (Unabridged)"
	const author = "Renée Author & Second Writer"
	book := c.Book(lib, title, author)
	part := c.BookFile(book, 1, "/private/books/first.m4b")
	second := c.BookFile(book, 2, "/private/books/second.m4b")
	tx, err := c.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	captured, err := capturePlayHistoryBookAuthor(tx, part.ID)
	if err != nil || !captured.Valid || captured.String != author {
		t.Fatal(captured, err)
	}
	other, err := capturePlayHistoryBookAuthor(tx, second.ID)
	if err != nil || other != captured {
		t.Fatal("parts did not use their actual parent book author", other, err)
	}
	actor := identity.Viewer{Authority: "local", AccountID: "account", ProfileID: "profile"}
	if err = insertPlay(tx, actor, "actual-book-play", part.ID, int64(compactcatalog.Part), 1792753200000, 0, 120, false); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(); err != nil {
		t.Fatal(err)
	}
	var actualTitle, parent string
	if err = c.DB.QueryRow(`SELECT title,parent_title FROM play_history WHERE playback_id='actual-book-play'`).Scan(&actualTitle, &parent); err != nil || actualTitle != "Part 1" || parent != title || parent == captured.String {
		t.Fatal("book title was replaced, shortened or confused with author", actualTitle, parent, err)
	}
	c.Exec(`UPDATE catalog_books SET author='Later Author' WHERE entity_id=?`, book.ID)
	c.Exec(`UPDATE catalog_entities SET title='Later Book Title' WHERE id=?`, book.ID)
	if captured.String != author {
		t.Fatal("event-time capture changed with current metadata", captured)
	}
	if err = c.DB.QueryRow(`SELECT parent_title FROM play_history WHERE playback_id='actual-book-play'`).Scan(&parent); err != nil || parent != title {
		t.Fatal("retained book title changed with current metadata", parent, err)
	}
	// This is a new event-time capture, never a fallback for the prior row.
	fresh, err := c.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Rollback()
	next, err := capturePlayHistoryBookAuthor(fresh, part.ID)
	if err != nil || !next.Valid || next.String != "Later Author" {
		t.Fatal(next, err)
	}
}

func TestHistoryBookAuthorOmitsMissingAndUnreadableSources(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("books", "Books", "audiobook", "/private/books")
	book := c.Book(lib, "Real Book", "Actual Author")
	part := c.BookFile(book, 1, "/private/books/file.m4b")
	read := func(entityID int64) sql.NullString {
		t.Helper()
		tx, err := c.DB.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		value, err := capturePlayHistoryBookAuthor(tx, entityID)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	unknown := func(entityID int64) {
		t.Helper()
		if got := read(entityID); got.Valid {
			t.Fatal("invented an author without an eligible actual source", got)
		}
	}
	unknown(0)
	unknown(book.ID) // The accepted playback entity must be an audiobook file.
	for _, value := range []string{"", " \t", "bad\nauthor", "https://private.invalid/author?token=secret", "/private/author", `private\author`, strings.Repeat("a", 1025), string([]byte{0xff})} {
		c.Exec(`UPDATE catalog_books SET author=? WHERE entity_id=?`, value, book.ID)
		unknown(part.ID)
	}
	c.Exec(`UPDATE catalog_books SET author='Actual Author' WHERE entity_id=?`, book.ID)
	c.Exec(`UPDATE catalog_entities SET retired=1 WHERE id=?`, book.ID)
	unknown(part.ID)
	c.Exec(`UPDATE catalog_entities SET retired=0 WHERE id=?`, book.ID)
	c.Exec(`UPDATE catalog_entities SET retired=1 WHERE id=?`, part.ID)
	unknown(part.ID)
	c.Exec(`UPDATE catalog_entities SET retired=0 WHERE id=?`, part.ID)
	otherLibrary := c.Library("other-books", "Other Books", "audiobook", "/private/other")
	c.Exec(`UPDATE catalog_entities SET library_id=? WHERE id=?`, otherLibrary, part.ID)
	unknown(part.ID)
	c.Exec(`UPDATE catalog_entities SET library_id=? WHERE id=?`, lib, part.ID)
	if got := read(part.ID); !got.Valid || got.String != "Actual Author" {
		t.Fatal("valid normalized author became unknown", got)
	}
	c.Exec(`DELETE FROM catalog_books WHERE entity_id=?`, book.ID)
	unknown(part.ID)
}
