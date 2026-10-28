package metadata

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"portico.local/server/internal/imagework"
)

func ambientTestPNG(t *testing.T, w, h int, alpha bool) []byte {
	t.Helper()
	im := image.NewNRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			c := color.NRGBA{R: 255, A: 255}
			if x%8 >= 4 {
				c = color.NRGBA{B: 255, A: 255}
			}
			if alpha && x >= w/2 {
				c = color.NRGBA{G: 255, A: 0}
			}
			im.SetNRGBA(x, y, c)
		}
	}
	var b bytes.Buffer
	if err := png.Encode(&b, im); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestAmbientArtworkEncodingIsSoftBoundedAndDeterministic(t *testing.T) {
	ctx := context.Background()
	raw := ambientTestPNG(t, 64, 64, false)
	encoded, w, h, err := encodeAmbientArtwork(ctx, raw)
	if err != nil || w != 64 || h != 64 || len(encoded) > artworkAmbientBytes {
		t.Fatal(w, h, len(encoded), err)
	}
	after, format, err := image.Decode(bytes.NewReader(encoded))
	if err != nil || format != "png" || after.Bounds().Dx() != w || after.Bounds().Dy() != h {
		t.Fatal(format, err)
	}
	before, _, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	variation := func(im image.Image) int64 {
		var total int64
		for x := 1; x < im.Bounds().Dx(); x++ {
			r1, _, _, _ := im.At(x-1, 32).RGBA()
			r2, _, _, _ := im.At(x, 32).RGBA()
			total += int64(max(r1, r2) - min(r1, r2))
		}
		return total
	}
	if variation(after)*10 >= variation(before) {
		t.Fatalf("encoded art was not softened: before=%d after=%d", variation(before), variation(after))
	}
	r, g, b, a := after.At(32, 32).RGBA()
	if r == 0 || b == 0 || g != 0 || a != 65535 {
		t.Fatalf("glow not derived from source colours: %d,%d,%d,%d", r, g, b, a)
	}
	repeat, _, _, err := encodeAmbientArtwork(ctx, raw)
	if err != nil || !bytes.Equal(encoded, repeat) {
		t.Fatal("non-deterministic encoding", err)
	}
	if out := os.Getenv("PORTICO_AMBIENT_EVIDENCE_DIR"); out != "" {
		for name, data := range map[string][]byte{"stripes-source.png": raw, "stripes-ambient.png": encoded} {
			if err := os.WriteFile(filepath.Join(out, name), data, 0600); err != nil {
				t.Fatal(err)
			}
		}
		if path := os.Getenv("PORTICO_AMBIENT_EVIDENCE_SOURCE"); path != "" {
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			source, _, _, _, err := normalizeArtwork(original)
			if err != nil {
				t.Fatal(err)
			}
			ambient, _, _, err := encodeAmbientArtwork(ctx, source)
			if err != nil {
				t.Fatal(err)
			}
			for name, data := range map[string][]byte{"cover-source.img": source, "cover-ambient.png": ambient} {
				if err := os.WriteFile(filepath.Join(out, name), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}

func TestAmbientArtworkPreservesAspectAlphaAndNeverUpscales(t *testing.T) {
	for _, size := range [][2]int{{10, 7}, {160, 90}, {90, 160}, {1, 1}, {1, 200}, {200, 1}} {
		raw := ambientTestPNG(t, size[0], size[1], true)
		encoded, w, h, err := encodeAmbientArtwork(context.Background(), raw)
		if err != nil || w > size[0] || h > size[1] || max(w, h) > 64 || len(encoded) > artworkAmbientBytes {
			t.Fatal(size, w, h, err)
		}
		wantW, wantH := size[0], size[1]
		if edge := max(wantW, wantH); edge > 64 {
			wantW, wantH = max(1, wantW*64/edge), max(1, wantH*64/edge)
		}
		if w != wantW || h != wantH {
			t.Fatalf("aspect changed: %v -> %dx%d", size, w, h)
		}
		im, _, err := image.Decode(bytes.NewReader(encoded))
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				_, g, _, _ := im.At(x, y).RGBA()
				if g != 0 {
					t.Fatal("hidden green from transparent pixels leaked")
				}
			}
		}
		if size[0] > 1 {
			_, _, _, alpha := im.At(w-1, h/2).RGBA()
			if alpha == 65535 {
				t.Fatal("alpha flattened", size)
			}
		}
	}
}

func TestAmbientArtworkRejectsMalformedBombsAndCancellation(t *testing.T) {
	valid := ambientTestPNG(t, 1, 1, false)
	header := func(w, h uint32) []byte {
		raw := append([]byte{}, valid...)
		binary.BigEndian.PutUint32(raw[16:20], w)
		binary.BigEndian.PutUint32(raw[20:24], h)
		binary.BigEndian.PutUint32(raw[29:33], crc32.ChecksumIEEE(raw[12:29]))
		return raw
	}
	for _, raw := range [][]byte{nil, []byte("not an image"), valid[:len(valid)/2], make([]byte, artworkMaxBytes+1), header(10001, 1), header(1, 10001), header(6000, 6000), header(0, 1)} {
		if _, _, _, err := encodeAmbientArtwork(context.Background(), raw); err == nil {
			t.Fatal("malformed/oversize source accepted")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, _, err := encodeAmbientArtwork(ctx, valid); !errors.Is(err, context.Canceled) {
		t.Fatal("encoding ignored cancellation", err)
	}
	im := image.NewRGBA(image.Rect(0, 0, 64, 64))
	if err := ambientBoxPass(ctx, im, im, true); !errors.Is(err, context.Canceled) {
		t.Fatal("blur ignored cancellation", err)
	}
}

func ambientSelectionFixture(t *testing.T) (*Service, *sql.DB, RepairTarget, string) {
	t.Helper()
	s, db, target := repairFixture(t)
	ctx := context.Background()
	state, err := s.RepairState(ctx, target, allowAll)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.UploadArtwork(ctx, target, "poster", "", state.Revision, ambientTestPNG(t, 256, 128, true), editorActor(), allowAll); err != nil {
		t.Fatal(err)
	}
	digest, _, err := s.EntityArtworkIdentity(ctx, target, "poster", "", false)
	if err != nil || digest == "" {
		t.Fatal(digest, err)
	}
	return s, db, target, digest
}

func TestAmbientArtworkCacheVersionSourceAndCleanup(t *testing.T) {
	s, db, target, source := ambientSelectionFixture(t)
	ctx := context.Background()
	read := func() ([]byte, string) {
		t.Helper()
		f, mime, err := s.EntityArtworkVariant(ctx, target, "poster", "", ArtworkAmbientEdge, source)
		if err != nil || mime != "image/png" {
			t.Fatal(mime, err)
		}
		defer f.Close()
		raw, err := io.ReadAll(f)
		if err != nil {
			t.Fatal(err)
		}
		return raw, f.Name()
	}
	first, path := read()
	want := filepath.Join(s.cacheRoot, ambientArtworkDigest(source)+".img")
	if path != want || filepath.Base(path) == source+".img" {
		t.Fatal("variant identity not separated", path)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("cache not private", err)
	}
	second, _ := read()
	info2, err := os.Stat(path)
	if err != nil || !bytes.Equal(first, second) || !info.ModTime().Equal(info2.ModTime()) {
		t.Fatal("cache was not reused", err)
	}
	var n int
	if err = db.QueryRow(`SELECT count(*) FROM artwork_objects WHERE digest=?`, ambientArtworkDigest(source)).Scan(&n); err != nil || n != 0 {
		t.Fatal("disposable ambient cache became persistent metadata", n, err)
	}
	if err = os.Remove(path); err != nil {
		t.Fatal(err)
	}
	third, _ := read()
	if !bytes.Equal(first, third) {
		t.Fatal("rebuild changed immutable bytes")
	}
	if err = os.WriteFile(path, first[:len(first)/2], 0600); err != nil {
		t.Fatal(err)
	}
	repaired, _ := read()
	if !bytes.Equal(first, repaired) {
		t.Fatal("corrupt cache was served rather than repaired")
	}
	// The ordinary bounded directory cleanup owns ambient retirement, even
	// while the source remains selected; a subsequent access derives it again.
	old := s.publicationTime().Add(-25 * time.Hour)
	if err = os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if err = s.CleanupArtwork(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("old derived cache not retired", err)
	}
	fourth, _ := read()
	if !bytes.Equal(first, fourth) {
		t.Fatal("cleanup rebuild changed bytes")
	}
	// A retained derived file cannot substitute for the exact physical source.
	if err = os.Remove(filepath.Join(s.cacheRoot, source+".img")); err != nil {
		t.Fatal(err)
	}
	if f, _, err := s.EntityArtworkVariant(ctx, target, "poster", "", ArtworkAmbientEdge, source); !errors.Is(err, ErrArtworkPending) {
		if f != nil {
			f.Close()
		}
		t.Fatal("missing source served cached ambient", err)
	}
	if f, _, err := s.EntityArtworkVariant(ctx, target, "poster", "", ArtworkAmbientEdge, "wrong-version"); !errors.Is(err, ErrArtworkVersion) {
		if f != nil {
			f.Close()
		}
		t.Fatal("stale source served cached ambient", err)
	}
}

func TestAmbientArtworkQueuedDerivationIsCancellableAndSourceBounded(t *testing.T) {
	s, _, _, source := ambientSelectionFixture(t)
	f, err := s.openArtworkObject(source)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	release, err := imagework.Acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- s.ensureAmbientArtwork(ctx, source, f) }()
	cancel()
	if err = <-result; !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
	release()
	if _, err = os.Stat(filepath.Join(s.cacheRoot, ambientArtworkDigest(source)+".img")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("cancelled work published", err)
	}
	for _, data := range [][]byte{make([]byte, artworkMaxBytes+1), []byte("corrupt source")} {
		sum := sha256.Sum256(data)
		digest := hex.EncodeToString(sum[:])
		path := filepath.Join(s.cacheRoot, digest+".img")
		if err = os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
		f, err := s.openArtworkObject(digest)
		if err != nil {
			t.Fatal(err)
		}
		err = s.ensureAmbientArtwork(context.Background(), digest, f)
		f.Close()
		if !errors.Is(err, ErrArtworkPending) {
			t.Fatal("invalid source accepted", err)
		}
	}
}
