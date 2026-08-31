package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestMetadataApplySameRevisionHasExactlyOneWinner(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	seed, err := server.getMedia("", "movie_meridian")
	if err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		index := index
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			summary := fmt.Sprintf("concurrent summary %d", index)
			_, applyErr := server.applyMetadata(context.Background(), metadataApplyRequest{
				MediaID:          seed.ID,
				ExpectedRevision: seed.MetadataRevision,
				Origin:           metadataSourceProvider,
				Source:           "concurrency-test",
				Provider:         "tmdb",
				Update:           UpdateMediaRequest{Summary: &summary},
			})
			errs <- applyErr
		}()
	}
	close(start)
	wait.Wait()
	close(errs)

	winners, conflicts := 0, 0
	for applyErr := range errs {
		switch {
		case applyErr == nil:
			winners++
		case errors.Is(applyErr, errMetadataRevisionConflict):
			conflicts++
		default:
			t.Fatalf("unexpected apply error: %v", applyErr)
		}
	}
	if winners != 1 || conflicts != 1 {
		t.Fatalf("winners=%d conflicts=%d, want 1/1", winners, conflicts)
	}
	after, err := server.getMedia("", seed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.MetadataRevision != seed.MetadataRevision+1 {
		t.Fatalf("revision=%d, want %d", after.MetadataRevision, seed.MetadataRevision+1)
	}
}

func TestMetadataApplyManualLockAndProjectionCommitTogether(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	seed, err := server.getMedia("", "movie_meridian")
	if err != nil {
		t.Fatal(err)
	}
	manualSummary := "The owner's preferred summary."
	genres := []string{"Adventure", "Science Fiction"}
	result, err := server.applyMetadata(context.Background(), metadataApplyRequest{
		MediaID:          seed.ID,
		ExpectedRevision: seed.MetadataRevision,
		Origin:           metadataSourceManual,
		Source:           "manual",
		ActorUserID:      "owner",
		Update: UpdateMediaRequest{
			Summary: &manualSummary,
			Genres:  &genres,
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var lockRevision, facetRevision int
	if err := server.db.QueryRow(`SELECT metadata_revision FROM media_metadata_locks WHERE media_id = ? AND field = 'summary'`, seed.ID).Scan(&lockRevision); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT metadata_revision FROM media_category_facets WHERE media_id = ? AND facet_type = 'genre' ORDER BY sort_value LIMIT 1`, seed.ID).Scan(&facetRevision); err != nil {
		t.Fatal(err)
	}
	if lockRevision != result.Revision || facetRevision != result.Revision {
		t.Fatalf("revision mismatch result=%d lock=%d facet=%d", result.Revision, lockRevision, facetRevision)
	}
	var searchCount int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_search WHERE media_id = ? AND media_search MATCH 'Science'`, seed.ID).Scan(&searchCount); err != nil {
		t.Fatal(err)
	}
	if searchCount != 1 {
		t.Fatalf("search projection count=%d, want 1", searchCount)
	}

	providerSummary := "Provider tried to replace the manual summary."
	if _, err := server.applyMetadata(context.Background(), metadataApplyRequest{
		MediaID:          seed.ID,
		ExpectedRevision: result.Revision,
		Origin:           metadataSourceProvider,
		Source:           "provider-refresh",
		Provider:         "tmdb",
		Update:           UpdateMediaRequest{Summary: &providerSummary},
	}); err != nil {
		t.Fatal(err)
	}
	after, err := server.getMedia("", seed.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Summary != manualSummary {
		t.Fatalf("locked summary=%q, want %q", after.Summary, manualSummary)
	}
}

func TestMetadataApplyManualArtworkMutationIsRevisionedAndAtomic(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	seed, err := server.getMedia("", "movie_meridian")
	if err != nil {
		t.Fatal(err)
	}
	mutation := func(tx *sql.Tx, revision int, now string) error {
		_, err := tx.Exec(`
			INSERT INTO media_images (
				id, media_id, image_type, source, provider, path, remote_url, width, height,
				language, rating, sort_order, preferred, created_at, metadata_revision
			) VALUES ('manual_atomic_art', ?, 'poster', 'manual', 'upload', '/tmp/manual-atomic.png', '', 1, 1, '', 100, 0, 1, ?, ?)`,
			seed.ID, now, revision)
		return err
	}
	_, err = server.applyMetadata(context.Background(), metadataApplyRequest{
		MediaID: seed.ID, ExpectedRevision: seed.MetadataRevision, Origin: metadataSourceManual,
		Source: "manual", ActorUserID: "owner", ArtworkMutation: mutation,
		StageHook: func(stage string) error {
			if stage == "facets_search" {
				return errors.New("force rollback")
			}
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected forced failure")
	}
	var count int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_images WHERE id = 'manual_atomic_art'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rolled-back artwork rows=%d, want 0", count)
	}

	result, err := server.applyMetadata(context.Background(), metadataApplyRequest{
		MediaID: seed.ID, ExpectedRevision: seed.MetadataRevision, Origin: metadataSourceManual,
		Source: "manual", ActorUserID: "owner", ArtworkMutation: mutation,
	})
	if err != nil {
		t.Fatal(err)
	}
	var imageRevision, lockRevision int
	if err := server.db.QueryRow(`SELECT metadata_revision FROM media_images WHERE id = 'manual_atomic_art'`).Scan(&imageRevision); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT metadata_revision FROM media_metadata_locks WHERE media_id = ? AND field = 'artwork'`, seed.ID).Scan(&lockRevision); err != nil {
		t.Fatal(err)
	}
	if imageRevision != result.Revision || lockRevision != result.Revision {
		t.Fatalf("revision mismatch result=%d image=%d lock=%d", result.Revision, imageRevision, lockRevision)
	}
}

func TestMetadataApplyFailureStagesRollbackWholeRevision(t *testing.T) {
	stages := []string{"canonical", "identities", "people", "artwork", "locks", "facets_search", "refresh_outcome"}
	for _, stage := range stages {
		t.Run(stage, func(t *testing.T) {
			_, _, server := newDiscoveryTestServer(t, config.Config{})
			seed, err := server.getMedia("", "movie_meridian")
			if err != nil {
				t.Fatal(err)
			}
			title := "This change must roll back"
			_, err = server.applyMetadata(context.Background(), metadataApplyRequest{
				MediaID:          seed.ID,
				ExpectedRevision: seed.MetadataRevision,
				Origin:           metadataSourceManual,
				Source:           "manual",
				ActorUserID:      "owner",
				Update:           UpdateMediaRequest{Title: &title},
				Identities: []metadataProviderIdentityProposal{{
					Provider: "tmdb", ExternalID: "777", ExternalType: "movie", Confidence: 1,
				}},
				StageHook: func(current string) error {
					if current == stage {
						return errors.New("injected failure")
					}
					return nil
				},
			})
			if err == nil {
				t.Fatal("injected failure unexpectedly committed")
			}
			after, loadErr := server.getMedia("", seed.ID)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			if after.Title != seed.Title || after.MetadataRevision != seed.MetadataRevision {
				t.Fatalf("state advanced after rollback: title=%q revision=%d", after.Title, after.MetadataRevision)
			}
			var revisions, identities int
			if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_metadata_revisions WHERE media_id = ?`, seed.ID).Scan(&revisions); err != nil {
				t.Fatal(err)
			}
			if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_provider_ids WHERE media_id = ? AND external_id = '777'`, seed.ID).Scan(&identities); err != nil {
				t.Fatal(err)
			}
			if revisions != 0 || identities != 0 {
				t.Fatalf("partial rows survived rollback: revisions=%d identities=%d", revisions, identities)
			}
		})
	}
}

func TestMetadataApplyPersistsRichProviderSnapshotAndCarriesItForward(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	seed, err := server.getMedia("", "movie_meridian")
	if err != nil {
		t.Fatal(err)
	}
	proposal := newProviderRichProposal("tmdb", map[string]any{"id": 42, "title": "Meridian"})
	proposal.Values = append(proposal.Values, metadataProviderValueProposal{Field: "alternateTitle", Value: "Le Méridien", Locale: "fr"})
	proposal.Relationships = append(proposal.Relationships,
		metadataRelationshipProposal{Kind: "country", Name: "Canada", ExternalIDs: map[string]string{"iso3166-1": "CA"}, Attributes: map[string]string{"country": "CA"}},
		metadataRelationshipProposal{Kind: "track", Name: "Theme", Role: "1", ExternalIDs: map[string]string{"musicbrainz": "track-1"}},
	)
	proposal.Images = append(proposal.Images, metadataProviderImageProposal{Kind: "poster", Path: "/poster.jpg", Width: 1000, Height: 1500})
	proposal.normalize()
	result, err := server.applyMetadata(context.Background(), metadataApplyRequest{
		MediaID:          seed.ID,
		ExpectedRevision: seed.MetadataRevision,
		Origin:           metadataSourceProvider,
		Source:           "provider-refresh",
		Provider:         "tmdb",
		Identities: []metadataProviderIdentityProposal{{
			Provider: "tmdb", ExternalType: "movie", ExternalID: "42", Confidence: 1,
		}},
		ProviderRich: &proposal,
	})
	if err != nil {
		t.Fatal(err)
	}

	var storedHash, sourceHash, status string
	var storedBytes, sourceBytes, truncated int
	if err := server.db.QueryRow(`
		SELECT payload_sha256, source_payload_sha256, byte_length, source_byte_length, truncated, result_status
		FROM media_provider_snapshots WHERE media_id = ?`, seed.ID).Scan(&storedHash, &sourceHash, &storedBytes, &sourceBytes, &truncated, &status); err != nil {
		t.Fatal(err)
	}
	if storedHash != proposal.SnapshotHash || sourceHash != proposal.SourceHash || storedBytes != len(proposal.Snapshot) || sourceBytes != proposal.SourceBytes || truncated != 0 || status != "ok" {
		t.Fatalf("snapshot contract mismatch: stored=%s/%d source=%s/%d truncated=%d status=%s", storedHash, storedBytes, sourceHash, sourceBytes, truncated, status)
	}
	var locale, country, imagePath, remoteURL string
	if err := server.db.QueryRow(`SELECT locale FROM media_metadata_field_values WHERE media_id = ? AND field_key = 'alternateTitle'`, seed.ID).Scan(&locale); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT country FROM media_metadata_relationships WHERE media_id = ? AND relationship_type = 'country'`, seed.ID).Scan(&country); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT path, remote_url FROM media_images WHERE media_id = ? AND provider_image_id = '/poster.jpg'`, seed.ID).Scan(&imagePath, &remoteURL); err != nil {
		t.Fatal(err)
	}
	if locale != "fr" || country != "CA" || !strings.HasPrefix(imagePath, "provider-evidence:") || remoteURL != "" {
		t.Fatalf("rich provenance mismatch: locale=%q country=%q path=%q remote=%q", locale, country, imagePath, remoteURL)
	}

	summary := "A later manual edit"
	next, err := server.applyMetadata(context.Background(), metadataApplyRequest{
		MediaID: seed.ID, ExpectedRevision: result.Revision, Origin: metadataSourceManual,
		Source: "manual", ActorUserID: "owner", Update: UpdateMediaRequest{Summary: &summary},
	})
	if err != nil {
		t.Fatal(err)
	}
	var searchCount, carried int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_search WHERE media_id = ? AND media_search MATCH 'Méridien'`, seed.ID).Scan(&searchCount); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`
		SELECT COUNT(*) FROM media_metadata_field_values f
		JOIN media_metadata_revisions r ON r.id = f.revision_id
		WHERE f.media_id = ? AND r.revision = ? AND f.field_key = 'alternateTitle'`, seed.ID, next.Revision).Scan(&carried); err != nil {
		t.Fatal(err)
	}
	if searchCount != 1 || carried != 1 {
		t.Fatalf("carried rich evidence search=%d rows=%d, want 1/1", searchCount, carried)
	}
}

func TestMetadataApplyPersistsSupplementalProviderWithIndependentProvenance(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	seed, err := server.getMedia("", "movie_meridian")
	if err != nil {
		t.Fatal(err)
	}
	primary := newProviderRichProposal("musicbrainz", map[string]any{"id": "release-1"})
	primary.Relationships = append(primary.Relationships, metadataRelationshipProposal{
		Kind: "recording", Name: "Main recording", ExternalIDs: map[string]string{"musicbrainz": "recording-1"},
	})
	supplement := mapCoverArtArchiveProviderRich("release", "release-1", coverArtArchiveResponse{Images: []coverArtArchiveImage{{
		ID: 99, Image: "https://coverartarchive.org/release/release-1/front.jpg", Front: true, Approved: true,
	}}})
	primary.Supplements = append(primary.Supplements, supplement)

	if _, err := server.applyMetadata(context.Background(), metadataApplyRequest{
		MediaID: seed.ID, ExpectedRevision: seed.MetadataRevision, Origin: metadataSourceProvider,
		Source: "provider-refresh", Provider: "musicbrainz",
		Identities: []metadataProviderIdentityProposal{{
			Provider: "musicbrainz", ExternalType: "release", ExternalID: "release-1", Confidence: 1,
		}},
		ProviderRich: &primary,
	}); err != nil {
		t.Fatal(err)
	}

	rows, err := server.db.Query(`SELECT provider, external_type, external_id FROM media_provider_snapshots WHERE media_id = ? ORDER BY provider`, seed.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	got := [][3]string{}
	for rows.Next() {
		var row [3]string
		if err := rows.Scan(&row[0], &row[1], &row[2]); err != nil {
			t.Fatal(err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := [][3]string{{"coverartarchive", "release", "release-1"}, {"musicbrainz", "release", "release-1"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("provider snapshots = %#v, want %#v", got, want)
	}

	var imageProvider, relationshipProvider string
	var imageSnapshotMatches, relationshipSnapshotMatches int
	if err := server.db.QueryRow(`
		SELECT i.provider, COUNT(s.id)
		FROM media_images i
		LEFT JOIN media_provider_snapshots s ON s.id = i.snapshot_id AND s.provider = i.provider
		WHERE i.media_id = ? AND i.provider_image_id = ?
		GROUP BY i.provider`, seed.ID, supplement.Images[0].Path).Scan(&imageProvider, &imageSnapshotMatches); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`
		SELECT r.provider, COUNT(s.id)
		FROM media_metadata_relationships r
		LEFT JOIN media_provider_snapshots s ON s.id = r.snapshot_id AND s.provider = r.provider
		WHERE r.media_id = ? AND r.relationship_type = 'artwork'
		GROUP BY r.provider`, seed.ID).Scan(&relationshipProvider, &relationshipSnapshotMatches); err != nil {
		t.Fatal(err)
	}
	if imageProvider != "coverartarchive" || imageSnapshotMatches != 1 || relationshipProvider != "coverartarchive" || relationshipSnapshotMatches != 1 {
		t.Fatalf("supplement provenance image=%q/%d relationship=%q/%d", imageProvider, imageSnapshotMatches, relationshipProvider, relationshipSnapshotMatches)
	}
}

func TestMetadataFillMissingPersistsSnapshotWithoutParallelRichAuthority(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	seed, err := server.getMedia("", "movie_meridian")
	if err != nil {
		t.Fatal(err)
	}
	primary := newProviderRichProposal("tmdb", map[string]any{"id": 42})
	primary.Values = append(primary.Values, metadataProviderValueProposal{Field: "alternateTitle", Value: "Primary alternate"})
	primary.Relationships = append(primary.Relationships, metadataRelationshipProposal{Kind: "franchise", Name: "Primary franchise"})
	primary.Images = append(primary.Images, metadataProviderImageProposal{Kind: "logo", Path: "/primary-logo.png"})
	primary.normalize()
	first, err := server.applyMetadata(context.Background(), metadataApplyRequest{
		MediaID: seed.ID, ExpectedRevision: seed.MetadataRevision, Origin: metadataSourceProvider,
		Source: "provider-refresh", Provider: "tmdb", RefreshIntent: metadataRefreshUnlocked,
		Identities: []metadataProviderIdentityProposal{{
			Provider: "tmdb", ExternalType: "movie", ExternalID: "42", Confidence: 1,
		}},
		ProviderRich: &primary,
	})
	if err != nil {
		t.Fatal(err)
	}

	fallback := newProviderRichProposal("tvdb", map[string]any{"id": 84})
	fallback.Values = append(fallback.Values,
		metadataProviderValueProposal{Field: "alternateTitle", Value: "Fallback alternate"},
		metadataProviderValueProposal{Field: "format", Value: "Movie"},
	)
	fallback.Relationships = append(fallback.Relationships,
		metadataRelationshipProposal{Kind: "franchise", Name: "Fallback franchise"},
		metadataRelationshipProposal{Kind: "keyword", Name: "supplemental-keyword"},
	)
	fallback.Images = append(fallback.Images,
		metadataProviderImageProposal{Kind: "logo", Path: "/fallback-logo.png"},
		metadataProviderImageProposal{Kind: "backdrop", Path: "/fallback-backdrop.jpg"},
	)
	fallback.normalize()
	second, err := server.applyMetadata(context.Background(), metadataApplyRequest{
		MediaID: seed.ID, ExpectedRevision: first.Revision, Origin: metadataSourceProvider,
		Source: "provider-supplement", Provider: "tvdb", RefreshIntent: metadataRefreshFillMissing,
		Identities: []metadataProviderIdentityProposal{{
			Provider: "tvdb", ExternalType: "movie", ExternalID: "84", Confidence: 1,
		}},
		ProviderRich: &fallback,
	})
	if err != nil {
		t.Fatal(err)
	}

	var snapshotCount, fallbackAlternate, fallbackFormat, fallbackFranchise, fallbackKeyword int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_provider_snapshots WHERE media_id=? AND provider='tvdb'`, seed.ID).Scan(&snapshotCount); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`
		SELECT COUNT(*) FROM media_metadata_field_values f
		JOIN media_metadata_revisions r ON r.id=f.revision_id
		WHERE f.media_id=? AND r.revision=? AND f.provider='tvdb' AND f.field_key='alternateTitle'`, seed.ID, second.Revision).Scan(&fallbackAlternate); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`
		SELECT COUNT(*) FROM media_metadata_field_values f
		JOIN media_metadata_revisions r ON r.id=f.revision_id
		WHERE f.media_id=? AND r.revision=? AND f.provider='tvdb' AND f.field_key='format'`, seed.ID, second.Revision).Scan(&fallbackFormat); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`
		SELECT COUNT(*) FROM media_metadata_relationships rel
		JOIN media_metadata_revisions r ON r.id=rel.revision_id
		WHERE rel.media_id=? AND r.revision=? AND rel.provider='tvdb' AND rel.relationship_type='franchise'`, seed.ID, second.Revision).Scan(&fallbackFranchise); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`
		SELECT COUNT(*) FROM media_metadata_relationships rel
		JOIN media_metadata_revisions r ON r.id=rel.revision_id
		WHERE rel.media_id=? AND r.revision=? AND rel.provider='tvdb' AND rel.relationship_type='keyword'`, seed.ID, second.Revision).Scan(&fallbackKeyword); err != nil {
		t.Fatal(err)
	}
	var fallbackLogo, fallbackBackdrop int
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_images WHERE media_id=? AND provider='tvdb' AND image_type='logo'`, seed.ID).Scan(&fallbackLogo); err != nil {
		t.Fatal(err)
	}
	if err := server.db.QueryRow(`SELECT COUNT(*) FROM media_images WHERE media_id=? AND provider='tvdb' AND image_type='backdrop'`, seed.ID).Scan(&fallbackBackdrop); err != nil {
		t.Fatal(err)
	}
	if snapshotCount != 1 || fallbackAlternate != 0 || fallbackFranchise != 0 || fallbackLogo != 0 {
		t.Fatalf("fallback became parallel authority: snapshots=%d alternate=%d franchise=%d logo=%d", snapshotCount, fallbackAlternate, fallbackFranchise, fallbackLogo)
	}
	if fallbackFormat != 1 || fallbackKeyword != 1 || fallbackBackdrop != 1 {
		t.Fatalf("fallback did not fill missing classes: format=%d keyword=%d backdrop=%d", fallbackFormat, fallbackKeyword, fallbackBackdrop)
	}
}

func TestCleanupUnreferencedStagedMetadataArtworkPreservesReferencedFiles(t *testing.T) {
	_, _, server := newDiscoveryTestServer(t, config.Config{})
	root := filepath.Join(server.cfg.AppDataDir, "artwork", "provider", "movie_meridian")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	orphan := filepath.Join(root, "orphan.jpg")
	referenced := filepath.Join(root, "referenced.jpg")
	personReferenced := filepath.Join(root, "person-referenced.jpg")
	for _, path := range []string{orphan, referenced, personReferenced} {
		if err := os.WriteFile(path, []byte("image"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := server.db.Exec(`INSERT INTO media_images
		(id,media_id,image_type,source,provider,path,remote_url,created_at)
		VALUES ('img_cleanup_reference','movie_meridian','poster','provider','tmdb',?,'','2026-01-01T00:00:00Z')`, referenced); err != nil {
		t.Fatal(err)
	}
	if _, err := server.db.Exec(`INSERT INTO media_people
		(id,media_id,name,role,source,sort_order,image_url,created_at)
		VALUES ('person_cleanup_reference','movie_meridian','Existing Person','Actor','provider',0,?,'2026-01-01T00:00:00Z')`, personReferenced); err != nil {
		t.Fatal(err)
	}
	server.cleanupUnreferencedStagedMetadataArtwork(
		[]stagedMetadataImage{{Path: orphan}, {Path: referenced}},
		nil,
		[]MediaPerson{{ImageURL: orphan}, {ImageURL: personReferenced}},
	)
	if _, err := os.Stat(orphan); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unreferenced staged artwork still exists: %v", err)
	}
	if _, err := os.Stat(referenced); err != nil {
		t.Fatalf("referenced staged artwork was removed: %v", err)
	}
	if _, err := os.Stat(personReferenced); err != nil {
		t.Fatalf("referenced staged person portrait was removed: %v", err)
	}
}

func TestUpdateMediaFailureCleansUnreferencedStagedPersonPortrait(t *testing.T) {
	pngBytes, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAAEElEQVR4nGL6z8AACAAA//8DCQECWLbVUAAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	previousClient := providerArtworkHTTPClient
	providerArtworkHTTPClient = &http.Client{Transport: metadataRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"image/png"}},
			Body:       io.NopCloser(bytes.NewReader(pngBytes)),
			Request:    req,
		}, nil
	})}
	defer func() { providerArtworkHTTPClient = previousClient }()

	for _, test := range []struct {
		name   string
		mutate func(UpdateMediaRequest) UpdateMediaRequest
	}{
		{
			name: "revision conflict",
			mutate: func(req UpdateMediaRequest) UpdateMediaRequest {
				stale := 999
				req.ExpectedRevision = &stale
				return req
			},
		},
		{
			name: "canonical apply failure",
			mutate: func(req UpdateMediaRequest) UpdateMediaRequest {
				empty := ""
				req.Title = &empty
				return req
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, server := newDiscoveryTestServer(t, config.Config{})
			people := []MediaPerson{{
				Name: "Staged Person", Role: "Actor", ImageURL: "https://image.tmdb.org/t/p/w780/staged-person.png",
				ProviderIDs: map[string]string{"tmdb": "123"},
			}}
			req := test.mutate(UpdateMediaRequest{
				People:           &people,
				metadataOrigin:   metadataSourceProvider,
				metadataSource:   "tmdb",
				metadataProvider: "tmdb",
			})
			if _, err := server.updateMediaForMetadata("", "movie_meridian", req); err == nil {
				t.Fatal("expected metadata update failure")
			}
			root := filepath.Join(server.cfg.AppDataDir, "artwork", "provider", "movie_meridian")
			matches, err := filepath.Glob(filepath.Join(root, "person-*-tmdb-*"))
			if err != nil {
				t.Fatal(err)
			}
			if len(matches) != 0 {
				t.Fatalf("orphaned staged person portraits remain: %#v", matches)
			}
		})
	}
}
