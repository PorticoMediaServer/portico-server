package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	xdraw "golang.org/x/image/draw"
)

type artworkRendition string

const (
	artworkRenditionSmall artworkRendition = "small"
	artworkRenditionLarge artworkRendition = "large"

	// Artwork originals below this bound are already an efficient large
	// rendition. Larger files are normalized before they become visible to a
	// client. The original remains private so future rendition policy changes do
	// not compound a previous lossy encode.
	artworkLargeFileLimit   = 1_000_000
	artworkRenditionVersion = "portico-artwork-rendition-v1"
)

type artworkRenditionSpec struct {
	maxWidth      int
	maxHeight     int
	preserveAlpha bool
}

func transparentArtworkKind(kind string) bool {
	kind = strings.ToLower(strings.TrimSpace(kind))
	return kind == "logo" || kind == "clearart" || kind == "clear-art" || kind == "clearlogo"
}

// Provider portrait ingestion uses a person-<identity> bookkeeping kind so
// independently sourced files cannot collide. Rendition policy and public
// serving deliberately use the stable visual class instead; the source path
// already supplies file identity to the cache key.
func artworkRenditionKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if strings.HasPrefix(kind, "person-") {
		return "person"
	}
	return kind
}

func parseArtworkRendition(value string) (artworkRendition, error) {
	switch artworkRendition(strings.ToLower(strings.TrimSpace(value))) {
	case "", artworkRenditionLarge:
		return artworkRenditionLarge, nil
	case artworkRenditionSmall:
		return artworkRenditionSmall, nil
	default:
		return "", errors.New("rendition must be small or large")
	}
}

func artworkRenditionSpecFor(kind string, rendition artworkRendition) artworkRenditionSpec {
	kind = strings.ToLower(strings.TrimSpace(kind))
	transparent := transparentArtworkKind(kind)
	landscape := kind == "backdrop" || kind == "thumb" || kind == "banner" || kind == "still" || transparent
	square := strings.HasPrefix(kind, "person-") || kind == "person" || kind == "avatar" || kind == "disc"
	if rendition == artworkRenditionSmall {
		switch {
		case landscape:
			return artworkRenditionSpec{maxWidth: 960, maxHeight: 540, preserveAlpha: transparent}
		case square:
			return artworkRenditionSpec{maxWidth: 384, maxHeight: 384}
		default:
			return artworkRenditionSpec{maxWidth: 480, maxHeight: 720}
		}
	}
	switch {
	case landscape:
		return artworkRenditionSpec{maxWidth: 1920, maxHeight: 1080, preserveAlpha: transparent}
	case square:
		return artworkRenditionSpec{maxWidth: 1000, maxHeight: 1000}
	default:
		return artworkRenditionSpec{maxWidth: 1000, maxHeight: 1500}
	}
}

// prepareArtworkRenditions performs every decode and encode before artwork is
// published in the catalog. Interactive artwork requests only resolve and
// stream one of these files.
func (s *Server) prepareArtworkRenditions(sourcePath, kind string) error {
	if _, err := os.Stat(sourcePath); err != nil {
		return err
	}
	if err := s.ensureArtworkRendition(sourcePath, kind, artworkRenditionSmall); err != nil {
		return err
	}
	info, err := os.Stat(sourcePath)
	if err != nil {
		return err
	}
	if info.Size() >= artworkLargeFileLimit {
		if err := s.ensureArtworkRendition(sourcePath, kind, artworkRenditionLarge); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) preparedArtworkRenditionPath(sourcePath, kind string, rendition artworkRendition) (string, bool) {
	info, err := os.Stat(sourcePath)
	if err != nil || info.IsDir() {
		return "", false
	}
	if rendition == artworkRenditionLarge && info.Size() < artworkLargeFileLimit {
		return sourcePath, true
	}
	cachePath, ok := s.artworkRenditionCachePath(sourcePath, kind, rendition)
	if !ok || !localArtworkFileExists(cachePath) {
		return "", false
	}
	return cachePath, true
}

func (s *Server) ensureArtworkRendition(sourcePath, kind string, rendition artworkRendition) error {
	cachePath, ok := s.artworkRenditionCachePath(sourcePath, kind, rendition)
	if !ok {
		return errors.New("artwork source is unavailable")
	}
	if localArtworkFileExists(cachePath) {
		return nil
	}
	return writeArtworkRendition(sourcePath, cachePath, artworkRenditionSpecFor(kind, rendition))
}

func (s *Server) artworkRenditionCachePath(sourcePath, kind string, rendition artworkRendition) (string, bool) {
	if resolved, err := filepath.EvalSymlinks(sourcePath); err == nil {
		sourcePath = resolved
	}
	info, err := os.Stat(sourcePath)
	if err != nil || info.IsDir() {
		return "", false
	}
	seed := strings.Join([]string{
		artworkRenditionVersion,
		filepath.Clean(sourcePath),
		strconv.FormatInt(info.ModTime().UnixNano(), 10),
		strconv.FormatInt(info.Size(), 10),
		artworkRenditionKind(kind),
		string(rendition),
	}, "\x00")
	sum := sha256.Sum256([]byte(seed))
	extension := ".jpg"
	if transparentArtworkKind(kind) {
		extension = ".png"
	}
	return filepath.Join(s.cfg.AppDataDir, "image-cache", "artwork-renditions", hex.EncodeToString(sum[:])+extension), true
}

func writeArtworkRendition(sourcePath, cachePath string, spec artworkRenditionSpec) error {
	file, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	source, _, err := image.Decode(file)
	_ = file.Close()
	if err != nil {
		return err
	}
	bounds := source.Bounds()
	width, height := fittedArtworkDimensions(bounds.Dx(), bounds.Dy(), spec.maxWidth, spec.maxHeight)
	if width <= 0 || height <= 0 {
		return errors.New("artwork dimensions are invalid")
	}
	var encoded bytes.Buffer
	if spec.preserveAlpha {
		for {
			target := image.NewNRGBA(image.Rect(0, 0, width, height))
			xdraw.CatmullRom.Scale(target, target.Bounds(), source, bounds, xdraw.Src, nil)
			encoded.Reset()
			encoder := png.Encoder{CompressionLevel: png.BestCompression}
			if err := encoder.Encode(&encoded, target); err != nil {
				return err
			}
			if encoded.Len() < artworkLargeFileLimit {
				break
			}
			if width == 1 && height == 1 {
				return errors.New("normalized transparent artwork exceeds the one megabyte limit")
			}
			width = max(1, width*4/5)
			height = max(1, height*4/5)
		}
	} else {
		target := image.NewRGBA(image.Rect(0, 0, width, height))
		xdraw.CatmullRom.Scale(target, target.Bounds(), source, bounds, xdraw.Src, nil)
		for _, quality := range []int{88, 84, 80, 76, 72} {
			encoded.Reset()
			if err := jpeg.Encode(&encoded, target, &jpeg.Options{Quality: quality}); err != nil {
				return err
			}
			if encoded.Len() < artworkLargeFileLimit {
				break
			}
		}
	}
	if encoded.Len() >= artworkLargeFileLimit {
		return errors.New("normalized artwork exceeds the one megabyte limit")
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(cachePath), ".artwork-rendition-*"+filepath.Ext(cachePath))
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	removeTemp := func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}
	if _, err := temp.Write(encoded.Bytes()); err != nil {
		removeTemp()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Chmod(tempPath, 0o600); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, cachePath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func fittedArtworkDimensions(sourceWidth, sourceHeight, maxWidth, maxHeight int) (int, int) {
	if sourceWidth <= 0 || sourceHeight <= 0 || maxWidth <= 0 || maxHeight <= 0 {
		return 0, 0
	}
	if sourceWidth <= maxWidth && sourceHeight <= maxHeight {
		return sourceWidth, sourceHeight
	}
	scale := float64(maxWidth) / float64(sourceWidth)
	if heightScale := float64(maxHeight) / float64(sourceHeight); heightScale < scale {
		scale = heightScale
	}
	return max(1, int(float64(sourceWidth)*scale+0.5)), max(1, int(float64(sourceHeight)*scale+0.5))
}
