package linearinput

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// rewrite parses URI-bearing HLS constructs, including alternate audio and
// encryption keys. Unknown URI constructs and low-latency extensions are rejected
// rather than letting FFmpeg resolve an unreviewed source path itself.
func (g *Gateway) rewrite(data []byte, base string, depth int) ([]byte, error) {
	if len(data) > maxManifest || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, ErrFormat
	}
	text := strings.TrimPrefix(string(data), "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if strings.Contains(text, "\r") {
		return nil, ErrFormat
	}
	lines := strings.Split(text, "\n")
	if len(lines) > 20000 || len(lines) == 0 || strings.TrimSpace(lines[0]) != "#EXTM3U" {
		return nil, ErrFormat
	}
	variant := false
	for i, line := range lines {
		if len(line) > 16384 {
			return nil, ErrFormat
		}
		line = strings.TrimSpace(line)
		lines[i] = line
		if line == "" {
			continue
		}
		if line[0] != '#' {
			kind := "media"
			if variant {
				kind = "manifest"
			}
			uri, e := g.child(line, base, kind, depth+1)
			if e != nil {
				return nil, e
			}
			lines[i] = uri
			variant = false
			continue
		}
		tag, attrs, has := strings.Cut(line, ":")
		switch tag {
		case "#EXT-X-STREAM-INF":
			if variant {
				return nil, ErrFormat
			}
			variant = true
		case "#EXT-X-PART", "#EXT-X-PRELOAD-HINT", "#EXT-X-RENDITION-REPORT", "#EXT-X-DEFINE", "#EXT-X-CONTENT-STEERING", "#EXT-X-SESSION-DATA":
			return nil, ErrFormat
		case "#EXT-X-MEDIA", "#EXT-X-I-FRAME-STREAM-INF", "#EXT-X-KEY", "#EXT-X-SESSION-KEY", "#EXT-X-MAP":
			if !has {
				return nil, ErrFormat
			}
			a, e := parseAttributes(attrs)
			if e != nil {
				return nil, e
			}
			kind := "manifest"
			if tag == "#EXT-X-MAP" {
				kind = "media"
			}
			if tag == "#EXT-X-KEY" || tag == "#EXT-X-SESSION-KEY" {
				method := attribute(a, "METHOD")
				if method == "NONE" {
					if attribute(a, "URI") != "" {
						return nil, ErrFormat
					}
					continue
				}
				if method != "AES-128" || (attribute(a, "KEYFORMAT") != "" && attribute(a, "KEYFORMAT") != "identity") {
					return nil, ErrFormat
				}
				kind = "key"
			}
			changed := false
			for n, p := range a {
				if p.key == "URI" {
					uri, e := g.child(p.value, base, kind, depth+1)
					if e != nil {
						return nil, e
					}
					a[n].value = uri
					a[n].quoted = true
					changed = true
				}
			}
			if !changed && tag != "#EXT-X-MEDIA" {
				return nil, ErrFormat
			}
			lines[i] = tag + ":" + formatAttributes(a)
		default:
			// Reject extension URI/path fields, even when the current FFmpeg build might
			// ignore them. Future builds must not silently widen the network boundary.
			if strings.Contains(strings.ToUpper(line), "URI=") || strings.Contains(strings.ToUpper(line), "URL=") {
				return nil, ErrFormat
			}
		}
	}
	if variant {
		return nil, ErrFormat
	}
	return []byte(strings.Join(lines, "\n")), nil
}
