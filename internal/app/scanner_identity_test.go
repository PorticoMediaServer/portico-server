package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOpaquePublicResourceIDGenerationIsCollisionResistant(t *testing.T) {
	const sampleSize = 4096
	seen := make(map[string]struct{}, sampleSize)
	for range sampleSize {
		id := randomOpaquePublicID()
		if !isOpaquePublicResourceID(id) {
			t.Fatalf("generated public ID %q is not an opaque 160-bit identifier", id)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("generated duplicate public ID %q in a %d-ID sample", id, sampleSize)
		}
		seen[id] = struct{}{}
	}
}

func TestScannerTVHierarchyUsesOpaqueIDsAndSurvivesRename(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	showDir := filepath.Join(root, "Legacy Show", "Season 01")
	if err := os.MkdirAll(showDir, 0o700); err != nil {
		t.Fatalf("create show hierarchy: %v", err)
	}
	episodePath := filepath.Join(showDir, "Legacy.Show.S01E01.Pilot.mkv")
	if err := os.WriteFile(episodePath, []byte("stable episode payload"), 0o600); err != nil {
		t.Fatalf("write episode: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Shows", Type: "show", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("initial scan: %v", err)
	}

	episodeID, seasonID, showID := loadEpisodeHierarchyIDs(t, server, library.ID)
	for label, id := range map[string]string{"episode": episodeID, "season": seasonID, "show": showID} {
		if !isOpaquePublicResourceID(id) {
			t.Fatalf("%s ID %q is not an opaque 160-bit Portico media ID", label, id)
		}
	}

	renamedShowDir := filepath.Join(root, "Renamed Show")
	if err := os.Rename(filepath.Join(root, "Legacy Show"), renamedShowDir); err != nil {
		t.Fatalf("rename show hierarchy: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan after hierarchy rename: %v", err)
	}
	renamedEpisodeID, renamedSeasonID, renamedShowID := loadEpisodeHierarchyIDs(t, server, library.ID)
	if episodeID != renamedEpisodeID || seasonID != renamedSeasonID || showID != renamedShowID {
		t.Fatalf("TV hierarchy IDs changed across rename: episode %q -> %q, season %q -> %q, show %q -> %q", episodeID, renamedEpisodeID, seasonID, renamedSeasonID, showID, renamedShowID)
	}
}

func TestScannerMusicHierarchyUsesOpaqueIDsAndSurvivesMove(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	albumDir := filepath.Join(root, "Old Artist", "Old Album")
	if err := os.MkdirAll(albumDir, 0o700); err != nil {
		t.Fatalf("create music hierarchy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(albumDir, "01 - Opening.flac"), []byte("stable track payload"), 0o600); err != nil {
		t.Fatalf("write track: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Music", Type: "music", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("initial scan: %v", err)
	}

	trackID, albumID, artistID := loadTrackHierarchyIDs(t, server, library.ID)
	for label, id := range map[string]string{"track": trackID, "album": albumID, "artist": artistID} {
		if !isOpaquePublicResourceID(id) {
			t.Fatalf("%s ID %q is not an opaque 160-bit Portico media ID", label, id)
		}
	}

	renamedDir := filepath.Join(root, "New Artist", "New Album")
	if err := os.MkdirAll(filepath.Dir(renamedDir), 0o700); err != nil {
		t.Fatalf("create renamed artist: %v", err)
	}
	if err := os.Rename(albumDir, renamedDir); err != nil {
		t.Fatalf("move album: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan after album move: %v", err)
	}
	renamedTrackID, renamedAlbumID, renamedArtistID := loadTrackHierarchyIDs(t, server, library.ID)
	if trackID != renamedTrackID || albumID != renamedAlbumID || artistID != renamedArtistID {
		t.Fatalf("music hierarchy IDs changed across move: track %q -> %q, album %q -> %q, artist %q -> %q", trackID, renamedTrackID, albumID, renamedAlbumID, artistID, renamedArtistID)
	}
}

func TestScannerAudiobookIdentitySurvivesFolderRename(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	bookDir := filepath.Join(root, "Author", "Original Book")
	if err := os.MkdirAll(bookDir, 0o700); err != nil {
		t.Fatalf("create audiobook hierarchy: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bookDir, "01 - Chapter One.m4b"), []byte("stable audiobook payload"), 0o600); err != nil {
		t.Fatalf("write audiobook: %v", err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Audiobooks", Type: "audiobook", Paths: []string{root}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("initial scan: %v", err)
	}
	items, err := server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'audiobook'`, []any{library.ID})
	if err != nil || len(items) != 1 {
		t.Fatalf("initial audiobook items=%#v err=%v", items, err)
	}
	originalID := items[0].ID
	if !isOpaquePublicResourceID(originalID) {
		t.Fatalf("audiobook ID %q is not an opaque 160-bit Portico media ID", originalID)
	}
	if err := os.Rename(bookDir, filepath.Join(root, "Author", "Renamed Book")); err != nil {
		t.Fatalf("rename audiobook folder: %v", err)
	}
	if _, err := server.performLibraryScan(library, ""); err != nil {
		t.Fatalf("scan after audiobook rename: %v", err)
	}
	items, err = server.queryMedia("", `WHERE m.library_id = ? AND m.type = 'audiobook'`, []any{library.ID})
	if err != nil || len(items) != 1 {
		t.Fatalf("renamed audiobook items=%#v err=%v", items, err)
	}
	if items[0].ID != originalID {
		t.Fatalf("audiobook ID changed across rename: %q -> %q", originalID, items[0].ID)
	}
}

func TestScannerParentProviderIdentityRequiresUniqueEvidence(t *testing.T) {
	server := newScannerTestServer(t)
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Provider Shows", Type: "show", Paths: []string{t.TempDir()}})
	if err != nil {
		t.Fatalf("create library: %v", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, id := range []string{"med_provider_a", "med_provider_b"} {
		if _, err := server.db.Exec(`
			INSERT INTO media_items (id, library_id, type, title, sort_title, added_at, random_key)
			VALUES (?, ?, 'show', ?, ?, ?, ?)`, id, library.ID, id, id, now, id); err != nil {
			t.Fatalf("insert provider candidate %s: %v", id, err)
		}
	}
	if _, err := server.db.Exec(`
		INSERT INTO media_provider_ids (media_id, provider, external_id, external_type, confidence, source, updated_at)
		VALUES ('med_provider_a', 'tmdb', '1234', 'tv', 1, 'test', ?)`, now); err != nil {
		t.Fatalf("insert unique provider identity: %v", err)
	}

	tx, err := server.db.Begin()
	if err != nil {
		t.Fatalf("begin unique resolution: %v", err)
	}
	resolved, err := resolveScannedParentIdentity(tx, library.ID, "", "show", "renamed-show", "/shows/renamed", scannerLocalMetadata{ProviderIDs: map[string]string{"tmdb": "1234"}}, now)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("resolve unique provider identity: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit unique resolution: %v", err)
	}
	if resolved != "med_provider_a" {
		t.Fatalf("unique provider evidence resolved to %q, expected med_provider_a", resolved)
	}

	if _, err := server.db.Exec(`
		INSERT INTO media_provider_ids (media_id, provider, external_id, external_type, confidence, source, updated_at)
		VALUES ('med_provider_b', 'tmdb', '1234', 'tv', 1, 'test', ?)`, now); err != nil {
		t.Fatalf("insert ambiguous provider identity: %v", err)
	}
	tx, err = server.db.Begin()
	if err != nil {
		t.Fatalf("begin ambiguous resolution: %v", err)
	}
	subjectID, err := resolveScannedParentIdentity(tx, library.ID, "", "show", "ambiguous-show", "/shows/ambiguous", scannerLocalMetadata{ProviderIDs: map[string]string{"tmdb": "1234"}}, now)
	if err != nil {
		_ = tx.Rollback()
		t.Fatalf("resolve ambiguous provider identity: %v", err)
	}
	if subjectID == "med_provider_a" || subjectID == "med_provider_b" || !isOpaquePublicResourceID(subjectID) {
		_ = tx.Rollback()
		t.Fatalf("ambiguous provider identity silently merged into %q", subjectID)
	}
	if err := upsertParentRow(tx, subjectID, library.ID, "", "show", "Ambiguous Show", "", 0, now); err != nil {
		_ = tx.Rollback()
		t.Fatalf("insert ambiguous subject: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit ambiguous resolution: %v", err)
	}
	var reviewSubject, candidates string
	if err := server.db.QueryRow(`
		SELECT subject_id, candidate_ids_json
		FROM identity_reconciliation_reviews
		WHERE domain = 'media_parent' AND status = 'open'
		ORDER BY created_at DESC, id DESC LIMIT 1`).Scan(&reviewSubject, &candidates); err != nil {
		t.Fatalf("load parent reconciliation review: %v", err)
	}
	if reviewSubject != subjectID || !strings.Contains(candidates, "med_provider_a") || !strings.Contains(candidates, "med_provider_b") {
		t.Fatalf("parent review subject=%q candidates=%s", reviewSubject, candidates)
	}
}

func loadEpisodeHierarchyIDs(t *testing.T, server *Server, libraryID string) (string, string, string) {
	t.Helper()
	var episodeID, seasonID, showID string
	if err := server.db.QueryRow(`
		SELECT episode.id, season.id, show.id
		FROM media_items episode
		JOIN media_items season ON season.id = episode.parent_id AND season.type = 'season'
		JOIN media_items show ON show.id = season.parent_id AND show.type = 'show'
		WHERE episode.library_id = ? AND episode.type = 'episode'`, libraryID).Scan(&episodeID, &seasonID, &showID); err != nil {
		t.Fatalf("load episode hierarchy: %v", err)
	}
	return episodeID, seasonID, showID
}

func loadTrackHierarchyIDs(t *testing.T, server *Server, libraryID string) (string, string, string) {
	t.Helper()
	var trackID, albumID, artistID string
	if err := server.db.QueryRow(`
		SELECT track.id, album.id, artist.id
		FROM media_items track
		JOIN media_items album ON album.id = track.parent_id AND album.type = 'album'
		JOIN media_items artist ON artist.id = album.parent_id AND artist.type = 'artist'
		WHERE track.library_id = ? AND track.type = 'track'`, libraryID).Scan(&trackID, &albumID, &artistID); err != nil {
		t.Fatalf("load track hierarchy: %v", err)
	}
	return trackID, albumID, artistID
}
