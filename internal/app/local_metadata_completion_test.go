package app

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadNFOExtendedLocalMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "movie.nfo")
	if err := os.Mkdir(filepath.Join(dir, "actors"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "actors", "alex.jpg"), []byte("image"), 0o600); err != nil {
		t.Fatal(err)
	}
	payload := `<movie><title>Display</title><localtitle>Local</localtitle><originaltitle>Original</originaltitle><showtitle>Show</showtitle><premiered>2024-03-09</premiered><season>2</season><episode>7</episode><runtime>43</runtime><set>Saga</set><studio>Studio A</studio><studio>Studio B</studio><country>CA</country><country>US</country><ratings><rating name="tmdb"><value>8.2</value><votes>1234</votes></rating></ratings><actor><name>Alex Smith</name><role>Pilot</role><tmdbid>42</tmdbid><thumb>actors/alex.jpg</thumb></actor><actor><name>Remote</name><thumb>https://tracker.invalid/p.jpg</thumb></actor></movie>`
	if err := os.WriteFile(path, []byte(payload), 0o600); err != nil {
		t.Fatal(err)
	}
	metadata, ok := readNFO(path)
	if !ok {
		t.Fatal("extended NFO was not read")
	}
	if metadata.Title != "Local" || metadata.OriginalTitle != "Original" || metadata.ShowTitle != "Show" || metadata.ExactDate != "2024-03-09" {
		t.Fatalf("titles/date = %#v", metadata)
	}
	if metadata.SeasonNumber != 2 || metadata.EpisodeNumber != 7 || metadata.RuntimeMinutes != 43 || metadata.Collection != "Saga" {
		t.Fatalf("episode fields = %#v", metadata)
	}
	if len(metadata.Studios) != 2 || len(metadata.Countries) != 2 || metadata.RatingName != "tmdb" || metadata.RatingVotes != 1234 {
		t.Fatalf("repeated/rating fields = %#v", metadata)
	}
	if len(metadata.People) != 2 || metadata.People[0].ProviderIDs["tmdb"] != "42" || metadata.People[0].ImageURL != filepath.Join(dir, "actors", "alex.jpg") || metadata.People[1].ImageURL != "" {
		t.Fatalf("actor metadata = %#v", metadata.People)
	}
}

func TestNFOCandidatesIncludeDVDVideoTS(t *testing.T) {
	paths := nfoCandidatePaths(filepath.Join("library", "DVD", "VIDEO_TS", "VTS_01_1.VOB"))
	want := filepath.Join("library", "DVD", "VIDEO_TS", "VIDEO_TS.nfo")
	for _, path := range paths {
		if path == want {
			return
		}
	}
	t.Fatalf("VIDEO_TS candidate absent: %#v", paths)
}

func TestEmbeddedMusicExtendedTypedMetadata(t *testing.T) {
	metadata := musicMetadataFromTags(map[string]string{
		"title": "Track", "genre": "Rock", "genres": "Ambient;Electronic", "date": "2025-02-03T00:00:00Z",
		"isrc": "CAABC2500001", "barcode": "012345678905", "catalog_number": "CAT-7", "musicbrainz_workid": "work-1",
		"musicbrainz_trackid": "recording-1", "musicbrainz_releasegroupid": "group-1", "musicbrainz_artistid": "artist-1",
		"compilation": "1", "grouping": "Suite", "description": "Description", "comment": "Comment", "artistsort": "Artist, The",
	})
	if len(metadata.Genres) != 3 {
		t.Fatalf("genres = %#v", metadata.Genres)
	}
	for key, want := range map[string]string{"isrc": "CAABC2500001", "barcode": "012345678905", "catalogNumber": "CAT-7", "workID": "work-1", "recordingID": "recording-1", "releaseGroupID": "group-1", "artistID": "artist-1", "releaseDate": "2025-02-03", "compilation": "1", "grouping": "Suite", "description": "Description", "comment": "Comment", "artistSort": "Artist, The"} {
		if got := metadata.TypedMetadata[key]; got != want {
			t.Errorf("%s=%q want %q", key, got, want)
		}
	}
	if got := sanitizeTypedMetadataForType("track", normalizeTypedMetadataMap(metadata.TypedMetadata)); len(got) != len(metadata.TypedMetadata) {
		t.Fatalf("typed allowlist dropped fields: got=%#v source=%#v", got, metadata.TypedMetadata)
	}
}
