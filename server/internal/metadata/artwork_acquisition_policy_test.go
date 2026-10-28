package metadata

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"image"
	"io"
	"net/http"
	"testing"
)

func TestArtworkUploadsUseCurrentSizeAndKeepDerivedBucketsFixed(t *testing.T) {
	for _, edge := range []int{1280, 1920, 2560} {
		s, db, target := repairFixture(t)
		ctx := context.Background()
		setArtworkPolicy(t, s, edge)
		before, err := s.RepairState(ctx, target, allowAll)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.UploadArtwork(ctx, target, "poster", "", before.Revision, editorJPEG(t, 2700, 1080), editorActor(), allowAll); err != nil {
			t.Fatal(edge, err)
		}
		var width, height, thumbWidth, mediumWidth int
		entity := resolveArtworkTestEntity(t, db, target)
		if err = db.QueryRow(`SELECT original.width,original.height,thumb.width,medium.width FROM artwork_selections s JOIN artwork_objects original ON original.digest=s.digest JOIN artwork_objects thumb ON thumb.digest=s.thumbnail_digest JOIN artwork_variants v ON v.source_digest=s.digest AND v.width=800 JOIN artwork_objects medium ON medium.digest=v.digest WHERE s.entity_id=? AND s.role='poster'`, entity).Scan(&width, &height, &thumbWidth, &mediumWidth); err != nil {
			t.Fatal(err)
		}
		if width != edge || height != edge*1080/2700 || thumbWidth != artworkSmallEdge || mediumWidth != artworkSmallEdge {
			t.Fatal("stored upload or derived buckets ignored actual policy", edge, width, height, thumbWidth, mediumWidth)
		}
		// The stored original is the policy edge. A request for the supported
		// 2560 representation is at or above that edge, so it opens the selected
		// original itself instead of a derived bucket.
		selected, _, err := s.EntityArtworkVariant(ctx, target, "poster", "", 2560, "")
		if err != nil {
			t.Fatal(edge, err)
		}
		cfg, _, err := image.DecodeConfig(selected)
		selected.Close()
		if err != nil || cfg.Width != edge {
			t.Fatalf("supported 2560 representation opened %d px (%v), want the %d px selected original", cfg.Width, err, edge)
		}
		// The legacy EntityArtwork default stays 1920: it never upscales a
		// smaller original and downscales the 2560 one.
		legacy, _, err := s.Artwork(ctx, target.ID, "poster")
		if err != nil {
			t.Fatal(edge, err)
		}
		cfg, _, err = image.DecodeConfig(legacy)
		legacy.Close()
		if err != nil || cfg.Width != min(edge, 1920) {
			t.Fatalf("legacy default served %d px (%v), want %d", cfg.Width, err, min(edge, 1920))
		}
	}
}

func TestArtworkUploadPolicyRevisionIsGuardedInThePublicationTransaction(t *testing.T) {
	s, db, target := repairFixture(t)
	ctx := context.Background()
	before, err := s.RepairState(ctx, target, allowAll)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.UploadArtwork(ctx, target, "poster", "", before.Revision, editorJPEG(t, 2000, 800), editorActor(), func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx, `UPDATE metadata_runtime_settings SET revision=revision+1,artwork_max_edge=1280 WHERE singleton=1`)
		return err
	})
	if !errors.Is(err, ErrRepairConflict) {
		t.Fatal("upload published under a different policy revision", err)
	}
	var selections, uploads int
	if err = db.QueryRow(`SELECT (SELECT count(*) FROM artwork_selections),(SELECT count(*) FROM artwork_uploads)`).Scan(&selections, &uploads); err != nil || selections != 0 || uploads != 0 {
		t.Fatal("stale upload partially published", selections, uploads, err)
	}
}

func TestProviderArtworkRetriesWhenPolicyChangesDuringAcquisition(t *testing.T) {
	s, db, target, _ := integrationScreen(t)
	ctx := context.Background()
	setArtworkPolicy(t, s, 1280)
	body := p08cPNG(t, 2700, 1080)
	requests := 0
	s.artClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			setArtworkPolicy(t, s, 1920)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
	})}
	if err := s.ScreenStep(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.ArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	var selected int
	if err := db.QueryRow(`SELECT count(*) FROM artwork_selections`).Scan(&selected); err != nil || selected != 0 {
		t.Fatal("old-policy provider pixels published", selected, err)
	}
	var retry bool
	if err := db.QueryRow(`SELECT EXISTS(SELECT 1 FROM artwork_jobs WHERE status='retry' AND error='encoding_policy_changed' AND attempts=0 AND lease='')`).Scan(&retry); err != nil || !retry {
		t.Fatal("policy change lost retryable work", retry, err)
	}
	if err := s.ArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	f, _, err := s.Artwork(ctx, target.ID, "poster")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, _, err := image.DecodeConfig(f)
	if err != nil || cfg.Width != 1920 || requests != 2 {
		t.Fatal("next bounded turn did not reacquire using new policy", cfg, requests, err)
	}
}
