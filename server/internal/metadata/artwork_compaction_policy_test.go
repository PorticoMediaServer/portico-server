package metadata

import (
	"context"
	"database/sql"
	"fmt"
	"image"
	"math/rand"
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
	if width != 1280 || thumb != artworkSmallEdge || medium != artworkSmallEdge || !locked || buckets != 3 {
		t.Fatal("compaction lost current pixels, fixed buckets or owner lock", width, thumb, medium, locked, buckets)
	}
	var uploadedWidth, uploadedHeight int
	if err = db.QueryRow(`SELECT width,height FROM artwork_uploads`).Scan(&uploadedWidth, &uploadedHeight); err != nil || uploadedWidth != 1280 || uploadedHeight != 512 {
		t.Fatal("upload projection retained obsolete display dimensions", uploadedWidth, uploadedHeight, err)
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

func TestArtworkCompactionHonoursLargerCeilingWithoutReencodingValidObjects(t *testing.T) {
	s, db, _ := repairFixture(t)
	ctx := context.Background()
	setArtworkPolicy(t, s, 2560)
	encoded, _, err := s.CaptureArtworkContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	picture := image.NewNRGBA(image.Rect(0, 0, 1600, 1000))
	random := rand.New(rand.NewSource(88))
	for i := 0; i < len(picture.Pix); i += 4 {
		picture.Pix[i], picture.Pix[i+1], picture.Pix[i+2], picture.Pix[i+3] = byte(random.Intn(256)), byte(random.Intn(256)), byte(random.Intn(256)), 255
	}
	raw, w, h, err := encodeArtworkSlot(encoded, picture, artworkSlotLarge)
	if err != nil || len(raw) <= artworkLargeCap[artworkLargeEdge]*3/2 || len(raw) > artworkLargeCap[2560]*3/2 {
		t.Fatal("fixture did not exercise the Larger ceiling", len(raw), err)
	}
	installed, err := s.installArtworkContext(encoded, raw, w, h)
	if err != nil {
		t.Fatal(err)
	}
	if err = dbwork.WithWriteTx(ctx, db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		return recordArtworkObject(ctx, tx, installed, "2020-01-01T00:00:00Z")
	}); err != nil {
		t.Fatal(err)
	}
	for n := 0; n < 3; n++ {
		if err = s.CompactArtworkStep(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var stored int
	if err = db.QueryRow(`SELECT bytes FROM artwork_objects WHERE digest=?`, installed.digest).Scan(&stored); err != nil || stored != len(raw) || s.artworkCompactionHasBacklog() {
		t.Fatal("valid Larger object was rewritten or the sweep did not idle", stored, err)
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
		measured, cost := dbwork.Measure(ctx)
		candidate, err := s.nextArtworkCompaction(measured, policy)
		if err != nil || candidate != "" || !s.artworkCompactionHasBacklog() {
			t.Fatal("walk exceeded its 64-row quantum", n, candidate, err)
		}
		if got := cost().Statements; got != 1 {
			t.Fatal("bounded page performed per-object reads", got)
		}
	}
	candidate, err := s.nextArtworkCompaction(ctx, policy)
	if err != nil || candidate != fmt.Sprintf("%064x", 129) {
		t.Fatal("walk did not reach the later candidate", candidate, err)
	}
	if _, err = s.nextArtworkCompaction(ctx, policy); err != sql.ErrNoRows || s.artworkCompactionHasBacklog() {
		t.Fatal("exhausted walk did not idle", err)
	}
	idle, cost := dbwork.Measure(ctx)
	if _, err = s.nextArtworkCompaction(idle, policy); err != sql.ErrNoRows || cost().Statements != 0 {
		t.Fatal("completed immutable-object sweep queried the table again", cost(), err)
	}
	if _, err = s.nextArtworkCompaction(ctx, artworkEncodingPolicy{edge: 1920, ceiling: 500000}); err != sql.ErrNoRows || s.artworkCompactionHasBacklog() {
		t.Fatal("standard policy retained stale walk progress", err)
	}
}
