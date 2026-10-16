package metadata

import (
	"context"
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
