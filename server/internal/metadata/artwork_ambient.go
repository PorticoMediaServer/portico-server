package metadata

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"io"
	"os"
	"path/filepath"

	"portico.local/server/internal/dirsync"
	"portico.local/server/internal/diskspace"
	"portico.local/server/internal/imagework"
)

// ArtworkAmbientEdge is an explicit preblurred representation, never a
// foreground width bucket. HTTP selects it only with size=ambient-v1.
const ArtworkAmbientEdge = 64
const artworkAmbientBytes = 20_000
const artworkAmbientRevision = "ambient-v1"

// The domain and algorithm revision separate these disposable derived files
// from content-addressed foreground objects. The URL carries the same revision;
// changing the algorithm requires a new size value and cache identity.
func ambientArtworkDigest(source string) string {
	sum := sha256.Sum256([]byte("portico-artwork:" + artworkAmbientRevision + ":" + source))
	return hex.EncodeToString(sum[:])
}

func (s *Service) openAmbientArtwork(ctx context.Context, source string) (*os.File, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f, err := s.openArtworkObject(ambientArtworkDigest(source))
	if err != nil {
		return nil, ErrArtworkPending
	}
	info, err := f.Stat()
	if err != nil || info.Size() > artworkAmbientBytes {
		f.Close()
		return nil, ErrArtworkPending
	}
	cfg, format, err := image.DecodeConfig(ambientContextReader{ctx, io.LimitReader(f, artworkAmbientBytes+1)})
	if err != nil || format != "png" || cfg.Width < 1 || cfg.Height < 1 || max(cfg.Width, cfg.Height) > ArtworkAmbientEdge {
		f.Close()
		return nil, ErrArtworkPending
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	// Atomic publication avoids partial files; validate a retained cache object
	// too, so truncated/corrupt disk bytes are rebuilt rather than made a 304.
	if _, err = png.Decode(ambientContextReader{ctx, io.LimitReader(f, artworkAmbientBytes+1)}); err != nil {
		f.Close()
		return nil, ErrArtworkPending
	}
	if _, err = f.Seek(0, io.SeekStart); err != nil {
		f.Close()
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func (s *Service) ensureAmbientArtwork(ctx context.Context, source string, f *os.File) error {
	release, err := imagework.Acquire(ctx)
	if err != nil {
		return err
	}
	defer release()
	// Concurrent requests share the decoder gate and reuse the first complete
	// publication rather than each retaining a full decoded source.
	if cached, err := s.openAmbientArtwork(ctx, source); err == nil {
		return cached.Close()
	}
	info, err := f.Stat()
	if err != nil || info.Size() < 1 || info.Size() > artworkMaxBytes {
		return ErrArtworkPending
	}
	raw, err := io.ReadAll(ambientContextReader{ctx, io.LimitReader(f, artworkMaxBytes+1)})
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	if len(raw) > artworkMaxBytes || hex.EncodeToString(sum[:]) != source {
		return ErrArtworkPending
	}
	encoded, _, _, err := encodeAmbientArtwork(ctx, raw)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrArtworkPending
	}
	release()
	if err = ctx.Err(); err != nil {
		return err
	}
	if !diskspace.Room(s.cacheRoot, diskspace.ProducerFloor) {
		return ErrArtworkPending
	}
	// The same stage prefix and .img namespace as ordinary artwork allow the
	// existing finite-batch orphan/tomb cleanup to retire these after 24h. No
	// persistent schema/index or additional directory walk is needed.
	stage, err := os.CreateTemp(s.cacheRoot, "stage-")
	if err != nil {
		return ErrArtworkPending
	}
	defer os.Remove(stage.Name())
	if _, err = stage.Write(encoded); err == nil {
		err = stage.Sync()
	}
	closeErr := stage.Close()
	if err != nil {
		return ErrArtworkPending
	}
	if closeErr != nil {
		return ErrArtworkPending
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = os.Rename(stage.Name(), filepath.Join(s.cacheRoot, ambientArtworkDigest(source)+".img")); err != nil {
		return ErrArtworkPending
	}
	if err = dirsync.Sync(s.cacheRoot); err != nil {
		return ErrArtworkPending
	}
	return nil
}

type ambientContextReader struct {
	ctx context.Context
	r   io.Reader
}

func (r ambientContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.r.Read(p)
}

// Input is an already ingested display object. Bounds are checked before
// allocating decoded pixels, and the caller holds the shared imagework gate.
func encodeAmbientArtwork(ctx context.Context, raw []byte) ([]byte, int, int, error) {
	if err := ctx.Err(); err != nil {
		return nil, 0, 0, err
	}
	if len(raw) < 1 || len(raw) > artworkMaxBytes {
		return nil, 0, 0, ErrRepairInput
	}
	cfg, format, err := image.DecodeConfig(ambientContextReader{ctx, bytes.NewReader(raw)})
	if ctx.Err() != nil {
		return nil, 0, 0, ctx.Err()
	}
	if err != nil || (format != "jpeg" && format != "png") || cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 10000 || cfg.Height > 10000 || int64(cfg.Width)*int64(cfg.Height) > 24000000 {
		return nil, 0, 0, ErrRepairInput
	}
	decoded, _, err := image.Decode(ambientContextReader{ctx, bytes.NewReader(raw)})
	if err != nil {
		return nil, 0, 0, err
	}
	if decoded.Bounds().Dx() != cfg.Width || decoded.Bounds().Dy() != cfg.Height {
		return nil, 0, 0, ErrRepairInput
	}
	w, h := cfg.Width, cfg.Height
	if edge := max(w, h); edge > ArtworkAmbientEdge {
		w, h = max(1, w*ArtworkAmbientEdge/edge), max(1, h*ArtworkAmbientEdge/edge)
	}
	small := image.NewNRGBA(image.Rect(0, 0, w, h))
	if err = imagework.CatmullRom(ctx, small, decoded); err != nil {
		return nil, 0, 0, err
	}
	// Premultiplied RGBA prevents hidden RGB in transparent pixels bleeding
	// into the glow. Clamp edges, preserve geometry/alpha, never add a canvas.
	a := image.NewRGBA(small.Bounds())
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a.Set(x, y, small.At(x, y))
		}
	}
	b := image.NewRGBA(a.Bounds())
	for pass := 0; pass < 3; pass++ {
		if err = ambientBoxPass(ctx, b, a, true); err != nil {
			return nil, 0, 0, err
		}
		if err = ambientBoxPass(ctx, a, b, false); err != nil {
			return nil, 0, 0, err
		}
	}
	var out bytes.Buffer
	if err = png.Encode(&out, a); err != nil {
		return nil, 0, 0, err
	}
	if err = ctx.Err(); err != nil {
		return nil, 0, 0, err
	}
	if out.Len() > artworkAmbientBytes {
		return nil, 0, 0, ErrRepairInput
	}
	return out.Bytes(), w, h, nil
}

func ambientBoxPass(ctx context.Context, dst, src *image.RGBA, horizontal bool) error {
	const radius = 4
	for y := 0; y < src.Rect.Dy(); y++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		for x := 0; x < src.Rect.Dx(); x++ {
			var r, g, b, a uint32
			for offset := -radius; offset <= radius; offset++ {
				sx, sy := x, y
				if horizontal {
					sx = min(src.Rect.Dx()-1, max(0, x+offset))
				} else {
					sy = min(src.Rect.Dy()-1, max(0, y+offset))
				}
				c := src.RGBAAt(sx, sy)
				r, g, b, a = r+uint32(c.R), g+uint32(c.G), b+uint32(c.B), a+uint32(c.A)
			}
			const n = 2*radius + 1
			dst.SetRGBA(x, y, color.RGBA{uint8((r + n/2) / n), uint8((g + n/2) / n), uint8((b + n/2) / n), uint8((a + n/2) / n)})
		}
	}
	return nil
}
