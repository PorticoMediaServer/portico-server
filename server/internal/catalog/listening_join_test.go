package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
)

func inventoryFile(t *testing.T, c *catalogtest.Catalog, source string, item catalogtest.Item, relative, dir string) string {
	t.Helper()
	object := "obj-" + item.Token
	c.Exec(`INSERT INTO inventory_objects(id,source_id,asset_id,root_incarnation,relative_path,revision,evidence_json,size,modified_ns,directory_path)
 VALUES(?,?,?,(SELECT incarnation FROM library_sources WHERE id=?),?,'r1','{}',1000,1,?)`, object, source, item.Token, source, relative, dir)
	return object
}

// Two albums of one MusicBrainz release are one album: the same-numbered
// tracks become versions of one track. The owner can separate one folder
// again, and identity never joins it back.
func TestAlbumsJoinByReleaseAndOwnerCanSeparateAFolder(t *testing.T) {
	c := catalogtest.Open(t)
	music := c.Library("music", "Music", "music", "/music")
	var src string
	if err := c.DB.QueryRow(`SELECT id FROM library_sources WHERE library_id='music'`).Scan(&src); err != nil {
		t.Fatal(err)
	}
	artist := c.Artist(music, "Artist")
	flac := c.Album(artist, "Release", 2020)
	mp3 := c.Album(artist, "Release (MP3)", 2020)
	var objects []string
	for n := 1; n <= 3; n++ {
		f := c.Song(flac, n, fmt.Sprintf("/music/flac/%02d.flac", n), fmt.Sprintf("Song %d", n))
		inventoryFile(t, c, src, f, fmt.Sprintf("flac/%02d.flac", n), "flac")
		m := c.Song(mp3, n, fmt.Sprintf("/music/mp3/%02d.mp3", n), fmt.Sprintf("Song %d", n))
		objects = append(objects, inventoryFile(t, c, src, m, fmt.Sprintf("mp3/%02d.mp3", n), "mp3"))
	}
	c.Drain()
	p := compactcatalog.ProviderIdentity{Provider: "musicbrainz", Kind: "release", ID: "rel-1"}
	join := func(album int64) {
		t.Helper()
		c.Write(func(ctx context.Context, tx *sql.Tx) error {
			if err := compactcatalog.RecordProviderIdentityTx(ctx, tx, album, p); err != nil {
				return err
			}
			_, err := AdoptProviderIdentityTx(ctx, tx, album, p)
			return err
		})
		drainListeningIdentityOperations(t, c)
		c.Drain()
	}
	join(mp3.ID)
	join(flac.ID)
	if n := rows(t, c, `SELECT count(*) FROM catalog_albums`); n != 1 {
		t.Fatalf("albums of one release after joining: %d", n)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_songs`); n != 3 {
		t.Fatalf("tracks after joining: %d (same-numbered tracks should be versions)", n)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_asset_links l JOIN catalog_songs s ON s.entity_id=l.entity_id`); n != 6 {
		t.Fatalf("files after joining: %d", n)
	}
	s := New(c.DB)
	separated, err := s.SeparateFolder(context.Background(), "music", objects[0], "r1", func(*sql.Tx) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_albums`); n != 2 {
		t.Fatalf("albums after separating a folder: %d", n)
	}
	id := c.ID(separated)
	if n := rows(t, c, `SELECT count(*) FROM catalog_songs s JOIN catalog_asset_links l ON l.entity_id=s.entity_id JOIN catalog_assets a ON a.id=l.asset_id WHERE s.album_id=? AND a.path LIKE '/music/mp3/%'`, id); n != 3 {
		t.Fatalf("the folder's files on the separated album: %d", n)
	}
	join(id)
	if n := rows(t, c, `SELECT count(*) FROM catalog_albums`); n != 2 {
		t.Fatalf("a separated album was joined back by identity: %d albums", n)
	}
}

// Two copies of one audiobook carrying the same ISBN are one book.
func TestBooksJoinByIdentifier(t *testing.T) {
	c := catalogtest.Open(t)
	books := c.Library("books", "Books", "audiobook", "/books")
	first := c.Book(books, "Book", "Author")
	second := c.Book(books, "Book (MP3)", "Author")
	for n := 1; n <= 2; n++ {
		c.BookFile(first, n, fmt.Sprintf("/books/a/%02d.m4b", n))
		c.BookFile(second, n, fmt.Sprintf("/books/b/%02d.mp3", n))
	}
	c.BookFile(second, 3, "/books/b/03.mp3")
	c.Drain()
	for _, b := range []int64{first.ID, second.ID} {
		c.Write(func(ctx context.Context, tx *sql.Tx) error {
			return joinBookByIdentifierTx(ctx, tx, b, map[string]string{"isbn": "978-0-00-000000-2"})
		})
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_books`); n != 1 {
		t.Fatalf("books of one ISBN after joining: %d", n)
	}
	if n := rows(t, c, `SELECT count(*) FROM catalog_book_files`); n != 3 {
		t.Fatalf("parts after joining: %d", n)
	}
	c.Drain()
	if n := rows(t, c, `SELECT count(*) FROM catalog_derivation_failures`); n != 0 {
		t.Fatalf("catalogue work failed after joining: %d", n)
	}
}

// Adoption became an actual asynchronous command in 33146dea9. Exercise its
// production adapter to terminal before submitting another identity event;
// c.Drain advances derivation, not the operations scheduler.
func drainListeningIdentityOperations(t *testing.T, c *catalogtest.Catalog) {
	t.Helper()
	adapter := New(c.DB).MutationAdapter()
	mounted := rows(t, c, `SELECT count(*) FROM catalog_entities`)
	// Each fixture entity may contribute one mutation. Use the existing
	// per-mutation writer ceiling, rather than a timing sleep or a larger plan.
	for turn := 0; turn < (mounted+1)*accountingMutationTurns; turn++ {
		var resource string
		err := c.DB.QueryRow(`SELECT resource FROM console_operations WHERE kind='catalog-mutation' AND state IN('queued','running','reconciling') ORDER BY sequence LIMIT 1`).Scan(&resource)
		if err == sql.ErrNoRows {
			if live := rows(t, c, `SELECT count(*) FROM catalog_entity_mutations WHERE status<>'terminal'`); live != 0 {
				t.Fatalf("identity command stranded %d live roots", live)
			}
			return
		}
		if err != nil {
			t.Fatal(err)
		}
		out, err := adapter.Step(context.Background(), resource)
		if err != nil {
			t.Fatal(resource, err)
		}
		if out.State == "failed" || out.State == "cancelled" || out.State == "paused" {
			t.Fatalf("identity command %s ended %s: %s", resource, out.State, out.Phase)
		}
	}
	t.Fatal("actual identity commands did not reach terminal within the fixture writer guard")
}
