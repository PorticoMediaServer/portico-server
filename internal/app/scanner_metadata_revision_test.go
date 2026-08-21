package app

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScannedMetadataRevisionRespectsLocksAndStampsProjections(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Revision", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	mediaID := randomOpaqueMediaID()
	file := scannerMediaFile{
		ID: mediaID, LibraryID: library.ID, Type: "movie", Title: "Path Title", SortTitle: "Path Title",
		SourcePath: filepath.Join(root, "Path Title.mkv"),
		LocalMetadata: scannerLocalMetadata{
			Title: "NFO Title", Summary: "NFO summary", Source: "nfo",
			Genres:      []string{"Drama"},
			ProviderIDs: map[string]string{"tmdb": "42"},
			People:      []MediaPerson{{Name: "Scanner Person", Role: "Actor"}},
		},
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := server.withBackgroundTxTagged(context.Background(), nil, func(tx *sql.Tx) error {
		return ensureScannedMediaIdentityTx(tx, file, now)
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`UPDATE media_items SET title = 'Locked Title', summary = 'Old summary' WHERE id = ?`, mediaID); err != nil {
		t.Fatal(err)
	}
	if err := server.replaceMetadataLocks(mediaID, []string{"title"}, "test-user"); err != nil {
		t.Fatal(err)
	}
	var revision int
	if err := server.withBackgroundTxTagged(context.Background(), nil, func(tx *sql.Tx) error {
		var applyErr error
		revision, _, applyErr = applyScannedMetadataRevisionTx(context.Background(), tx, file, now)
		if applyErr != nil {
			return applyErr
		}
		if err := replaceMediaSearchTx(context.Background(), tx, mediaID, revision, "nfo"); err != nil {
			return err
		}
		return replaceMediaCategoryFacetsTx(context.Background(), tx, mediaID, revision, "nfo")
	}); err != nil {
		t.Fatal(err)
	}
	var title, summary, etag string
	var canonicalRevision, revisionRows, peopleRevision, providerRevision, facetRevision int
	if err := server.db.QueryRow(`SELECT title, summary, metadata_revision, metadata_etag FROM media_items WHERE id = ?`, mediaID).Scan(&title, &summary, &canonicalRevision, &etag); err != nil {
		t.Fatal(err)
	}
	if title != "Locked Title" || summary != "NFO summary" {
		t.Fatalf("locked/unlocked fields = %q / %q", title, summary)
	}
	if err := server.db.QueryRow(`SELECT count(*) FROM media_metadata_revisions WHERE media_id = ? AND revision = ?`, mediaID, revision).Scan(&revisionRows); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT metadata_revision FROM media_people WHERE media_id = ? AND name = 'Scanner Person'`, mediaID).Scan(&peopleRevision); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT evidence_revision FROM media_provider_ids WHERE media_id = ? AND provider = 'tmdb'`, mediaID).Scan(&providerRevision); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT metadata_revision FROM media_category_facets WHERE media_id = ? LIMIT 1`, mediaID).Scan(&facetRevision); err != nil {
		t.Fatal(err)
	}
	if canonicalRevision != revision || revisionRows != 1 || peopleRevision != revision || providerRevision != revision || facetRevision != revision || etag == "" {
		t.Fatalf("revision mismatch canonical=%d rows=%d people=%d provider=%d facet=%d etag=%q", canonicalRevision, revisionRows, peopleRevision, providerRevision, facetRevision, etag)
	}
}

func TestScannerPublicationFailureDoesNotAdvanceMetadataRevision(t *testing.T) {
	server := newScannerTestServer(t)
	root := t.TempDir()
	mediaPath := filepath.Join(root, "Revision Retry.mkv")
	sidecarPath := filepath.Join(root, "Revision Retry.en.srt")
	if err := os.WriteFile(mediaPath, []byte("movie"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sidecarPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nOriginal\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	library, err := server.createLibrary(CreateLibraryRequest{Name: "Revision Retry", Type: "movie", Paths: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = server.performLibraryScanWithMode(context.Background(), library, "", "reconcile"); err != nil {
		t.Fatal(err)
	}
	var mediaID string
	var before int
	if err = server.db.QueryRow(`SELECT id, metadata_revision FROM media_items WHERE library_id = ? AND type = 'movie'`, library.ID).Scan(&mediaID, &before); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(mediaPath, []byte("movie changed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(sidecarPath, []byte("1\n00:00:01,000 --> 00:00:02,000\nChanged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	originalPublish := scannerPublishArtifact
	t.Cleanup(func() { scannerPublishArtifact = originalPublish })
	scannerPublishArtifact = func(string, string, []byte) error { return fs.ErrPermission }
	if _, err = server.performLibraryScanWithMode(context.Background(), library, "", "force_full"); !errors.Is(err, fs.ErrPermission) {
		t.Fatalf("scan error = %v", err)
	}
	var after int
	if err = server.db.QueryRow(`SELECT metadata_revision FROM media_items WHERE id = ?`, mediaID).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("publication failure advanced metadata revision %d -> %d", before, after)
	}
}
