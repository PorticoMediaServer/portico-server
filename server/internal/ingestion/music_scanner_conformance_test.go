package ingestion

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"portico.local/server/internal/assets"
	"portico.local/server/internal/catalog"
	"portico.local/server/internal/catalogtest"
	"portico.local/server/internal/decodertest"
	"portico.local/server/internal/localmetadata"
	"portico.local/server/internal/naming"
	"portico.local/server/internal/persistence"
	"portico.local/server/internal/storage"
)

// The Plex music layout's exact examples, built from generated/blank audio
// fixtures. They exercise the real scanner and report the producer's current
// facts; they copy no real media, artwork or provider metadata.

func TestMusicScannerConformancePlexTheWallPackedNumbers(t *testing.T) {
	ffmpeg := decodertest.QualifiedFFmpeg(t)
	root := t.TempDir()
	albumDir := filepath.Join(root, "music", "Pink Floyd", "The Wall")
	if err := os.MkdirAll(albumDir, 0700); err != nil {
		t.Fatal(err)
	}
	discOne := []string{"In the Flesh?", "The Thin Ice", "Another Brick in the Wall, Part 1", "The Happiest Days of Our Lives", "Another Brick in the Wall, Part 2", "Mother", "Goodbye Blue Sky", "Empty Spaces", "Young Lust", "One of My Turns", "Don't Leave Me Now", "Another Brick in the Wall, Part 3", "Goodbye Cruel World"}
	discTwo := []string{"Hey You", "Is There Anybody Out There?", "Nobody Home", "Vera", "Bring the Boys Back Home", "Comfortably Numb", "The Show Must Go On", "In the Flesh", "Run Like Hell", "Waiting for the Worms", "Stop", "The Trial", "Outside the Wall"}
	var numbers []int
	titles := append(append([]string(nil), discOne...), discTwo...)
	for disc, discTitles := range [][]string{discOne, discTwo} {
		for i, title := range discTitles {
			number := (disc+1)*100 + i + 1
			numbers = append(numbers, number)
			path := filepath.Join(albumDir, fmt.Sprintf("%03d - %s.flac", number, title))
			musicConformanceFLAC(t, ffmpeg, path, "title", title, "album", "The Wall", "artist", "Pink Floyd", "album_artist", "Pink Floyd")
		}
	}

	lib, cat := musicConformanceScan(t, root)
	artist := musicConformanceArtist(t, cat, lib, "Pink Floyd")
	albums, _, err := cat.Albums(artist, "", 20)
	if err != nil || len(albums) != 1 || albums[0].Title != "The Wall" {
		t.Fatalf("The Wall album: %+v %v", albums, err)
	}
	songs, _, err := cat.AudioItems("profile", albums[0].ID, "album", "", 100)
	if err != nil || len(songs) != len(numbers) {
		t.Fatalf("The Wall tracks: %d %v", len(songs), err)
	}
	cohort, ok := naming.PackedTrackCohort(numbers, false)
	if !ok {
		t.Fatalf("file-name cohort %v is not a clear packed cohort", numbers)
	}
	for i, song := range songs {
		if song.Title != titles[i] || song.Song == nil || song.Song.AlbumTitle != "The Wall" || song.Song.Artist != "Pink Floyd" {
			t.Fatalf("track %d: %+v", i, song)
		}
		raw := numbers[i]
		packed := cohort[raw]
		switch {
		case song.Song.TrackNumber == nil:
			t.Fatalf("track %d has no number: %+v", i, song.Song)
		case *song.Song.TrackNumber == raw:
			// Caller wiring is reviewer-owned; before it the file-name number
			// stays as the track number, exactly as the Plex cohort supplies it.
		case *song.Song.TrackNumber == packed.Track && song.Song.DiscNumber != nil && *song.Song.DiscNumber == packed.Disc:
			// Wired: the packed number is disc and track.
		default:
			t.Fatalf("track %d number %d disc %v is neither raw %d nor packed %d/%d", i, *song.Song.TrackNumber, song.Song.DiscNumber, raw, packed.Disc, packed.Track)
		}
	}
}

func TestMusicScannerConformancePlexVariousArtistsCompilation(t *testing.T) {
	ffmpeg := decodertest.QualifiedFFmpeg(t)
	root := t.TempDir()
	albumDir := filepath.Join(root, "music", "Various Artists", "Guardians Of The Galaxy")
	if err := os.MkdirAll(albumDir, 0700); err != nil {
		t.Fatal(err)
	}
	tracks := []struct {
		number        int
		title, artist string
	}{
		{1, "Hooked on a Feeling", "Blue Swede"},
		{2, "Go All the Way", "The Raspberries"},
	}
	for _, track := range tracks {
		path := filepath.Join(albumDir, fmt.Sprintf("%02d - %s.flac", track.number, track.title))
		musicConformanceFLAC(t, ffmpeg, path, "title", track.title, "track", fmt.Sprint(track.number), "album", "Guardians Of The Galaxy", "artist", track.artist, "album_artist", "Various Artists")
	}

	lib, cat := musicConformanceScan(t, root)
	artists, _, err := cat.Artists(lib.ID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	var compilation string
	for _, artist := range artists {
		names[artist.Name] = true
		if artist.Name == "Various Artists" {
			compilation = artist.ID
		}
	}
	if compilation == "" {
		t.Fatalf("compilation album artist missing: %+v", artists)
	}
	for _, track := range tracks {
		if !names[track.artist] {
			t.Fatalf("per-track artist %q missing: %+v", track.artist, artists)
		}
	}
	albums, _, err := cat.Albums(compilation, "", 20)
	if err != nil || len(albums) != 1 || albums[0].Title != "Guardians Of The Galaxy" {
		t.Fatalf("compilation album: %+v %v", albums, err)
	}
	songs, _, err := cat.AudioItems("profile", albums[0].ID, "album", "", 20)
	if err != nil || len(songs) != len(tracks) {
		t.Fatalf("compilation tracks: %d %v", len(songs), err)
	}
	for i, song := range songs {
		want := tracks[i]
		if song.Title != want.title || song.Song == nil || song.Song.Artist != want.artist || song.Song.AlbumArtist != "Various Artists" || song.Song.AlbumTitle != "Guardians Of The Galaxy" || song.Song.TrackNumber == nil || *song.Song.TrackNumber != want.number {
			t.Fatalf("compilation track %d: %+v", i, song)
		}
	}
}

func musicConformanceFLAC(t *testing.T, ffmpeg, path string, tags ...string) {
	t.Helper()
	args := []string{"-nostdin", "-y", "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=0.2", "-c:a", "flac"}
	for i := 0; i+1 < len(tags); i += 2 {
		args = append(args, "-metadata", tags[i]+"="+tags[i+1])
	}
	args = append(args, path)
	if out, err := exec.Command(ffmpeg, args...).CombinedOutput(); err != nil {
		t.Fatalf("generated fixture %s: %v %s", filepath.Base(path), err, out)
	}
}

func musicConformanceScan(t *testing.T, root string) (catalog.Library, *catalog.Service) {
	t.Helper()
	ffprobe := decodertest.QualifiedFFprobe(t)
	db, err := persistence.Open(filepath.Join(root, "catalog.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	store := storage.New(binary)
	cat := catalog.New(db)
	cat.SetStorage(store)
	meta, err := localmetadata.New(filepath.Join(root, "artwork"), decodertest.QualifiedFFmpeg(t), store)
	if err != nil {
		t.Fatal(err)
	}
	scanner := New(db, cat, assets.Probe{Binary: ffprobe, Supervisor: store.Supervisor})
	scanner.SetStorage(store)
	scanner.LocalMetadata = meta
	lib, err := cat.Create("Music", "music", filepath.Join(root, "music"))
	if err != nil {
		t.Fatal(err)
	}
	job, err := scanner.Queue(lib.ID)
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 40; n++ {
		scanner.process(context.Background(), job.ID, lib.ID)
		state, err := scanner.Get(job.ID)
		if err != nil {
			t.Fatal(err)
		}
		switch state.Status {
		case "complete":
			catalogtest.New(t, db).Drain()
			return lib, cat
		case "failed", "cancelled":
			t.Fatalf("music scan %s: %+v", state.Status, state)
		}
	}
	t.Fatal("music scan did not finish")
	return catalog.Library{}, nil
}

func musicConformanceArtist(t *testing.T, cat *catalog.Service, lib catalog.Library, name string) string {
	t.Helper()
	artists, _, err := cat.Artists(lib.ID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, artist := range artists {
		if artist.Name == name {
			return artist.ID
		}
	}
	t.Fatalf("artist %q missing: %+v", name, artists)
	return ""
}
