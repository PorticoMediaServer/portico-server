package catalog

import (
	"testing"
)

func TestAlbumsJoinByReleaseOrTagsAcrossFolders(t *testing.T) {
	s, db := openEmptyAlbumDB(t)
	for i, folder := range []string{"Album/CD1", "Album/Disc 2 - Bonus"} {
		commitAlbumSong(t, s, db, []string{"one", "two"}[i], folder, map[string]string{"title": "Track", "album": "Album", "album_artist": "Artist", "artist": "Artist", "date": "2020"}, false)
	}
	if n := albumCount(t, db); n != 1 {
		t.Fatalf("tag grouping created %d albums", n)
	}
	for i, folder := range []string{"Other/A", "Other/B"} {
		commitAlbumSong(t, s, db, []string{"three", "four"}[i], folder, map[string]string{"title": "Track", "album": "Release", "album_artist": "Different", "artist": "Guest", "date": []string{"1990", "2022"}[i], "musicbrainz_albumid": "11111111-1111-1111-1111-111111111111"}, false)
	}
	if n := albumCount(t, db); n != 2 {
		t.Fatalf("release grouping created %d albums", n)
	}
}
func TestNaturalAudioNameAndAlbumFallback(t *testing.T) {
	if naturalAudioName("/books/Chapter 2.mp3") >= naturalAudioName("/books/Chapter 10.mp3") {
		t.Fatal("lexical number ordering")
	}
	if albumIdentityKey(nil, "Artist", "Album", 2020) != albumIdentityKey(nil, "Artist", "Album", 2020) {
		t.Fatal("unstable grouping")
	}
}
