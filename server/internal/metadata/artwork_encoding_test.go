package metadata

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"portico.local/server/internal/imagework"
	"testing"
)

// artworkFixture is a deterministic picture: smooth gradients compress well,
// noise does not. Alpha, when set, varies across the picture.
func artworkFixture(w, h int, noise, alpha bool) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	var random uint32 = 42
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			random = random*1664525 + 1013904223
			c := color.NRGBA{uint8(x * 255 / w), uint8(y * 255 / h), uint8((x + y) * 127 / (w + h)), 255}
			if noise {
				c = color.NRGBA{uint8(random), uint8(random >> 8), uint8(random >> 16), 255}
			}
			if alpha {
				c.A = uint8(x * 255 / w)
				if noise {
					c.A = uint8(random >> 24)
				}
			}
			img.SetNRGBA(x, y, c)
		}
	}
	return img
}

// artworkLadder encodes the scaled picture at every quality rung, exactly as
// the slot encoder scales it.
func artworkLadder(t *testing.T, source image.Image, w, h int) map[int][]byte {
	t.Helper()
	scaled := image.NewNRGBA(image.Rect(0, 0, w, h))
	if err := imagework.CatmullRom(context.Background(), scaled, source); err != nil {
		t.Fatal(err)
	}
	out := map[int][]byte{}
	for _, quality := range artworkJPEGQualities {
		var b bytes.Buffer
		if err := jpeg.Encode(&b, scaled, &jpeg.Options{Quality: quality}); err != nil {
			t.Fatal(err)
		}
		out[quality] = b.Bytes()
	}
	return out
}

func artworkTally(slot, format string) artworkEncodingCount {
	return ArtworkEncodingCounts()[slot][format]
}

// Opaque art is JPEG at full slot size. The first ladder rung within the cap
// wins; past every rung, only the floor may publish, inside 1.5x the cap, and
// it is counted. Nothing is ever encoded below q75 or at a smaller size.
func TestArtworkSlotJPEGLadderFloorAndAllowance(t *testing.T) {
	ctx := context.Background()
	source := artworkFixture(1200, 1800, true, false)
	w, h := 533, artworkSmallEdge
	ladder := artworkLadder(t, source, w, h)
	q90, q82, q75 := len(ladder[90]), len(ladder[82]), len(ladder[75])
	if !(q90 > q82 && q82 > q75) {
		t.Fatal("fixture ladder is not monotonic", q90, q82, q75)
	}
	cases := []struct {
		name  string
		limit int
		want  []byte
		state string
	}{
		{"first rung fits", q90, ladder[90], "within"},
		{"middle rung fits", q82, ladder[82], "within"},
		{"floor fits", q75, ladder[75], "within"},
		{"floor inside allowance", q75 - 1, ladder[75], "outlier"},
		{"floor at the allowance edge", (q75*2 + 2) / 3, ladder[75], "outlier"},
		{"floor past the allowance", q75*2/3 - 1, nil, "unreachable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			before := artworkTally("small", "jpeg")
			raw, gotW, gotH, err := encodeArtworkWithin(ctx, source, artworkSlotSmall, artworkSmallEdge, tc.limit)
			after := artworkTally("small", "jpeg")
			if tc.state == "unreachable" {
				if !errors.Is(err, errArtworkCapUnreachable) || raw != nil {
					t.Fatal("a picture past its allowance was published", len(raw), err)
				}
				if after.CapUnreachable != before.CapUnreachable+1 {
					t.Fatal("cap_unreachable was not counted")
				}
				return
			}
			if err != nil || gotW != w || gotH != h {
				t.Fatal("the slot was not encoded at its full size", gotW, gotH, err)
			}
			if !bytes.Equal(raw, tc.want) {
				t.Fatal("the encoder left the quality ladder", len(raw), len(tc.want))
			}
			if tc.state == "outlier" && (after.Outliers != before.Outliers+1 || len(raw) > tc.limit*3/2) {
				t.Fatal("an outlier was not counted or exceeded the allowance", len(raw), tc.limit)
			}
			if tc.state == "within" && after.Within != before.Within+1 {
				t.Fatal("a picture within its cap was not counted")
			}
		})
	}
}

// The real slots: a smooth picture fits both caps at full size; noise at the
// small slot is past its allowance even at q75 and is refused, not shrunk.
func TestArtworkSlotsUseTheirApprovedEdgesAndCaps(t *testing.T) {
	ctx := context.Background()
	smooth := artworkFixture(2400, 1600, false, false)
	for _, slot := range []artworkSlot{artworkSlotSmall, artworkSlotLarge} {
		edge, limit := artworkSlotLimits(ctx, slot)
		raw, w, h, err := encodeArtworkSlot(ctx, smooth, slot)
		if err != nil || w != edge || h != edge*1600/2400 || len(raw) > limit {
			t.Fatal("smooth art did not fit its slot at full size", slot, w, h, len(raw), err)
		}
		if _, format, err := image.DecodeConfig(bytes.NewReader(raw)); err != nil || format != "jpeg" {
			t.Fatal("opaque art was not JPEG", format, err)
		}
	}
	if edge, limit := artworkSlotLimits(ctx, artworkSlotSmall); edge != 800 || limit != 150_000 {
		t.Fatal("small slot limits", edge, limit)
	}
	for edge, limit := range map[int]int{1280: 300_000, 1920: 500_000, 2560: 800_000} {
		if artworkLargeCap[edge] != limit {
			t.Fatal("large cap", edge, artworkLargeCap[edge])
		}
	}
	// Unscaled noise at exactly the small edge needs about 260 KB at q75.
	if _, _, _, err := encodeArtworkSlot(ctx, artworkFixture(533, 800, true, false), artworkSlotSmall); !errors.Is(err, errArtworkCapUnreachable) {
		t.Fatal("noise past the small allowance was published", err)
	}
}

// Opaque PNG input is still opaque art: it becomes JPEG.
func TestArtworkSlotEncodesOpaquePNGInputAsJPEG(t *testing.T) {
	var source bytes.Buffer
	if err := png.Encode(&source, artworkFixture(900, 600, false, false)); err != nil {
		t.Fatal(err)
	}
	decoded, format, err := image.Decode(bytes.NewReader(source.Bytes()))
	if err != nil || format != "png" {
		t.Fatal(format, err)
	}
	raw, _, _, err := encodeArtworkSlot(context.Background(), decoded, artworkSlotSmall)
	if err != nil {
		t.Fatal(err)
	}
	if _, format, err = image.DecodeConfig(bytes.NewReader(raw)); err != nil || format != "jpeg" {
		t.Fatal("opaque PNG input stayed PNG", format, err)
	}
}

// Transparency is lossless PNG: the decoded pixels are exactly the scaled
// pixels, whether written as an exact palette or full colour. Noisy alpha past
// the allowance is refused, never shrunk or flattened.
func TestArtworkSlotKeepsAlphaLossless(t *testing.T) {
	ctx := context.Background()
	flat := image.NewNRGBA(image.Rect(0, 0, 600, 300))
	for y := 0; y < 300; y++ {
		for x := 0; x < 600; x++ {
			flat.SetNRGBA(x, y, color.NRGBA{uint8(x / 100 * 40), 80, 200, uint8(y / 60 * 50)})
		}
	}
	for name, source := range map[string]*image.NRGBA{"palette": flat, "full colour": artworkFixture(700, 400, false, true)} {
		before := artworkTally("small", "png")
		raw, w, h, err := encodeArtworkSlot(ctx, source, artworkSlotSmall)
		if err != nil || w != source.Bounds().Dx() || h != source.Bounds().Dy() {
			t.Fatal(name, "alpha slot changed size", w, h, err)
		}
		decoded, format, err := image.Decode(bytes.NewReader(raw))
		if err != nil || format != "png" {
			t.Fatal(name, "alpha was not PNG", format, err)
		}
		scaled := image.NewNRGBA(image.Rect(0, 0, w, h))
		if err = imagework.CatmullRom(ctx, scaled, source); err != nil {
			t.Fatal(err)
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				if got := color.NRGBAModel.Convert(decoded.At(x, y)).(color.NRGBA); got != scaled.NRGBAAt(x, y) {
					t.Fatal(name, "PNG was not lossless at", x, y, got, scaled.NRGBAAt(x, y))
				}
			}
		}
		if _, paletted := decoded.(*image.Paletted); paletted != (name == "palette") {
			t.Fatal(name, "exact palette choice", paletted)
		}
		if after := artworkTally("small", "png"); after.Within+after.Outliers != before.Within+before.Outliers+1 {
			t.Fatal(name, "alpha encoding was not counted")
		}
	}
	before := artworkTally("small", "png")
	if _, _, _, err := encodeArtworkSlot(ctx, artworkFixture(700, 400, true, true), artworkSlotSmall); !errors.Is(err, errArtworkCapUnreachable) {
		t.Fatal("noisy alpha past the allowance was published", err)
	}
	if after := artworkTally("small", "png"); after.CapUnreachable != before.CapUnreachable+1 {
		t.Fatal("refused alpha was not counted")
	}
}

// The output buffer is charged once at the allowance and never grows past it.
func TestArtworkBoundedBufferRefusesToGrow(t *testing.T) {
	out := &artworkBoundedBuffer{limit: 8, b: make([]byte, 0, 8)}
	if n, err := out.Write([]byte("12345678")); n != 8 || err != nil {
		t.Fatal(n, err)
	}
	if n, err := out.Write([]byte("9")); n != 0 || !errors.Is(err, errArtworkEncodedTooLarge) || cap(out.b) != 8 {
		t.Fatal("bounded buffer grew", n, err, cap(out.b))
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
