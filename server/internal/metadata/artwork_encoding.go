package metadata

import (
	"bytes"
	"errors"
	"image"
	"image/jpeg"
	"image/png"

	"golang.org/x/image/draw"
)

const artworkDisplayBytes = 500_000
const artworkLargeEdge = 1920
const artworkSmallEdge = 400

// Display images preserve aspect, never upscale, and have a hard encoded-byte
// ceiling. Detail is reduced before JPEG quality would drop below 75. Alpha
// remains lossless PNG; noisy transparent inputs reduce dimensions instead.
func encodeDisplayArtwork(source image.Image, edge int, preferJPEG bool) ([]byte, int, int, error) {
	bounds := source.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w < 1 || h < 1 {
		return nil, 0, 0, errors.New("dimension_limit")
	}
	if max(w, h) > edge {
		w = max(1, w*edge/max(bounds.Dx(), bounds.Dy()))
		h = max(1, h*edge/max(bounds.Dx(), bounds.Dy()))
	}
	for {
		scaled := image.NewNRGBA(image.Rect(0, 0, w, h))
		draw.CatmullRom.Scale(scaled, scaled.Bounds(), source, bounds, draw.Src, nil)
		var b bytes.Buffer
		opaque := scaled.Opaque()
		if !preferJPEG || !opaque {
			encoder := png.Encoder{CompressionLevel: png.DefaultCompression}
			if err := encoder.Encode(&b, scaled); err != nil {
				return nil, 0, 0, err
			}
			if b.Len() <= artworkDisplayBytes {
				return b.Bytes(), w, h, nil
			}
		}
		if opaque {
			for _, quality := range []int{90, 82, 75} {
				b.Reset()
				if err := jpeg.Encode(&b, scaled, &jpeg.Options{Quality: quality}); err != nil {
					return nil, 0, 0, err
				}
				if b.Len() <= artworkDisplayBytes {
					return b.Bytes(), w, h, nil
				}
			}
		}
		if w == 1 && h == 1 {
			return nil, 0, 0, errors.New("normalized_size_limit")
		}
		next := max(1, max(w, h)*4/5)
		w = max(1, bounds.Dx()*next/max(bounds.Dx(), bounds.Dy()))
		h = max(1, bounds.Dy()*next/max(bounds.Dx(), bounds.Dy()))
	}
}
