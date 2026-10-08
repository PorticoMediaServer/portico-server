package metadata

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestArtworkRepresentationsBoundBytesDimensionsAndKeepAlpha(t *testing.T) {
	for _, alpha := range []bool{false, true} {
		img := image.NewNRGBA(image.Rect(0, 0, 2400, 1600))
		var random uint32 = 42
		for y := 0; y < 1600; y++ {
			for x := 0; x < 2400; x++ {
				random = random*1664525 + 1013904223
				a := uint8(255)
				if alpha {
					a = uint8(random >> 24)
				}
				img.SetNRGBA(x, y, color.NRGBA{uint8(random), uint8(random >> 8), uint8(random >> 16), a})
			}
		}
		for _, edge := range []int{artworkSmallEdge, artworkLargeEdge} {
			data, w, h, err := encodeDisplayArtwork(img, edge, false)
			if err != nil {
				t.Fatal(err)
			}
			if len(data) > artworkDisplayBytes || max(w, h) > edge || (w*2-h*3 > 3 || w*2-h*3 < -3) {
				t.Fatal("unbounded or distorted representation", len(data), w, h)
			}
			decoded, format, err := image.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatal(err)
			}
			if alpha {
				if format != "png" {
					t.Fatal("lost transparency")
				}
				_, _, _, a := decoded.At(0, 0).RGBA()
				if a == 65535 {
					t.Fatal("alpha flattened")
				}
			}
		}
	}
}

func TestArtworkCompactionPreservesSelectionAndLocks(t *testing.T) {
	s, db, target, _ := integrationScreen(t)
	ctx := context.Background()
	s.artClient = &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(p08cPNG(t, 20, 20))), Header: http.Header{}}, nil
	})}
	if err := s.ScreenStep(ctx); err != nil {
		t.Fatal(err)
	}
	if err := s.ArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	// Model a pre-policy original while retaining the real candidate/selection.
	old, err := s.installArtwork(p08cPNG(t, 2400, 1600), 2400, 1600)
	if err != nil {
		t.Fatal(err)
	}
	integrationExec(t, db, `INSERT INTO artwork_objects VALUES(?,'image/png',2400,1600,?,'2020-01-01T00:00:00Z','ready')`, old.digest, old.size)
	integrationExec(t, db, `UPDATE artwork_selections SET digest=?,locked=1 WHERE entity_id=? AND role='poster'`, old.digest, resolveArtworkTestEntity(t, db, target))
	// Undo and retained owner uploads must follow the same atomic replacement.
	integrationExec(t, db, `INSERT INTO artwork_uploads SELECT candidate_id,kind,entity_id,role,subject,digest,thumbnail_digest,'image/png',?,2400,1600,'owner','2020-01-01T00:00:00Z' FROM artwork_selections WHERE entity_id=? AND role='poster'`, old.size, resolveArtworkTestEntity(t, db, target))
	integrationExec(t, db, `INSERT INTO metadata_owner_history(kind,entity_id,base_revision,revision,trigger,actor,observed_at,before_json,after_json) VALUES('item',?,'before','after','upload','owner','2020-01-01T00:00:00Z',?,?)`, resolveArtworkTestEntity(t, db, target), `{"digest":"`+old.digest+`"}`, `{"digest":"`+old.digest+`"}`)
	reader, err := os.Open(filepath.Join(s.cacheRoot, old.digest+".img"))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err = s.CompactArtworkStep(ctx); err != nil {
		t.Fatal(err)
	}
	var digest string
	var locked int
	if err = db.QueryRow(`SELECT digest,locked FROM artwork_selections WHERE entity_id=? AND role='poster'`, resolveArtworkTestEntity(t, db, target)).Scan(&digest, &locked); err != nil {
		t.Fatal(err)
	}
	var retained string
	if err = db.QueryRow(`SELECT digest FROM artwork_uploads WHERE entity_id=?`, resolveArtworkTestEntity(t, db, target)).Scan(&retained); err != nil || retained != digest {
		t.Fatal("upload reference lost", retained, err)
	}
	var oldRefs int
	if err = db.QueryRow(`SELECT count(*) FROM metadata_owner_history WHERE instr(before_json,?)>0 OR instr(after_json,?)>0`, old.digest, old.digest).Scan(&oldRefs); err != nil || oldRefs != 0 {
		t.Fatal("undo reference left on original", err)
	}
	if digest == old.digest || locked != 1 {
		t.Fatal("selection was lost or not compacted")
	}
	f, _, err := s.Artwork(ctx, target.ID, "poster")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	cfg, err := png.DecodeConfig(f)
	if err != nil || max(cfg.Width, cfg.Height) > artworkLargeEdge {
		t.Fatal(cfg, err)
	}
	if data, err := io.ReadAll(reader); err != nil || len(data) != old.size {
		t.Fatal("old reader invalidated", err)
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("dangling artwork reference")
	}
	if err = s.CompactArtworkStep(ctx); err != nil {
		t.Fatal("idempotent compaction", err)
	}
}
