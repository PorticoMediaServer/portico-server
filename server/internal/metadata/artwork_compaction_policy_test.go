package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"testing"

	"portico.local/server/internal/dbwork"
)

func TestArtworkCompactionShrinksSelectedUploadAndPreservesVariantReferences(t *testing.T) {
	s, db, target := repairFixture(t)
	ctx := context.Background()
	before, err := s.RepairState(ctx, target, allowAll)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UploadArtwork(ctx, target, "poster", "", before.Revision, editorJPEG(t, 2700, 1080), editorActor(), allowAll); err != nil {
		t.Fatal(err)
	}
	setArtworkPolicy(t, s, 1280)
	if err = s.CompactArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	var width, thumb, medium, buckets int
	var locked bool
	if err = db.QueryRow(`SELECT o.width,t.width,m.width,s.locked,(SELECT count(*) FROM artwork_variants WHERE source_digest=s.digest) FROM artwork_selections s JOIN artwork_objects o ON o.digest=s.digest JOIN artwork_objects t ON t.digest=s.thumbnail_digest JOIN artwork_variants v ON v.source_digest=s.digest AND v.width=800 JOIN artwork_objects m ON m.digest=v.digest WHERE s.role='poster'`).Scan(&width, &thumb, &medium, &locked, &buckets); err != nil {
		t.Fatal(err)
	}
	if width != 1280 || thumb != 400 || medium != 800 || !locked || buckets != 3 {
		t.Fatal("compaction lost current pixels, fixed buckets or owner lock", width, thumb, medium, locked, buckets)
	}
	var violations int
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		violations++
	}
	err = rows.Err()
	rows.Close()
	if err != nil || violations != 0 {
		t.Fatal("compaction left broken references", violations, err)
	}
}

func TestArtworkCompactionNonstandardWalkIsBoundedAndStopsAtTheEnd(t *testing.T) {
	s := artworkPolicyService(t)
	ctx := context.Background()
	if err := dbwork.WithWriteTx(ctx, s.db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		for n := 0; n < 130; n++ {
			width := 1000
			if n == 129 {
				width = 1800
			}
			if _, err := tx.ExecContext(ctx, `INSERT INTO artwork_objects VALUES(?,'image/jpeg',?,600,10000,'2020-01-01T00:00:00Z','ready')`, fmt.Sprintf("%064x", n), width); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	policy := artworkEncodingPolicy{edge: 1280, ceiling: 500000}
	for n := 0; n < 2; n++ {
		candidate, err := s.nextArtworkCompaction(ctx, policy)
		if err != nil || candidate != "" || !s.artworkCompactionHasBacklog() {
			t.Fatal("walk exceeded its 64-row quantum", n, candidate, err)
		}
	}
	candidate, err := s.nextArtworkCompaction(ctx, policy)
	if err != nil || candidate != fmt.Sprintf("%064x", 129) {
		t.Fatal("walk did not reach the later candidate", candidate, err)
	}
	if _, err = s.nextArtworkCompaction(ctx, policy); err != sql.ErrNoRows || s.artworkCompactionHasBacklog() {
		t.Fatal("exhausted walk did not idle", err)
	}
	if _, err = s.nextArtworkCompaction(ctx, artworkEncodingPolicy{edge: 1920, ceiling: 500000}); err != sql.ErrNoRows || s.artworkCompactionHasBacklog() {
		t.Fatal("standard policy retained stale walk progress", err)
	}
}
