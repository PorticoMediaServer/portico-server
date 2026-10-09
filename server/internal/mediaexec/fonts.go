package mediaexec

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Text subtitles are drawn by libass, which finds fonts through fontconfig.
// A confined decoder sees no host font directories, and a minimal host (or
// the production FFmpeg bundle's static fontconfig, whose compiled-in config
// path does not exist) has no usable configuration at all: every glyph was
// silently dropped and burned-in subtitles never appeared. Every decoder
// therefore gets a fontconfig file of Portico's own, naming the host's font
// directories (when there are any) and one bundled fallback face, DejaVu Sans
// (fonts/LICENSE), so a subtitle always has a font whatever family it names.

//go:embed fonts/DejaVuSans.ttf
var bundledFont []byte

//go:embed fonts/LICENSE
var bundledFontLicense []byte

// hostFontDirs are read-only font sources a decoder may use when present:
// the distribution's configuration, symlinked rules and fonts on Linux, the
// system's fonts on macOS. Debian/Ubuntu conf.d links target /usr/share/fontconfig.
var hostFontDirs = []string{"/etc/fonts", "/usr/share/fontconfig", "/usr/share/fonts", "/usr/local/share/fonts", "/System/Library/Fonts", "/Library/Fonts"}

// FontPack is the materialized font set: Dir holds the bundled face and
// Config (FONTCONFIG_FILE); Host lists existing font/configuration sources.
type FontPack struct {
	Dir, Config string
	Host        []string
}

var fontPackMu sync.Mutex

// fontRoot is where the pack is written (tests point it elsewhere).
var fontRoot = os.TempDir

// Fonts writes the pack once per content (a directory named by its hash) and
// rewrites it when something removed it (a temporary-file cleaner).
func Fonts() (FontPack, error) {
	fontPackMu.Lock()
	defer fontPackMu.Unlock()
	var host []string
	for _, d := range hostFontDirs {
		if info, e := os.Stat(d); e == nil && info.IsDir() {
			host = append(host, d)
		}
	}
	sum := sha256.New()
	sum.Write(bundledFont)
	sum.Write([]byte(strings.Join(host, "\n")))
	dir := filepath.Join(fontRoot(), "portico-fonts-"+hex.EncodeToString(sum.Sum(nil))[:16])
	if !filepath.IsAbs(dir) {
		return FontPack{}, errors.New("font directory is not absolute")
	}
	var conf strings.Builder
	conf.WriteString("<?xml version=\"1.0\"?>\n<!DOCTYPE fontconfig SYSTEM \"urn:fontconfig:fonts.dtd\">\n<fontconfig>\n")
	for _, d := range host {
		if d == "/usr/share/fontconfig" {
			continue // Fontconfig rule symlink targets; not a font directory.
		}
		if d == "/etc/fonts" {
			// The distribution's rules (aliases, language preferences) when it has them.
			conf.WriteString("  <include ignore_missing=\"yes\">/etc/fonts/fonts.conf</include>\n")
			continue
		}
		conf.WriteString("  <dir>" + d + "</dir>\n")
	}
	conf.WriteString("  <dir>" + dir + "</dir>\n  <cachedir>/tmp/portico-fontconfig</cachedir>\n</fontconfig>\n")
	pack := FontPack{Dir: dir, Config: filepath.Join(dir, "fonts.conf"), Host: host}
	want := map[string][]byte{"DejaVuSans.ttf": bundledFont, "LICENSE": bundledFontLicense, "fonts.conf": []byte(conf.String())}
	for name, body := range want {
		if have, e := os.ReadFile(filepath.Join(dir, name)); e == nil && bytes.Equal(have, body) {
			continue
		}
		if e := os.MkdirAll(dir, 0o755); e != nil {
			return FontPack{}, e
		}
		tmp, e := os.CreateTemp(dir, ".write-")
		if e != nil {
			return FontPack{}, e
		}
		_, werr := tmp.Write(body)
		cerr := tmp.Close()
		if werr == nil {
			werr = cerr
		}
		if werr == nil {
			werr = os.Chmod(tmp.Name(), 0o644)
		}
		if werr == nil {
			werr = os.Rename(tmp.Name(), filepath.Join(dir, name))
		}
		if werr != nil {
			_ = os.Remove(tmp.Name())
			return FontPack{}, werr
		}
	}
	return pack, nil
}
