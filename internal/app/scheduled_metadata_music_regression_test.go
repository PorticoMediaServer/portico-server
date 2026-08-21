package app

import (
	"context"
	"testing"
	"time"
)

func TestScheduledMetadataSelectionIncludesChildMusicTrackWithParent(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Scheduled Music", Type: "music", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create music library: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, type, title, sort_title, genres_json, added_at)
		VALUES ('artist_scheduled_music', ?, 'artist', 'Artist', 'Artist', '[]', ?)`, library.ID, now); err != nil {
		t.Fatalf("insert artist: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, genres_json, added_at)
		VALUES ('album_scheduled_music', ?, 'artist_scheduled_music', 'album', 'Album', 'Album', '[]', ?)`, library.ID, now); err != nil {
		t.Fatalf("insert album: %v", err)
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_items (id, library_id, parent_id, type, title, sort_title, source_url, genres_json, added_at)
		VALUES ('track_scheduled_music', ?, 'album_scheduled_music', 'track', 'Track', 'Track', '/music/track.flac', '[]', ?)`, library.ID, now); err != nil {
		t.Fatalf("insert track: %v", err)
	}

	items, err := server.libraryMetadataRefreshItems(context.Background(), library.ID, map[string]string{"limit": "100"})
	if err != nil {
		t.Fatalf("select scheduled metadata candidates: %v", err)
	}
	for _, item := range items {
		if item.ID == "track_scheduled_music" {
			if item.ParentID != "album_scheduled_music" {
				t.Fatalf("selected track parent = %q", item.ParentID)
			}
			return
		}
	}
	t.Fatalf("scheduled metadata candidates omitted child music track: %#v", items)
}
