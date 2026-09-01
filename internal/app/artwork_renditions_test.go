package app

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/config"
)

func TestArtworkRenditionsPreserveEfficientOriginalAndPrepareSmall(t *testing.T) {
	appDataDir := t.TempDir()
	sourcePath := writeArtworkRenditionTestPNG(t, appDataDir, 800, 1200, false)
	server := &Server{cfg: config.Config{AppDataDir: appDataDir}}
	if err := server.prepareArtworkRenditions(sourcePath, "poster"); err != nil {
		t.Fatal(err)
	}
	largePath, ok := server.preparedArtworkRenditionPath(sourcePath, "poster", artworkRenditionLarge)
	if !ok || largePath != sourcePath {
		t.Fatalf("efficient provider original large path=%q ok=%v, want original %q", largePath, ok, sourcePath)
	}
	smallPath, ok := server.preparedArtworkRenditionPath(sourcePath, "poster", artworkRenditionSmall)
	if !ok || smallPath == sourcePath {
		t.Fatalf("small rendition path=%q ok=%v", smallPath, ok)
	}
	file, err := os.Open(smallPath)
	if err != nil {
		t.Fatal(err)
	}
	config, format, err := image.DecodeConfig(file)
	_ = file.Close()
	if err != nil || format != "jpeg" || config.Width != 480 || config.Height != 720 {
		t.Fatalf("small rendition format=%q dimensions=%dx%d err=%v", format, config.Width, config.Height, err)
	}
}

func TestArtworkRenditionsNormalizeLargeFileWithoutUpscaling(t *testing.T) {
	appDataDir := t.TempDir()
	sourcePath := writeArtworkRenditionTestPNG(t, appDataDir, 640, 960, true)
	server := &Server{cfg: config.Config{AppDataDir: appDataDir}}
	if err := server.prepareArtworkRenditions(sourcePath, "poster"); err != nil {
		t.Fatal(err)
	}
	largePath, ok := server.preparedArtworkRenditionPath(sourcePath, "poster", artworkRenditionLarge)
	if !ok || largePath == sourcePath {
		t.Fatalf("oversized original large path=%q ok=%v", largePath, ok)
	}
	info, err := os.Stat(largePath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() >= artworkLargeFileLimit {
		t.Fatalf("normalized large rendition bytes=%d, limit=%d", info.Size(), artworkLargeFileLimit)
	}
	file, err := os.Open(largePath)
	if err != nil {
		t.Fatal(err)
	}
	config, format, err := image.DecodeConfig(file)
	_ = file.Close()
	if err != nil || format != "jpeg" || config.Width != 640 || config.Height != 960 {
		t.Fatalf("normalized large rendition format=%q dimensions=%dx%d err=%v", format, config.Width, config.Height, err)
	}
}

func writeArtworkRenditionTestPNG(t *testing.T, dir string, width, height int, pad bool) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 251), G: uint8(y % 241), B: uint8((x + y) % 239), A: 255})
		}
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, img); err != nil {
		t.Fatal(err)
	}
	if pad && encoded.Len() < artworkLargeFileLimit {
		encoded.Write(make([]byte, artworkLargeFileLimit-encoded.Len()+1))
	}
	path := filepath.Join(dir, "source.png")
	if err := os.WriteFile(path, encoded.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
