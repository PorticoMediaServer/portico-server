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
