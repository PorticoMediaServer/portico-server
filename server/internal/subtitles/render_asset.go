package subtitles

// Render assets are immutable, server-owned inputs to a confined producer. They
// are never interpreted as HTML or delivered to a native text overlay. Bitmap
// data is preserved, not OCR'd. Filenames below are generated, never user paths.
import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"html"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const MaxBinaryBytes = 32 << 20
const MaxAssetBytes = 48 << 20

type RenderAsset struct {
	Version    int    `json:"version"`
	TimeDomain string `json:"timeDomain"`
	Renderer   string `json:"renderer"`
	Format     string `json:"format"`
	Data       []byte `json:"data"`
	Companion  []byte `json:"companion,omitempty"`
}

func DecodeRenderAsset(raw []byte) (RenderAsset, error) {
	var a RenderAsset
	if len(raw) > MaxAssetBytes || json.Unmarshal(raw, &a) != nil || a.Version != 2 || a.TimeDomain != "source-relative" || a.Renderer != "burn_in" || len(a.Data) == 0 {
		return a, ErrInput
	}
	switch a.Format {
	case "ass", "ssa", "pgs", "vobsub", "mks":
	default:
		return a, ErrUnsupported
	}
	return a, nil
}
func assetJSON(format string, data, companion []byte) ([]byte, error) {
	if len(data)+len(companion) > MaxBinaryBytes {
		return nil, ErrCapacity
	}
	return json.Marshal(RenderAsset{2, "source-relative", "burn_in", format, data, companion})
}
func assetRenderer(raw []byte) string {
	var h struct {
		Version int `json:"version"`
	}
	if json.Unmarshal(raw, &h) == nil && h.Version == 2 {
		return "burn_in"
	}
	return "external_text"
}

// CanonicalAsset supports text and bitmap uploads. The optional companion is
// the .sub packet file belonging to a VobSub .idx; neither file names another
// file. Matroska subtitle-only capsules are accepted only from our extractor.
func CanonicalAsset(raw, companion []byte, format string, durationUS int64) ([]byte, error) {
	switch format {
	case "pgs", "sup":
		if len(companion) != 0 {
			return nil, ErrInput
		}
		if e := validatePGS(raw); e != nil {
			return nil, e
		}
		return assetJSON("pgs", raw, nil)
	case "vobsub", "idx":
		idx, e := DecodeText(raw)
		if e != nil {
			return nil, e
		}
		if len(companion) < 4 || len(companion) > MaxBinaryBytes || !bytes.Equal(companion[:4], []byte{0, 0, 1, 0xba}) {
			return nil, ErrText
		}
		if !bytes.Contains(idx, []byte("VobSub index file")) || !bytes.Contains(idx, []byte("timestamp:")) || bytes.Contains(idx, []byte{0}) {
			return nil, ErrText
		}
		// VobSub's parser has no external references besides the generated .sub
		// basename. Do not accept undocumented file/include directives.
		for _, l := range strings.Split(string(idx), "\n") {
			x := strings.ToLower(strings.TrimSpace(l))
			if strings.HasPrefix(x, "file:") || strings.HasPrefix(x, "include:") {
				return nil, ErrText
			}
			if strings.HasPrefix(x, "size:") {
				dims := strings.Split(strings.TrimSpace(strings.TrimPrefix(x, "size:")), "x")
				if len(dims) != 2 {
					return nil, ErrText
				}
				w, ew := strconv.Atoi(strings.TrimSpace(dims[0]))
				h, eh := strconv.Atoi(strings.TrimSpace(dims[1]))
				if ew != nil || eh != nil || w < 1 || h < 1 || w > 8192 || h > 4320 {
					return nil, ErrCapacity
				}
			}
		}
		return assetJSON("vobsub", idx, companion)
	case "ass", "ssa":
		if len(companion) != 0 {
			return nil, ErrInput
		}
		text, e := DecodeText(raw)
		if e != nil {
			return nil, e
		}
		if e = validateASS(text, durationUS); e != nil {
			return nil, e
		}
		return assetJSON(format, text, nil)
	case "srt", "vtt":
		if len(companion) != 0 {
			return nil, ErrInput
		}
		// Text stays text. These formats used to become a burn-in asset the moment
		// they carried an <i> tag, which turned ordinary subtitles into a full
		// re-encode of the film; see tolerant.go.
		return canonicalPlain(raw, format, durationUS)
	default:
		return nil, ErrUnsupported
	}
}
func validatePGS(raw []byte) error {
	if len(raw) == 0 || len(raw) > MaxBinaryBytes {
		return ErrCapacity
	}
	cues := 0
	for len(raw) > 0 {
		if len(raw) < 13 || string(raw[:2]) != "PG" {
			return ErrText
		}
		n := int(binary.BigEndian.Uint16(raw[11:13]))
		if n > len(raw)-13 {
			return ErrText
		}
		switch raw[10] {
		case 0x14, 0x15, 0x16, 0x17, 0x80:
		default:
			return ErrText
		}
		if raw[10] == 0x16 {
			if n < 11 {
				return ErrText
			}
			w, h := int(binary.BigEndian.Uint16(raw[13:15])), int(binary.BigEndian.Uint16(raw[15:17]))
			if w < 1 || h < 1 || w > 8192 || h > 4320 {
				return ErrCapacity
			}
			cues++
		}
		if raw[10] == 0x15 && n >= 4 && raw[16]&0x80 != 0 {
			if n < 11 {
				return ErrText
			}
			w, h := int(binary.BigEndian.Uint16(raw[20:22])), int(binary.BigEndian.Uint16(raw[22:24]))
			if w < 1 || h < 1 || w > 8192 || h > 4320 {
				return ErrCapacity
			}
		}
		raw = raw[13+n:]
	}
	if cues == 0 || cues > MaxCues {
		return ErrCapacity
	}
	return nil
}
func validateASS(raw []byte, durationUS int64) error {
	lower := strings.ToLower(string(raw))
	if !strings.Contains(lower, "[script info]") || !strings.Contains(lower, "[events]") || !strings.Contains(lower, "dialogue:") {
		return ErrText
	}
	if strings.Contains(lower, "[fonts]") || strings.Contains(lower, "[graphics]") || strings.ContainsRune(lower, 0) {
		return ErrText
	}
	// Preserve script order and overlapping events. ASS layers need not be sorted.
	section := ""
	fields := []string{}
	count := 0
	for _, line := range strings.Split(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if len(line) > MaxCueBytes*4 {
			return ErrCapacity
		}
		if strings.HasPrefix(line, "[") {
			section = strings.ToLower(line)
			continue
		}
		if section != "[events]" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.ToLower(key) {
		case "format":
			fields = strings.Split(strings.ToLower(value), ",")
			for i := range fields {
				fields[i] = strings.TrimSpace(fields[i])
			}
			if len(fields) < 3 || len(fields) > 32 || fields[len(fields)-1] != "text" {
				return ErrText
			}
		case "dialogue":
			if len(fields) == 0 {
				return ErrText
			}
			values := strings.SplitN(value, ",", len(fields))
			if len(values) != len(fields) {
				return ErrText
			}
			start, end := int64(-1), int64(-1)
			for i, f := range fields {
				if f == "start" || f == "end" {
					ms, e := assTimestamp(strings.TrimSpace(values[i]))
					if e != nil {
						return e
					}
					if f == "start" {
						start = ms
					} else {
						end = ms
					}
				}
			}
			if start < 0 || end <= start || end > MaxDurationMS || durationUS > 0 && end*1000 > durationUS+2000000 {
				return ErrTiming
			}
			count++
			if count > MaxCues {
				return ErrCapacity
			}
		}
	}
	if count == 0 {
		return ErrText
	}
	return nil
}

var assTimestampPattern = regexp.MustCompile(`^([0-9]{1,2}):([0-9]{2}):([0-9]{2})\.([0-9]{2})$`)

func assTimestamp(v string) (int64, error) {
	m := assTimestampPattern.FindStringSubmatch(v)
	if m == nil {
		return 0, ErrTiming
	}
	h, _ := strconv.ParseInt(m[1], 10, 64)
	min, _ := strconv.ParseInt(m[2], 10, 64)
	sec, _ := strconv.ParseInt(m[3], 10, 64)
	cs, _ := strconv.ParseInt(m[4], 10, 64)
	if min > 59 || sec > 59 {
		return 0, ErrTiming
	}
	return ((h*60+min)*60+sec)*1000 + cs*10, nil
}

var cueTiming = regexp.MustCompile(`^(\S+)\s+-->\s+(\S+)(?:\s+(.*))?$`)
var htmlTag = regexp.MustCompile(`(?i)<(/?)(b|i|u|font|c|v|lang)([^>]*)>`)
var fontAttribute = regexp.MustCompile(`(?i)^\s*(color|size|face)\s*=\s*(?:"([^"<>]*)"|'([^'<>]*)'|([^\s"'=<>]+))`)

func fontOverrides(attrs string) (string, error) {
	var out strings.Builder
	seen := map[string]bool{}
	for strings.TrimSpace(attrs) != "" {
		m := fontAttribute.FindStringSubmatchIndex(attrs)
		if m == nil {
			return "", ErrUnsupported
		}
		key := strings.ToLower(attrs[m[2]:m[3]])
		if seen[key] {
			return "", ErrText
		}
		seen[key] = true
		value := ""
		for i := 4; i <= 8; i += 2 {
			if m[i] >= 0 {
				value = attrs[m[i]:m[i+1]]
				break
			}
		}
		switch key {
		case "color":
			color, ok := assColor(value)
			if !ok {
				return "", ErrText
			}
			out.WriteString("{\\c" + color + "}")
		case "size":
			n, e := strconv.Atoi(value)
			if e != nil || n < 8 || n > 200 {
				return "", ErrText
			}
			fmt.Fprintf(&out, "{\\fs%d}", n)
		case "face":
			if value == "" || len(value) > 128 || strings.ContainsAny(value, "{}\\/\n\r") {
				return "", ErrUnsupported
			}
			out.WriteString("{\\fn" + value + "}")
		}
		attrs = attrs[m[1]:]
	}
	return out.String(), nil
}

var assEscape = strings.NewReplacer("\\", "\\\\", "{", "\\{", "}", "\\}")

const assHeader = "[Script Info]\nScriptType: v4.00+\nPlayResX: 1920\nPlayResY: 1080\nWrapStyle: 0\nScaledBorderAndShadow: yes\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\nStyle: Default,Arial,48,&H00FFFFFF,&H00FFFFFF,&H00000000,&H80000000,0,0,0,0,100,100,0,0,1,2,1,2,80,80,54,1\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"

func assTime(ms int64) string {
	return fmt.Sprintf("%d:%02d:%02d.%02d", ms/3600000, (ms/60000)%60, (ms/1000)%60, (ms%1000)/10)
}
func assColor(c string) (string, bool) {
	colors := map[string]string{"white": "ffffff", "black": "000000", "red": "ff0000", "lime": "00ff00", "green": "008000", "blue": "0000ff", "yellow": "ffff00", "cyan": "00ffff", "aqua": "00ffff", "magenta": "ff00ff", "fuchsia": "ff00ff"}
	c = strings.ToLower(c)
	if v, ok := colors[c]; ok {
		c = v
	}
	c = strings.TrimPrefix(c, "#")
	if len(c) == 3 {
		c = string([]byte{c[0], c[0], c[1], c[1], c[2], c[2]})
	}
	if len(c) != 6 {
		return "", false
	}
	if _, e := strconv.ParseUint(c, 16, 24); e != nil {
		return "", false
	}
	return "&H" + c[4:6] + c[2:4] + c[0:2] + "&", true
}
func styledText(s string) (string, error) { return styledTextStyles(s, nil) }
func styledTextStyles(s string, styles map[string]string) (string, error) {
	if len(s) > MaxCueBytes || strings.ContainsRune(s, 0) {
		return "", ErrText
	}
	var out strings.Builder
	out.WriteString(styles[""])
	if strings.HasPrefix(s, "{\\an") {
		if len(s) < 6 || s[5] != '}' || s[4] < '1' || s[4] > '9' {
			return "", ErrUnsupported
		}
		out.WriteString(s[:6])
		s = s[6:]
	}
	type nestedStyle struct{ name, tags string }
	var stack []nestedStyle
	pos := 0
	for _, m := range htmlTag.FindAllStringSubmatchIndex(s, -1) {
		literal := s[pos:m[0]]
		if strings.ContainsAny(literal, "<>") {
			return "", ErrUnsupported
		}
		out.WriteString(assEscape.Replace(html.UnescapeString(literal)))
		close := s[m[2]:m[3]] == "/"
		tag := strings.ToLower(s[m[4]:m[5]])
		attrs := s[m[6]:m[7]]
		if close {
			if strings.TrimSpace(attrs) != "" || len(stack) == 0 || stack[len(stack)-1].name != tag {
				return "", ErrText
			}
			stack = stack[:len(stack)-1]
			out.WriteString("{\\b0\\i0\\u0\\s0\\c\\fs48\\fnArial}" + styles[""])
			for _, parent := range stack {
				out.WriteString(parent.tags)
			}
			pos = m[1]
			continue
		}
		if (tag == "b" || tag == "i" || tag == "u") && strings.TrimSpace(attrs) != "" {
			return "", ErrUnsupported
		}
		before := out.Len()
		switch tag {
		case "b", "i", "u":
			v := "1"
			if close {
				v = "0"
			}
			out.WriteString("{\\" + tag + v + "}")
		case "font":
			tags, e := fontOverrides(attrs)
			if e != nil {
				return "", e
			}
			out.WriteString(tags)
		case "c":
			if close {
				out.WriteString("{\\c}" + styles[""])
			} else {
				for _, c := range strings.Split(strings.TrimSpace(attrs), ".") {
					if c == "" {
						continue
					}
					if rule, ok := styles[c]; ok {
						out.WriteString(rule)
						continue
					}
					color, ok := assColor(c)
					if !ok {
						return "", ErrUnsupported
					}
					out.WriteString("{\\c" + color + "}")
				}
			}
		case "v", "lang": // Voice/language annotations carry no executable markup.
		}
		stack = append(stack, nestedStyle{tag, out.String()[before:]})
		pos = m[1]
	}
	literal := s[pos:]
	if strings.ContainsAny(literal, "<>") {
		return "", ErrUnsupported
	}
	out.WriteString(assEscape.Replace(html.UnescapeString(literal)))
	text := out.String()
	text = strings.ReplaceAll(text, "\n", "\\N")
	return text, nil
}
func percentage(s string) (float64, error) {
	if !strings.HasSuffix(s, "%") {
		return 0, ErrText
	}
	n, e := strconv.ParseFloat(strings.TrimSuffix(s, "%"), 64)
	if e != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 100 {
		return 0, ErrText
	}
	return n, nil
}
func positionASS(settings string) (string, error) {
	if settings == "" {
		return "", nil
	}
	x, y, size := 50.0, 95.0, 100.0
	align := 2
	horizontal := 2
	vertical := 0
	seen := map[string]bool{}
	for _, part := range strings.Fields(settings) {
		key, val, ok := strings.Cut(part, ":")
		if !ok || seen[key] {
			return "", ErrText
		}
		seen[key] = true
		switch key {
		case "align":
			switch val {
			case "start", "left":
				horizontal = 1
			case "center", "middle":
				horizontal = 2
			case "end", "right":
				horizontal = 3
			default:
				return "", ErrText
			}
		case "position":
			a := strings.Split(val, ",")
			if len(a) > 2 {
				return "", ErrText
			}
			var e error
			x, e = percentage(a[0])
			if e != nil {
				return "", e
			}
			if len(a) > 1 {
				switch a[1] {
				case "line-left", "start":
					horizontal = 1
				case "center", "middle":
					horizontal = 2
				case "line-right", "end":
					horizontal = 3
				case "auto":
				default:
					return "", ErrText
				}
			}
		case "size":
			var e error
			size, e = percentage(val)
			if e != nil || size == 0 {
				return "", ErrText
			}
		case "line":
			a := strings.Split(val, ",")
			if len(a) > 2 {
				return "", ErrText
			}
			if a[0] == "auto" {
				continue
			}
			if strings.HasSuffix(a[0], "%") {
				var e error
				y, e = percentage(a[0])
				if e != nil {
					return "", e
				}
				vertical = 6
			} else {
				n, e := strconv.Atoi(a[0])
				if e != nil || n < -100 || n > 100 {
					return "", ErrText
				}
				if n < 0 {
					y = 100 + float64(n)*5
					vertical = 0
				} else {
					y = float64(n) * 5
					vertical = 6
				}
				if y < 0 || y > 100 {
					return "", ErrText
				}
			}
			if len(a) > 1 {
				switch a[1] {
				case "start":
					vertical = 6
				case "center":
					vertical = 3
				case "end":
					vertical = 0
				default:
					return "", ErrText
				}
			}
		default:
			return "", ErrUnsupported // Vertical writing / region scroll isn't silently flattened.
		}
	}
	align = horizontal + vertical
	margin := int((100 - size) * 1920 / 200)
	return fmt.Sprintf("%d,%d,0,,{\\an%d\\pos(%.2f,%.2f)}", margin, margin, align, x*19.2, y*10.8), nil
}
func textASS(raw []byte, format string, durationUS int64) ([]byte, error) {
	s := strings.TrimPrefix(string(raw), "\ufeff")
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	if format == "vtt" {
		header, rest, ok := strings.Cut(s, "\n")
		if !ok || !(header == "WEBVTT" || strings.HasPrefix(header, "WEBVTT ") || strings.HasPrefix(header, "WEBVTT\t")) || strings.Contains(header, "-->") {
			return nil, ErrText
		}
		s = rest
		s = strings.TrimLeft(s, "\n")
	}
	var out strings.Builder
	out.WriteString(assHeader)
	count := 0
	styles := map[string]string{}
	last := int64(-1)
	for _, block := range regexp.MustCompile(`\n[ \t]*\n`).Split(strings.TrimSpace(s), -1) {
		lines := strings.Split(block, "\n")
		if len(lines) == 0 {
			continue
		}
		if format == "vtt" && (lines[0] == "NOTE" || strings.HasPrefix(lines[0], "NOTE ")) {
			continue
		}
		if lines[0] == "STYLE" {
			if count != 0 {
				return nil, ErrText
			}
			var e error
			styles, e = parseCueCSS(strings.Join(lines[1:], "\n"), styles)
			if e != nil {
				return nil, e
			}
			continue
		}
		if lines[0] == "REGION" {
			return nil, ErrUnsupported
		}
		i := 0
		if !strings.Contains(lines[0], "-->") {
			i = 1
		}
		if i >= len(lines)-1 {
			return nil, ErrText
		}
		m := cueTiming.FindStringSubmatch(strings.TrimSpace(lines[i]))
		if m == nil {
			return nil, ErrTiming
		}
		start, e := timestamp(m[1], format)
		if e != nil {
			return nil, e
		}
		end, e := timestamp(m[2], format)
		if e != nil || end <= start || start < last || end > MaxDurationMS || durationUS > 0 && end*1000 > durationUS+2000000 {
			return nil, ErrTiming
		}
		last = start
		text, e := styledTextStyles(strings.Join(lines[i+1:], "\n"), styles)
		if e != nil {
			return nil, e
		}
		pos, e := positionASS(m[3])
		if e != nil {
			return nil, e
		}
		if pos == "" {
			pos = "0,0,0,,"
		}
		fmt.Fprintf(&out, "Dialogue: 0,%s,%s,Default,,%s%s\n", assTime(start), assTime(end), pos, text)
		count++
		if count > MaxCues {
			return nil, ErrCapacity
		}
	}
	if count == 0 {
		return nil, ErrText
	}
	return []byte(out.String()), nil
}
