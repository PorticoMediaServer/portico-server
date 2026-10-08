package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"

	"portico.local/server/internal/compactcatalog"
	"portico.local/server/internal/dbwork"
)

// The reference catalogue: 2% films, 39% episodes across shows of 250, 59% songs
// across 12-track albums, six credits and three genres per item, a content
// rating on one item in five, and progress on one in a hundred. Every row is
// written through the catalogue write API, exactly as a scan and a metadata
// publication would, and the derived-data worker drains after each batch, so a
// batch's cost per item is the real per-item cost of cataloguing: it must stay
// flat from the first batch to the last.

type referenceShape struct {
	movies, episodes, songs, shows, albums, people int
}

func shapeOf(n int) referenceShape {
	s := referenceShape{movies: max(1, n*25/1275), episodes: n * 500 / 1275}
	s.songs = n - s.movies - s.episodes
	s.shows = max(2, s.episodes/250)
	s.albums = max(2, s.songs/12)
	s.people = max(1, n*300/1275)
	return s
}

// seedReference writes items [startItem, stopItem) in batches and reports the
// first and last batch's milliseconds per item (write plus derivation).
func seedReference(db *sql.DB, n, startItem, stopItem, batch int) (first, last float64, err error) {
	if startItem < 0 || stopItem > n || startItem > stopItem {
		return 0, 0, errors.New("invalid scale seed range")
	}
	ctx := context.Background()
	shape := shapeOf(n)
	if startItem == 0 {
		if _, err = db.Exec(`INSERT INTO libraries(id,name,kind,root) VALUES('movies','Movies','movie','/fixture/movies'),('tv','TV','tv','/fixture/tv'),('music','Music','music','/fixture/music')`); err != nil {
			return
		}
	}
	worker := compactcatalog.NewWorker(db)
	for offset := startItem; offset < stopItem; offset += batch {
		start := time.Now()
		end := min(stopItem, offset+batch)
		err = dbwork.WithWriteTx(ctx, db, dbwork.ClassBackgroundMedia, func(tx *sql.Tx) error {
			for i := offset; i < end; i++ {
				if e := seedItem(ctx, tx, shape, i); e != nil {
					return fmt.Errorf("item %d: %w", i, e)
				}
			}
			return nil
		})
		if err != nil {
			return
		}
		for {
			var done int
			if done, err = worker.Step(ctx, 500); err != nil {
				return
			}
			if done == 0 {
				break
			}
		}
		last = float64(time.Since(start)) / float64(time.Millisecond) / float64(end-offset)
		if offset == startItem {
			first = last
		}
		if end%10000 < batch || end == n {
			fmt.Fprintf(os.Stderr, "seed %d/%d latest %.3f ms/item\n", end, n, last)
		}
	}
	return
}

// seedItem writes item i and, the first time one is needed, its container.
func seedItem(ctx context.Context, tx *sql.Tx, s referenceShape, i int) error {
	library, kind := "movies", compactcatalog.Movie
	if i >= s.movies {
		library, kind = "tv", compactcatalog.Episode
	}
	if i >= s.movies+s.episodes {
		library, kind = "music", compactcatalog.Track
	}
	handle, err := compactcatalog.LibraryTx(ctx, tx, library)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/fixture/%s/item%09d.mp4", library, i)
	var parent int64
	facts := map[string]any{}
	switch kind {
	case compactcatalog.Episode:
		num := i - s.movies
		show, number := 0, num+1
		if num >= 1100 {
			show = 1 + (num-1100)%(s.shows-1)
			number = 1 + (num-1100)/(s.shows-1)
		}
		showID, seasonID, err := seedShow(ctx, tx, handle, show)
		if err != nil {
			return err
		}
		parent = seasonID
		facts = map[string]any{"show_id": showID, "season_id": seasonID, "numbering": "seasonal", "number": number}
	case compactcatalog.Track:
		num := i - s.movies - s.episodes
		album := min(s.albums-1, num/12)
		artist := album/5 + 1
		if album < 417 {
			artist = 0
		}
		albumID, _, err := seedAlbum(ctx, tx, handle, album, artist)
		if err != nil {
			return err
		}
		parent = albumID
		facts = map[string]any{"album_id": albumID, "disc_number": 1, "track_number": num%12 + 1}
	}
	item, _, err := compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: handle, Kind: kind, Parent: parent, Key: compactcatalog.ItemKey("/fixture/"+library, path, 0),
		Title: fmt.Sprintf("Title %09d", i), Year: 1950 + i%76, Added: time.Date(2026, 1, 1, 0, 0, i%86400, 0, time.UTC).Format("2006-01-02T15:04:05.000Z")})
	if err != nil {
		return err
	}
	if len(facts) > 0 {
		if err = compactcatalog.SetFactsTx(ctx, tx, item, facts); err != nil {
			return err
		}
	}
	if kind == compactcatalog.Track {
		var artist int64
		if err = tx.QueryRowContext(ctx, `SELECT artist_id FROM catalog_albums WHERE entity_id=?`, parent).Scan(&artist); err != nil {
			return err
		}
		if err = compactcatalog.SetSongArtistsTx(ctx, tx, item, []int64{artist}); err != nil {
			return err
		}
	}
	asset, _, err := compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{Path: path, Size: 1048576, ModifiedNS: 1, Container: "mp4", VideoCodec: "h264", AudioCodec: "aac", Width: 1920, Height: 1080, Duration: 3600})
	if err != nil {
		return err
	}
	if err = compactcatalog.LinkAssetTx(ctx, tx, item, asset, compactcatalog.Link{}); err != nil {
		return err
	}
	if i%10 < 2 {
		rating := "G"
		if i%10 == 1 {
			rating = "R"
		}
		if err = compactcatalog.SetAttributesTx(ctx, tx, item, "contentRating", []string{rating}); err != nil {
			return err
		}
	}
	credits := make([]compactcatalog.Credit, 6)
	for c := range credits {
		person := fmt.Sprint("person", (i*3+c)%s.people)
		credits[c] = compactcatalog.Credit{PersonKey: "fixture:" + person, PersonName: "Person " + person, ProviderPersonID: person, CreditID: person, CreditedName: "Person " + person, Role: "Actor", Department: "Acting", Ordinal: c}
	}
	if err = compactcatalog.SetCreditsTx(ctx, tx, item, "fixture", credits); err != nil {
		return err
	}
	genres := make([]compactcatalog.Term, 3)
	for g := range genres {
		genre := fmt.Sprint((i + g) % 5000)
		genres[g] = compactcatalog.Term{SourceID: genre, Name: "Genre " + genre}
	}
	if err = compactcatalog.SetTermsTx(ctx, tx, item, compactcatalog.VocabGenre, "fixture", genres); err != nil {
		return err
	}
	if i%100 == 0 {
		if _, err = tx.ExecContext(ctx, `INSERT INTO progress(profile_id,item_id,position,unit,playback_id) VALUES('viewer',?,120000,0,'fixture')`, item); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO progress_activity(profile_id,library_id,item_id,updated_at,state) VALUES('viewer',?,?,'2026-09-22T12:00:00Z','paused')`, library, item); err != nil {
			return err
		}
	}
	return nil
}

func seedShow(ctx context.Context, tx *sql.Tx, library int64, n int) (int64, int64, error) {
	key := fmt.Sprint(n)
	show, created, err := compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Show, Key: compactcatalog.ShowKey(key), Title: fmt.Sprintf("Show %06d", n)})
	if err != nil {
		return 0, 0, err
	}
	if created {
		if err = compactcatalog.SetFactsTx(ctx, tx, show, map[string]any{"local_key": key}); err != nil {
			return 0, 0, err
		}
	}
	season, created, err := compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Season, Parent: show, Key: compactcatalog.SeasonKey(key, 1), Title: "Season 1"})
	if err != nil {
		return 0, 0, err
	}
	if created {
		err = compactcatalog.SetFactsTx(ctx, tx, season, map[string]any{"show_id": show, "number": 1})
	}
	return show, season, err
}

func seedAlbum(ctx context.Context, tx *sql.Tx, library int64, album, artist int) (int64, int64, error) {
	artistKey := fmt.Sprint(artist)
	artistID, created, err := compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Artist, Key: compactcatalog.ArtistKey(artistKey), Title: fmt.Sprintf("Artist %06d", artist)})
	if err != nil {
		return 0, 0, err
	}
	if created {
		if err = compactcatalog.SetFactsTx(ctx, tx, artistID, map[string]any{"local_key": artistKey}); err != nil {
			return 0, 0, err
		}
	}
	key := fmt.Sprint(album)
	albumID, created, err := compactcatalog.EnsureEntityTx(ctx, tx, compactcatalog.Entity{Library: library, Kind: compactcatalog.Album, Parent: artistID, Key: compactcatalog.AlbumKey(key), Title: fmt.Sprintf("Album %06d", album), Year: 2000})
	if err != nil {
		return 0, 0, err
	}
	if created {
		err = compactcatalog.SetFactsTx(ctx, tx, albumID, map[string]any{"artist_id": artistID, "local_key": key})
	}
	return albumID, artistID, err
}
