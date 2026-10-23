package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"reflect"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
	"portico.local/server/internal/identity"
)

func TestSearchQueueCapturesEverySongInSearchOrder(t *testing.T) {
	c := catalogtest.Open(t)
	music := c.Library("music", "Music", "music", "/music")
	artist := c.Artist(music, "Artist")
	album := c.Album(artist, "Album", 2025)
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		for i := 0; i < 1203; i++ {
			path := fmt.Sprintf("/music/%04d.flac", i)
			id, _, err := compactcatalog.UpsertEntityTx(ctx, tx, compactcatalog.Entity{Library: music, Kind: compactcatalog.Track, Parent: album.ID, Key: compactcatalog.ItemKey("/music", path, 0), Title: fmt.Sprintf("Harbor %04d", i)})
			if err != nil {
				return err
			}
			if err = compactcatalog.SetFactsTx(ctx, tx, id, map[string]any{"album_id": album.ID, "track_number": i + 1}); err != nil {
				return err
			}
			if err = compactcatalog.SetSongArtistsTx(ctx, tx, id, []int64{artist.ID}); err != nil {
				return err
			}
		}
		return nil
	})
	restricted := c.Song(album, 1204, "/music/private.flac", "Harbor private")
	c.Attributes(restricted.ID, "label", "Adult")
	c.Drain()
	s := New(c.DB)
	v := Viewer{Profile: "mine", Fence: "current", Libraries: []string{"music"}, Restrictions: identity.ContentRestrictions{BlockedLabels: []string{"Adult"}}}
	for _, sort := range []string{"relevance", "title", "releaseYear", "dateAdded"} {
		t.Run(sort, func(t *testing.T) {
			q := SearchRequest{Viewer: v, Q: "Harbor", Group: "songs", Sort: sort, Direction: "asc", Limit: 37}
			want := []string{}
			for {
				page, err := s.Search(context.Background(), q)
				if err != nil || page.Groups[0].Status != "success" {
					t.Fatal("Search", page.Groups, err)
				}
				for _, e := range page.Groups[0].Items {
					want = append(want, e.ID)
				}
				q.Cursor = page.Groups[0].NextCursor
				if q.Cursor == "" {
					break
				}
			}
			tx, close, err := dbwork.BeginRead(context.Background(), c.DB)
			if err != nil {
				t.Fatal(err)
			}
			defer close()
			plan, err := s.SearchQueueKeysTx(context.Background(), tx, v, SearchQueueQuery{Q: "Harbor", Group: "songs", Sort: sort, Direction: "asc"})
			if err != nil {
				t.Fatal(err)
			}
			rows, err := tx.Query(plan.SQL, plan.Args...)
			if err != nil {
				t.Fatal(err)
			}
			got := []string{}
			for rows.Next() {
				var id string
				if err = rows.Scan(&id); err != nil {
					break
				}
				if id == restricted.Public {
					t.Fatal("restricted queue source")
				}
				got = append(got, id)
			}
			if e := rows.Err(); err == nil {
				err = e
			}
			rows.Close()
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 1203 || !reflect.DeepEqual(got, want) {
				t.Fatalf("Search queue differs from 1,203 actual results: got %d want %d", len(got), len(want))
			}
		})
	}
}

func TestSearchQueueFallbackTiesMatchPublicSearchBothDirections(t *testing.T) {
	for _, scenario := range []struct {
		name, q   string
		libraries int
	}{{"CJK", "東京", 1}, {"long-word", "abcdefghijklmnop", 1}, {"wide-authority", "Harbor", 65}} {
		t.Run(scenario.name, func(t *testing.T) {
			c := catalogtest.Open(t)
			lib := c.Library("music", "Music", "music", "/music")
			album := c.Album(c.Artist(lib, "Artist"), "Album", 2026)
			a := c.Song(album, 1, "/music/a.flac", scenario.q)
			b := c.Song(album, 2, "/music/b.flac", scenario.q)
			// Deterministically invert public and internal ordering, independent of RNG.
			c.Exec(`UPDATE catalog_identities SET public_id=X'22222222222222222222222222222222' WHERE public_id=(SELECT public_id FROM catalog_entities WHERE id=?)`, a.ID)
			c.Exec(`UPDATE catalog_entities SET public_id=X'22222222222222222222222222222222' WHERE id=?`, a.ID)
			c.Exec(`UPDATE catalog_identities SET public_id=X'11111111111111111111111111111111' WHERE public_id=(SELECT public_id FROM catalog_entities WHERE id=?)`, b.ID)
			c.Exec(`UPDATE catalog_entities SET public_id=X'11111111111111111111111111111111' WHERE id=?`, b.ID)
			libs := []string{"music"}
			for i := 1; i < scenario.libraries; i++ {
				id := fmt.Sprintf("lib-%02d", i)
				c.Library(id, id, "music", "/"+id)
				libs = append(libs, id)
			}
			c.Drain()
			c.Exec(`UPDATE catalog_search_documents SET entity_pid=pid((SELECT public_id FROM catalog_entities WHERE id=entity_id)) WHERE entity_id IN(?,?)`, a.ID, b.ID)
			s := New(c.DB)
			v := Viewer{Profile: "p", Fence: "f", Libraries: libs}
			for _, direction := range []string{"asc", "desc"} {
				req := SearchRequest{Viewer: v, Q: scenario.q, Group: "songs", Sort: "relevance", Direction: direction, Limit: 1}
				want := []string{}
				for {
					page, err := s.Search(context.Background(), req)
					if err != nil || len(page.Groups) != 1 || page.Groups[0].Status != "success" {
						t.Fatal(page, err)
					}
					for _, e := range page.Groups[0].Items {
						want = append(want, e.ID)
					}
					req.Cursor = page.Groups[0].NextCursor
					if req.Cursor == "" {
						break
					}
				}
				tx, done, err := dbwork.BeginRead(context.Background(), c.DB)
				if err != nil {
					t.Fatal(err)
				}
				plan, err := s.SearchQueueKeysTx(context.Background(), tx, v, SearchQueueQuery{Q: scenario.q, Group: "songs", Sort: "relevance", Direction: direction})
				if err != nil {
					done()
					t.Fatal(err)
				}
				rows, err := tx.Query(plan.SQL, plan.Args...)
				if err != nil {
					done()
					t.Fatal(err)
				}
				got := []string{}
				for rows.Next() {
					var id string
					if err = rows.Scan(&id); err != nil {
						t.Fatal(err)
					}
					got = append(got, id)
				}
				err = rows.Err()
				rows.Close()
				done()
				if err != nil || len(want) != 2 || !reflect.DeepEqual(want, got) {
					t.Fatalf("%s public=%v queue=%v err=%v", direction, want, got, err)
				}
				expected := c.Public(b.ID)
				if direction == "desc" {
					expected = c.Public(a.ID)
				}
				if want[0] != expected {
					t.Fatal("fixture failed to exercise opposite tie", want)
				}
			}
		})
	}
}

func TestSearchQueueRevisionBindsSelectionAuthority(t *testing.T) {
	c := catalogtest.Open(t)
	lib := c.Library("music", "Music", "music", "/music")
	album := c.Album(c.Artist(lib, "Artist"), "Album", 2026)
	c.Song(album, 1, "/music/a.flac", "Harbor")
	c.Library("other", "Other", "music", "/other")
	c.Drain()
	s := New(c.DB)
	tx, done, err := dbwork.BeginRead(context.Background(), c.DB)
	if err != nil {
		t.Fatal(err)
	}
	defer done()
	base := Viewer{Profile: "p", Fence: "f", Libraries: []string{"music", "other"}}
	revision := func(v Viewer, q SearchQueueQuery) string {
		t.Helper()
		plan, e := s.SearchQueueKeysTx(context.Background(), tx, v, q)
		if e != nil {
			t.Fatal(e)
		}
		return plan.Revision
	}
	q := SearchQueueQuery{Q: "Harbor", Group: "songs"}
	original := revision(base, q)
	reordered := base
	reordered.Libraries = []string{"other", "music"}
	if revision(reordered, q) != original {
		t.Fatal("order-only authority changed")
	}
	changed := base
	changed.Fence = "new"
	if revision(changed, q) == original {
		t.Fatal("missing current authority fence")
	}
	changed = base
	changed.Restrictions = identity.ContentRestrictions{BlockedLabels: []string{"Adult"}}
	if revision(changed, q) == original {
		t.Fatal("missing effective restrictions")
	}
	changed = base
	changed.Libraries = []string{"music"}
	if revision(changed, q) == original {
		t.Fatal("missing selected scope")
	}
	q.Direction = "asc"
	if revision(base, q) == original {
		t.Fatal("missing effective sort direction")
	}
}
