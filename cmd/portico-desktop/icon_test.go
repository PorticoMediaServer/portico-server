package main

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestPlayPIconHasVisibleTemplateMask(t *testing.T) {
	decoded, err := png.Decode(bytes.NewReader(playPIconPNG()))
	if err != nil {
		t.Fatalf("decode icon: %v", err)
	}

	var opaque int
	bounds := decoded.Bounds()
	opaqueBounds := image.Rectangle{Min: image.Pt(bounds.Max.X, bounds.Max.Y)}
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			red, green, blue, alpha := decoded.At(x, y).RGBA()
			if alpha > 0 && (red != 0 || green != 0 || blue != 0) {
				t.Fatalf("template mask contains a non-black source pixel at %d,%d", x, y)
			}
			if alpha >= 0x8000 {
				opaque++
				if x < opaqueBounds.Min.X {
					opaqueBounds.Min.X = x
				}
				if y < opaqueBounds.Min.Y {
					opaqueBounds.Min.Y = y
				}
				if x+1 > opaqueBounds.Max.X {
					opaqueBounds.Max.X = x + 1
				}
				if y+1 > opaqueBounds.Max.Y {
					opaqueBounds.Max.Y = y + 1
				}
			}
		}
	}
	if opaque < 400 {
		t.Fatalf("icon mask is effectively transparent: %d opaque pixels", opaque)
	}
	if opaqueBounds.Min.X < 8 || opaqueBounds.Min.Y < 2 || opaqueBounds.Max.X > 56 || opaqueBounds.Max.Y > 62 {
		t.Fatalf("icon mask exceeds its menu-bar-safe inset: %v", opaqueBounds)
	}
	if opaqueBounds.Dx() < 24 || opaqueBounds.Dy() < 48 {
		t.Fatalf("icon mask is too small to remain legible: %v", opaqueBounds)
	}
}

func TestWindowsIconWrapsVisiblePNG(t *testing.T) {
	pngBytes := playPIconPNG()
	ico := pngAsICO(pngBytes)
	if len(ico) != 22+len(pngBytes) {
		t.Fatalf("unexpected ICO size: got %d, want %d", len(ico), 22+len(pngBytes))
	}
	if !bytes.Equal(ico[22:], pngBytes) {
		t.Fatal("ICO does not contain the tested tray PNG")
	}
}
