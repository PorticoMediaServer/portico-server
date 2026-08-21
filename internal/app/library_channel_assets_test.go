package app

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/PorticoMediaServer/portico-server/internal/librarychannels"
)

func TestNormalizeLibraryChannelLogoRasterizesSafeCustomSVG(t *testing.T) {
	normalized, err := normalizeLibraryChannelLogo([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="96" height="54" viewBox="0 0 96 54"><rect width="96" height="54" rx="8" fill="#071019"/><path d="M12 16h30v22H12z" fill="#39BDF8"/></svg>`), "custom.svg")
	if err != nil {
		t.Fatalf("rasterize safe custom SVG: %v", err)
	}
	if normalized.MIMEType != "image/png" || normalized.Ext != "png" || normalized.Width != 96 || normalized.Height != 54 || len(normalized.Data) == 0 {
		t.Fatalf("normalized SVG = %#v", normalized)
	}
}

func TestNormalizeLibraryChannelLogoRejectsUnsafeCustomSVG(t *testing.T) {
	cases := map[string]string{
		"script":        `<svg width="10" height="10"><script>alert(1)</script></svg>`,
		"event":         `<svg width="10" height="10" onload="alert(1)"><path d="M0 0"/></svg>`,
		"external-ref":  `<svg width="10" height="10"><image href="https://attacker.example/x"/></svg>`,
		"css-url":       `<svg width="10" height="10"><path d="M0 0" fill="url(https://attacker.example/x)"/></svg>`,
		"doctype":       `<!DOCTYPE svg [<!ENTITY xxe SYSTEM "file:///etc/passwd">]><svg width="10" height="10">&xxe;</svg>`,
		"embedded-text": `<svg width="10" height="10"><text x="1" y="1">unsafe font surface</text></svg>`,
	}
	for name, input := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := normalizeLibraryChannelLogo([]byte(input), "custom.svg"); err == nil {
				t.Fatal("expected custom SVG to be rejected at the raster boundary")
			}
		})
	}
}

func TestNormalizeLibraryChannelLogoAcceptsSafeRasterTypesAndStoresPNG(t *testing.T) {
	var encodedPNG bytes.Buffer
	if err := png.Encode(&encodedPNG, image.NewRGBA(image.Rect(0, 0, 16, 9))); err != nil {
		t.Fatal(err)
	}
	webpData, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"logo.png": encodedPNG.Bytes(), "logo.webp": webpData} {
		t.Run(name, func(t *testing.T) {
			normalized, err := normalizeLibraryChannelLogo(data, name)
			if err != nil {
				t.Fatalf("normalize safe raster: %v", err)
			}
			if normalized.MIMEType != "image/png" || normalized.Ext != "png" || normalized.Width < 1 || normalized.Height < 1 || len(normalized.Data) == 0 {
				t.Fatalf("normalized raster = %#v", normalized)
			}
		})
	}
}

func TestNormalizeLibraryChannelLogoRejectsOversizedAndDecompressionDimensions(t *testing.T) {
	if _, err := normalizeLibraryChannelLogo(bytes.Repeat([]byte("x"), libraryChannelLogoUploadLimit+1), "large.png"); err == nil {
		t.Fatal("expected oversized upload rejection")
	}
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, libraryChannelLogoMaxEdge+1, 1))); err != nil {
		t.Fatal(err)
	}
	if _, err := normalizeLibraryChannelLogo(encoded.Bytes(), "wide.png"); err == nil {
		t.Fatal("expected decoded dimensions to be bounded")
	}
	// Rewrite only the IHDR dimensions and checksum. DecodeConfig must reject
	// this hostile allocation claim before a pixel buffer is constructed.
	hostilePNG := append([]byte(nil), encoded.Bytes()...)
	binary.BigEndian.PutUint32(hostilePNG[16:20], 100_000)
	binary.BigEndian.PutUint32(hostilePNG[20:24], 100_000)
	binary.BigEndian.PutUint32(hostilePNG[29:33], crc32.ChecksumIEEE(hostilePNG[12:29]))
	if _, err := normalizeLibraryChannelLogo(hostilePNG, "hostile.png"); err == nil {
		t.Fatal("expected hostile PNG DecodeConfig dimensions to be rejected")
	}
	if _, err := normalizeLibraryChannelLogo(hostileWebPConfig(100_000, 100_000), "hostile.webp"); err == nil {
		t.Fatal("expected hostile WebP DecodeConfig dimensions to be rejected")
	}
}

func hostileWebPConfig(width, height int) []byte {
	data := make([]byte, 30)
	copy(data[0:4], "RIFF")
	binary.LittleEndian.PutUint32(data[4:8], uint32(len(data)-8))
	copy(data[8:12], "WEBP")
	copy(data[12:16], "VP8X")
	binary.LittleEndian.PutUint32(data[16:20], 10)
	width--
	height--
	data[24], data[25], data[26] = byte(width), byte(width>>8), byte(width>>16)
	data[27], data[28], data[29] = byte(height), byte(height>>8), byte(height>>16)
	return data
}

func TestLibraryChannelAssetIDsRejectTraversal(t *testing.T) {
	for _, value := range []string{"../secret", `..\\secret`, "lca_/secret", "lca_.."} {
		if validLibraryChannelAssetID(value) {
			t.Fatalf("accepted traversal-shaped asset id %q", value)
		}
	}
}

func TestEveryBuiltInLibraryChannelTemplateHasSafePackagedLogo(t *testing.T) {
	for _, template := range librarychannels.BuiltInChannelTemplates() {
		ref := builtInLibraryChannelLogoRef(template.Key)
		data, ok := builtInLibraryChannelLogo(ref)
		if !ok || len(data) == 0 {
			t.Fatalf("template %q has no built-in logo", template.Key)
		}
		lower := strings.ToLower(string(data))
		lower = strings.ReplaceAll(lower, `xmlns="http://www.w3.org/2000/svg"`, "")
		for _, forbidden := range []string{"<script", "javascript:", "http://", "https://", "url("} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("template %q logo contains %q", template.Key, forbidden)
			}
		}
	}
}
