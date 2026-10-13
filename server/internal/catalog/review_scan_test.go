package catalog

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/compactcatalog"
)

func TestAudioTitlesDoNotMergeDifferentReleases(t *testing.T) {
	for _, field := range []string{"musicbrainz_albumid", "date", "edition"} {
		t.Run(field, func(t *testing.T) {
			s, db := openEmptyAlbumDB(t)
			values := map[string][]string{
				"musicbrainz_albumid": {"11111111-1111-1111-1111-111111111111", "22222222-2222-2222-2222-222222222222"},
				"date":                {"2000", "2020"}, "edition": {"Studio", "Live"},
			}
			for i := 0; i < 2; i++ {
				tags := map[string]string{"title": "Song", "album": "Release", "album_artist": "Artist", "artist": "Artist", "disc": "1", "track": "1", "date": "2000"}
				tags[field] = values[field][i]
				commitAlbumSong(t, s, db, fmt.Sprintf("copy%d", i), fmt.Sprintf("Release%d", i), tags, false)
			}
			var songs, albums int
			if err := db.QueryRow(`SELECT count(*),count(DISTINCT album_id) FROM catalog_songs`).Scan(&songs, &albums); err != nil {
				t.Fatal(err)
			}
			if songs != 2 || albums != 2 {
				t.Fatalf("different %s merged into songs=%d albums=%d", field, songs, albums)
			}
		})
	}
}

func TestExtraOwnershipDoesNotTrustTruncatedCandidates(t *testing.T) {
	c := catalogtest.Open(t)
	c.Library("m", "Movies", "movie", "/m")
	c.Write(func(ctx context.Context, tx *sql.Tx) error {
		for n := 0; n < 513; n++ {
			path := fmt.Sprintf("/m/film%d.mkv", n)
			asset, _, err := compactcatalog.UpsertAssetTx(ctx, tx, compactcatalog.Asset{Path: path, Size: 1, ModifiedNS: 1})
			if err != nil {
				return err
			}
			title := "Alpha"
			if n == 512 {
				title = "Beta"
			}
			if _, err = AttachMovieTx(ctx, tx, MovieFile{Library: "m", Asset: asset, Path: path, Title: title, Year: 2000}); err != nil {
				return err
			}
		}
		owner, err := ResolveExtraOwnerTx(ctx, tx, "m", "/m/Beta-trailer.mkv", "/m")
		if err != nil {
			return err
		}
		if owner != 0 {
			var title string
			if err = tx.QueryRow(`SELECT title FROM catalog_entities WHERE id=?`, owner).Scan(&title); err != nil {
				return err
			}
			if title != "Beta" {
				t.Errorf("Beta trailer attached to %q after candidate truncation", title)
			}
		}
		return nil
	})
}

func TestUnicodeEquivalentAudioTagsShareArtistAndAlbum(t *testing.T) {
 s, db := openEmptyAlbumDB(t)
 for i, artist := range []string{"Beyoncé", "Beyonce\u0301"} {
  tags := map[string]string{"title":fmt.Sprintf("Song %d",i), "album":"Album", "album_artist":artist, "artist":artist, "track":fmt.Sprint(i+1), "date":"2000"}
  commitAlbumSong(t,s,db,fmt.Sprintf("track%d",i),"Album",tags,false)
 }
 var artists int
 if err := db.QueryRow(`SELECT count(*) FROM catalog_artists`).Scan(&artists); err != nil { t.Fatal(err) }
 if artists != 1 || albumCount(t,db) != 1 { t.Fatalf("Unicode-equivalent tags split artists=%d albums=%d",artists,albumCount(t,db)) }
}
