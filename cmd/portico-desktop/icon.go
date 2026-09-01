package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/png"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// Keep the fill explicit. oksvg does not apply the browser's implicit black
// SVG fill, and an omitted fill therefore produces a valid but fully
// transparent PNG. macOS template icons use this alpha mask and recolor it for
// the active menu-bar appearance; Windows uses the opaque black glyph.
const playPSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 304 405" fill="#000000"><g transform="translate(-206 -184)"><path fill="#000000" d="M228 567V206h126.353q41.348 0 71.027 15.251 29.679 15.252 45.649 43.934Q487 293.868 487 333.639q0 39.27-15.971 67.945-15.97 28.675-45.649 43.934-29.679 15.259-71.027 15.259H324v-52.313h28.414q40.267 0 60.823-19.605 20.556-19.605 20.556-55.22 0-36.116-20.556-55.721-20.556-19.605-60.823-19.605h-71.669v150.151H281v52.313h-.255V567Z"/><path fill="#000000" d="M324 298v71q0 5 5 2l63-33q6-5 0-10l-63-32q-5-3-5 2Z"/></g></svg>`

func playPIconPNG() []byte {
	icon, err := oksvg.ReadIconStream(strings.NewReader(playPSVG), oksvg.StrictErrorMode)
	if err != nil {
		return nil
	}
	const size = 64
	canvas := image.NewRGBA(image.Rect(0, 0, size, size))
	icon.SetTarget(12, 4, 40, 56)
	icon.Draw(rasterx.NewDasher(size, size, rasterx.NewScannerGV(size, size, canvas, canvas.Bounds())), 1)
	var encoded bytes.Buffer
	if png.Encode(&encoded, canvas) != nil {
		return nil
	}
	return encoded.Bytes()
}

// Windows supports PNG-compressed images inside an ICO container. Keeping the
// same official monochrome glyph in both formats avoids maintaining divergent
// hand-rasterized tray artwork.
func pngAsICO(pngBytes []byte) []byte {
	if len(pngBytes) == 0 {
		return nil
	}
	var encoded bytes.Buffer
	_ = binary.Write(&encoded, binary.LittleEndian, uint16(0))
	_ = binary.Write(&encoded, binary.LittleEndian, uint16(1))
	_ = binary.Write(&encoded, binary.LittleEndian, uint16(1))
	encoded.WriteByte(64)
	encoded.WriteByte(64)
	encoded.WriteByte(0)
	encoded.WriteByte(0)
	_ = binary.Write(&encoded, binary.LittleEndian, uint16(1))
	_ = binary.Write(&encoded, binary.LittleEndian, uint16(32))
	_ = binary.Write(&encoded, binary.LittleEndian, uint32(len(pngBytes)))
	_ = binary.Write(&encoded, binary.LittleEndian, uint32(22))
	encoded.Write(pngBytes)
	return encoded.Bytes()
}
