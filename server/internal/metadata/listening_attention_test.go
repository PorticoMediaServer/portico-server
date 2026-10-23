package metadata

import (
	"context"
	"database/sql"
	"errors"
	"portico.local/server/internal/compactcatalog"
	"testing"

	"portico.local/server/internal/catalogtest"
)

// Music and audiobooks have a needs-attention list too: an album MusicBrainz
// could not settle and a book with no author are listed and counted, can be
// dismissed, and an album can be looked up again.
func TestMusicAndBooksNeedAttention(t *testing.T) {
	c := catalogtest.Open(t)
	music := c.Library("music", "Music", "music", "/music")
	books := c.Library("books", "Books", "audiobook", "/books")
	album := c.Album(c.Artist(music, "Artist"), "Release", 2020)
	c.Song(album, 1, "/music/01.mp3", "Song")
	book := c.Book(books, "Untagged", "")
	c.Exec(`INSERT INTO mb_jobs(kind,entity_id,album_id,status) VALUES('album',?,?,'unresolved')
 ON CONFLICT(kind,entity_id) DO UPDATE SET status='unresolved',generation=generation+1`, album.ID, album.ID)
	s := New(c.DB, "")
	ctx := context.Background()
	for library, want := range map[string]string{"music": "album", "books": "book"} {
		page, err := s.Attention(ctx, library, "", 25)
		if err != nil || len(page.Entries) != 1 || page.Entries[0].Kind != want || page.Total != 1 {
			t.Fatalf("%s attention: %+v %v", library, page, err)
		}
	}
	counts, err := s.AttentionCounts(ctx)
	if err != nil || counts.Total != 2 {
		t.Fatalf("counts: %+v %v", counts, err)
	}
	page, _ := s.Attention(ctx, "books", "", 25)
	entry := page.Entries[0]
	if err = s.AttentionAction(ctx, "books", AttentionCommand{Kind: entry.Kind, EntityID: entry.EntityID, ExpectedRevision: entry.Revision, Action: "dismiss"}, MBActor{}, nil); err != nil {
		t.Fatal(err)
	}
	if page, _ = s.Attention(ctx, "books", "", 25); len(page.Entries) != 0 || page.Total != 0 {
		t.Fatalf("dismissed book still listed: %+v", page)
	}
	page, _ = s.Attention(ctx, "music", "", 25)
	entry = page.Entries[0]
	if err = s.AttentionAction(ctx, "music", AttentionCommand{Kind: entry.Kind, EntityID: entry.EntityID, ExpectedRevision: entry.Revision, Action: "retry"}, MBActor{}, nil); err != nil {
		t.Fatal(err)
	}
	if page, _ = s.Attention(ctx, "music", "", 25); len(page.Entries) != 0 || page.Total != 0 {
		t.Fatalf("an album looked up again is still waiting: %+v", page)
	}
	// A book given an author leaves the list by itself.
	c.Exec(`UPDATE catalog_books SET author='' WHERE entity_id=?`, book.ID)
	c.Exec(`UPDATE catalog_books SET author='Someone' WHERE entity_id=?`, book.ID)
	var n int
	if err = c.DB.QueryRow(`SELECT count(*) FROM listening_metadata_attention WHERE entity_id=?`, book.ID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("authored book still waits: %d %v", n, err)
	}
}

// Owner attention uses its existing source policy independently of whether a
// book has an online matcher. Hidden/retired rows, stale edits and refused
// authorization never acquire a dismissal.
func TestBookAttentionOwnerPolicyRetirementAndActionFences(t *testing.T) {
	c := catalogtest.Open(t)
	library := c.Library("books", "Books", "audiobook", "/books")
	book := c.Book(library, "Untagged", "")
	service := New(c.DB, "")
	ctx := context.Background()
	read := func(want int) AttentionEntry {
		t.Helper()
		page, e := service.Attention(ctx, "books", "", 25)
		if e != nil || page.Total != int64(want) || len(page.Entries) != want {
			t.Fatalf("owner book list want%d: %+v %v", want, page, e)
		}
		counts, e := service.AttentionCounts(ctx)
		if e != nil || counts.Total != int64(want) {
			t.Fatalf("owner book counts want%d: %+v %v", want, counts, e)
		}
		if want == 0 {
			return AttentionEntry{}
		}
		entry := page.Entries[0]
		if entry.Kind != "book" || entry.EntityID != book.Public || entry.Reason != "no_author" || len(entry.Actions) != 1 || entry.Actions[0] != "dismiss" {
			t.Fatalf("manual book contract: %+v", entry)
		}
		return entry
	}
	entry := read(1)
	command := AttentionCommand{Kind: "book", EntityID: entry.EntityID, ExpectedRevision: entry.Revision + 1, Action: "dismiss"}
	if e := service.AttentionAction(ctx, "books", command, MBActor{}, nil); !errors.Is(e, ErrScreenConflict) {
		t.Fatalf("stale book dismissal: %v", e)
	}
	command.ExpectedRevision = entry.Revision
	refused := errors.New("owner authority refused")
	if e := service.AttentionAction(ctx, "books", command, MBActor{}, func(*sql.Tx) error { return refused }); !errors.Is(e, refused) {
		t.Fatalf("refused book dismissal: %v", e)
	}
	read(1)
	var kept int
	if e := c.DB.QueryRow(`SELECT count(*) FROM metadata_kept_files WHERE entity_id=?`, book.ID).Scan(&kept); e != nil || kept != 0 {
		t.Fatal("refused/stale command wrote dismissal", kept, e)
	}
	c.Exec(`UPDATE library_metadata_agents SET agent='local',revision=revision+1 WHERE library_id='books'`)
	read(0) // Keep current explicit-local source policy; broadening it is separate.
	c.Exec(`UPDATE library_metadata_agents SET agent='online',revision=revision+1 WHERE library_id='books'`)
	read(1)
	c.Write(func(ctx context.Context, tx *sql.Tx) error { return compactcatalog.RetireEntityTx(ctx, tx, book.ID) })
	read(0)
	restored := c.Book(library, "Untagged", "")
	if restored.Public != book.Public {
		t.Fatal("book fixture changed stable identity")
	}
	entry = read(1)
	c.Exec(`UPDATE catalog_libraries SET retired=1 WHERE library_id='books'`)
	read(0)
	c.Exec(`UPDATE catalog_libraries SET retired=0 WHERE library_id='books'`)
	entry = read(1)
	command.ExpectedRevision = entry.Revision
	if e := service.AttentionAction(ctx, "books", command, MBActor{}, nil); e != nil {
		t.Fatal(e)
	}
	read(0)
}
