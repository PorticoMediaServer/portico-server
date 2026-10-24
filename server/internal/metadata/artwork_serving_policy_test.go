package metadata

import (
	"context"
	"database/sql"
	"image"
	"path/filepath"
	"testing"

	"portico.local/server/internal/dbwork"
)

func TestArtworkLargerServingKeeps1920FixedAndReturnsStoredOriginalAtItsEdge(t *testing.T) {
	s, db, target := repairFixture(t)
	ctx := context.Background()
	setArtworkPolicy(t, s, 2560)
	before, err := s.RepairState(ctx, target, allowAll)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UploadArtwork(ctx, target, "poster", "", before.Revision, editorJPEG(t, 2700, 1080), editorActor(), allowAll); err != nil {
		t.Fatal(err)
	}
	var source string
	if err = db.QueryRow(`SELECT digest FROM artwork_selections WHERE role='poster'`).Scan(&source); err != nil {
		t.Fatal(err)
	}
	// Even a caller carrying the full-image policy must get fixed serving
	// bounds, rather than accidentally overriding a requested 1920 with 2560.
	encoded, _, err := s.CaptureArtworkContext(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{400, 800, 1920, 2560} {
		f, _, err := s.EntityArtworkVariant(encoded, target, "poster", "", width, source)
		if err != nil {
			t.Fatal(width, err)
		}
		name := filepath.Base(f.Name())
		cfg, _, err := image.DecodeConfig(f)
		f.Close()
		if err != nil || cfg.Width != width {
			t.Fatal("serving bucket ignored fixed long edge", width, cfg, err)
		}
		if width == 2560 && name != source+".img" {
			t.Fatal("stored original was not returned at its long edge", name)
		}
	}
	// An old erroneous mapping cannot make a 1920 URL serve a 2560 object.
	if _, err = db.Exec(`UPDATE artwork_variants SET digest=? WHERE source_digest=? AND width=1920`, source, source); err != nil {
		t.Fatal(err)
	}
	f, _, err := s.EntityArtworkVariant(ctx, target, "poster", "", 1920, source)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(f)
	f.Close()
	if err != nil || cfg.Width != 1920 {
		t.Fatal("oversized retained bucket was served", cfg, err)
	}
}

func TestArtworkServingAboveStoredEdgeIgnoresSmallerCachedVariant(t *testing.T) {
	s, db, target := repairFixture(t)
	ctx := context.Background()
	setArtworkPolicy(t, s, 1280)
	before, err := s.RepairState(ctx, target, allowAll)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UploadArtwork(ctx, target, "poster", "", before.Revision, editorJPEG(t, 2700, 1080), editorActor(), allowAll); err != nil {
		t.Fatal(err)
	}
	var source, thumb string
	if err = db.QueryRow(`SELECT digest,thumbnail_digest FROM artwork_selections WHERE role='poster'`).Scan(&source, &thumb); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`UPDATE artwork_variants SET digest=? WHERE source_digest=? AND width=1920`, thumb, source); err != nil {
		t.Fatal(err)
	}
	for _, width := range []int{1920, 2560} {
		f, _, err := s.EntityArtworkVariant(ctx, target, "poster", "", width, source)
		if err != nil {
			t.Fatal(width, err)
		}
		name := filepath.Base(f.Name())
		cfg, _, err := image.DecodeConfig(f)
		f.Close()
		if err != nil || cfg.Width != 1280 || name != source+".img" {
			t.Fatal("larger request enlarged or substituted the stored picture", width, cfg, name, err)
		}
	}
}

func TestArtwork2560VariantPersistsFromLargerLegacyStoredSource(t *testing.T) {
	s, db, target := repairFixture(t)
	ctx := context.Background()
	before, err := s.RepairState(ctx, target, allowAll)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UploadArtwork(ctx, target, "poster", "", before.Revision, editorJPEG(t, 2700, 1080), editorActor(), allowAll); err != nil {
		t.Fatal(err)
	}
	legacy, err := s.installArtworkFile(editorJPEG(t, 3000, 1200), 3000, 1200)
	if err != nil {
		t.Fatal(err)
	}
	if err = dbwork.WithWriteTx(ctx, db, dbwork.ClassMaintenance, func(tx *sql.Tx) error {
		if err := recordArtworkObject(ctx, tx, legacy, "2020-01-01T00:00:00Z"); err != nil {
			return err
		}
		_, err := tx.ExecContext(ctx, `UPDATE artwork_selections SET digest=?,revision=revision+1 WHERE role='poster'`, legacy.digest)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f, _, err := s.EntityArtworkVariant(ctx, target, "poster", "", 2560, legacy.digest)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(f)
	f.Close()
	if err != nil || cfg.Width != 2560 {
		t.Fatal("legacy source did not produce a real 2560 variant", cfg, err)
	}
	var storedWidth int
	if err = db.QueryRow(`SELECT o.width FROM artwork_variants v JOIN artwork_objects o ON o.digest=v.digest WHERE v.source_digest=? AND v.width=2560`, legacy.digest).Scan(&storedWidth); err != nil || storedWidth != 2560 {
		t.Fatal("canonical variant relation did not retain 2560", storedWidth, err)
	}
}
