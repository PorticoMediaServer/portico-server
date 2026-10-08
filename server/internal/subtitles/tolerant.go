package subtitles

import (
	"bytes"
	"html"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// Real subtitle files are not tidy. An SRT from the wild has <i> around a line,
// {\an8} in front of a sign, a Windows-1252 é, a lone carriage return, a cue that
// starts before the one above it, hours written with one digit. The first parser
// here admitted only the immaculate subset and sent everything else to the
// burn-in renderer, which meant that turning on perfectly ordinary English
// subtitles re-encoded the whole film. Text that every player can draw must
// reach the player as text.
//
// This parser reads what is there and keeps what a player can show: the words,
// their timing, whether the whole cue is italic, and whether it was placed at the
// top of the picture. Everything else about styling is dropped, not refused.

// StyledCue is a text cue with the two presentation facts every player can honour.
type StyledCue struct {
	StartMS, EndMS int64
	Text           string
	Italic         bool
	// Top is true for a cue authored at the top of the picture ({\an7-9}, or a
	// WebVTT line setting in the upper half): a sign or a translation that must
	// not cover the speaker's own subtitle.
	Top bool
}

var (
	tolerantBlock  = regexp.MustCompile(`\n[ \t]*\n+`)
	tolerantTiming = regexp.MustCompile(`^\s*((?:\d{1,3}:)?\d{1,2}:\d{1,2}[,.]\d{1,3})\s*-->\s*((?:\d{1,3}:)?\d{1,2}:\d{1,2}[,.]\d{1,3})(.*)$`)
	tolerantStamp  = regexp.MustCompile(`^(?:(\d{1,3}):)?(\d{1,2}):(\d{1,2})[,.](\d{1,3})$`)
	markupTag      = regexp.MustCompile(`</?[A-Za-z][^<>]{0,255}>`)
	assOverride    = regexp.MustCompile(`\{[^{}]{0,512}\}`)
	assAlignment   = regexp.MustCompile(`\\an?([0-9]{1,2})`)
	vttLine        = regexp.MustCompile(`(?:^|\s)line:(-?\d+(?:\.\d+)?)(%?)`)
	italicWrapped  = regexp.MustCompile(`(?is)^\s*<i>.*</i>\s*$`)
)

// decodeLoosely returns UTF-8 text for any plausible subtitle encoding: UTF-8 or
// UTF-16 with or without a byte-order mark, and Windows-1252 for the legacy files
// that are neither. Windows-1252 is the fallback because it decodes every byte
// and is what the overwhelming majority of non-Unicode Western subtitles are.
func decodeLoosely(raw []byte) ([]byte, error) {
	if len(raw) == 0 || len(raw) > MaxInputBytes {
		return nil, ErrCapacity
	}
	if decoded, err := DecodeText(raw); err == nil {
		return decoded, nil
	}
	if bytes.HasPrefix(raw, []byte{0xff, 0xfe}) || bytes.HasPrefix(raw, []byte{0xfe, 0xff}) {
		return nil, ErrText
	}
	out, err := charmap.Windows1252.NewDecoder().Bytes(raw)
	if err != nil || !utf8.Valid(out) || len(out) > MaxInputBytes*2 {
		return nil, ErrText
	}
	return out, nil
}

func tolerantStampMS(s string) (int64, bool) {
	m := tolerantStamp.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, false
	}
	var h int64
	if m[1] != "" {
		h, _ = strconv.ParseInt(m[1], 10, 64)
	}
	minutes, _ := strconv.ParseInt(m[2], 10, 64)
	seconds, _ := strconv.ParseInt(m[3], 10, 64)
	fraction := m[4]
	for len(fraction) < 3 {
		fraction += "0"
	}
	ms, _ := strconv.ParseInt(fraction, 10, 64)
	if minutes >= 60 || seconds >= 60 {
		return 0, false
	}
	value := ((h*60+minutes)*60+seconds)*1000 + ms
	if value > MaxDurationMS {
		return 0, false
	}
	return value, true
}

// plainCueText reduces one cue body to the words a player draws.
func plainCueText(body string) (text string, italic, top bool) {
	for _, block := range assOverride.FindAllString(body, -1) {
		if m := assAlignment.FindStringSubmatch(block); m != nil {
			if n, _ := strconv.Atoi(m[1]); n >= 7 && n <= 9 {
				top = true
			}
		}
		if strings.Contains(block, `\i1`) && strings.HasPrefix(strings.TrimSpace(body), block) {
			italic = true
		}
	}
	body = assOverride.ReplaceAllString(body, "")
	italic = italic || italicWrapped.MatchString(body)
	body = markupTag.ReplaceAllString(body, "")
	body = strings.NewReplacer(`\N`, "\n", `\n`, "\n", `\h`, " ").Replace(body)
	body = html.UnescapeString(body)
	body = strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t':
			return r
		case r == '\u00a0':
			return ' '
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r) && r != '\u200d' && r != '\u200c':
			return -1
		}
		return r
	}, body)
	lines := strings.Split(body, "\n")
	kept := lines[:0]
	for _, line := range lines {
		if line = strings.TrimSpace(line); line != "" {
			kept = append(kept, line)
		}
	}
	text = strings.Join(kept, "\n")
	// A cue this long is a broken file, not a subtitle; keep the part that fits
	// rather than losing the whole track.
	for len(text) > MaxCueBytes {
		_, size := utf8.DecodeLastRuneInString(text)
		text = text[:len(text)-size]
	}
	return text, italic, top
}

// ParseTolerant reads SRT or WebVTT as it is actually written. It returns cues in
// presentation order. It fails only when the file holds no readable cue at all or
// exceeds the capacity every subtitle is held to.
func ParseTolerant(raw []byte, format string, durationUS int64) ([]StyledCue, error) {
	if format != "srt" && format != "vtt" {
		return nil, ErrText
	}
	decoded, err := decodeLoosely(raw)
	if err != nil {
		return nil, err
	}
	text := strings.TrimPrefix(string(decoded), "\ufeff")
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	limitMS := int64(0)
	if durationUS > 0 {
		limitMS = durationUS/1000 + 2000
	}
	var cues []StyledCue
	for _, block := range tolerantBlock.Split(strings.Trim(text, "\n \t"), -1) {
		lines := strings.Split(strings.Trim(block, "\n"), "\n")
		timing := -1
		// The timing line is the first or second line: an SRT counter or a WebVTT
		// cue identifier may sit above it. WEBVTT, NOTE, STYLE and REGION blocks
		// have no timing line and fall through.
		for i := 0; i < len(lines) && i < 2; i++ {
			if strings.Contains(lines[i], "-->") {
				timing = i
				break
			}
		}
		if timing < 0 || timing+1 >= len(lines) {
			continue
		}
		m := tolerantTiming.FindStringSubmatch(lines[timing])
		if m == nil {
			continue
		}
		start, ok1 := tolerantStampMS(m[1])
		end, ok2 := tolerantStampMS(m[2])
		if !ok1 || !ok2 || end <= start {
			continue
		}
		if limitMS > 0 {
			// A cue past the end of the film belongs to a different cut of it.
			if start >= limitMS {
				continue
			}
			if end > limitMS {
				end = limitMS
			}
		}
		body, italic, top := plainCueText(strings.Join(lines[timing+1:], "\n"))
		if body == "" {
			continue
		}
		if l := vttLine.FindStringSubmatch(m[3]); l != nil {
			if v, convErr := strconv.ParseFloat(l[1], 64); convErr == nil && ((l[2] == "%" && v < 50) || (l[2] == "" && v >= 0 && v < 8)) {
				top = true
			}
		}
		cues = append(cues, StyledCue{StartMS: start, EndMS: end, Text: body, Italic: italic, Top: top})
		if len(cues) > MaxCues {
			return nil, ErrCapacity
		}
	}
	if len(cues) == 0 {
		return nil, ErrText
	}
	sort.SliceStable(cues, func(i, j int) bool { return cues[i].StartMS < cues[j].StartMS })
	return cues, nil
}

// ParseASSAsText reads an ASS or SSA script for a player that draws plain text.
// Override tags are removed, vector drawings are skipped, and the lines that
// typesetters stack at one instant for a layered effect collapse into one.
func ParseASSAsText(raw []byte, durationUS int64) ([]StyledCue, error) {
	decoded, err := decodeLoosely(raw)
	if err != nil {
		return nil, err
	}
	text := strings.ReplaceAll(strings.ReplaceAll(strings.TrimPrefix(string(decoded), "\ufeff"), "\r\n", "\n"), "\r", "\n")
	limitMS := int64(0)
	if durationUS > 0 {
		limitMS = durationUS/1000 + 2000
	}
	startColumn, endColumn, textColumn, columns := 1, 2, 9, 10
	inEvents := false
	seen := map[string]bool{}
	var cues []StyledCue
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "[") {
			inEvents = strings.EqualFold(line, "[Events]")
			continue
		}
		if !inEvents {
			continue
		}
		if rest, ok := cutPrefixFold(line, "Format:"); ok {
			fields := strings.Split(rest, ",")
			columns = len(fields)
			for i, f := range fields {
				switch strings.ToLower(strings.TrimSpace(f)) {
				case "start":
					startColumn = i
				case "end":
					endColumn = i
				case "text":
					textColumn = i
				}
			}
			continue
		}
		rest, ok := cutPrefixFold(line, "Dialogue:")
		if !ok || columns < 3 || textColumn != columns-1 {
			continue
		}
		fields := strings.SplitN(rest, ",", columns)
		if len(fields) != columns {
			continue
		}
		start, e1 := assTimestamp(strings.TrimSpace(fields[startColumn]))
		end, e2 := assTimestamp(strings.TrimSpace(fields[endColumn]))
		if e1 != nil || e2 != nil || end <= start || start > MaxDurationMS {
			continue
		}
		if limitMS > 0 {
			if start >= limitMS {
				continue
			}
			if end > limitMS {
				end = limitMS
			}
		}
		body := fields[textColumn]
		// \p1 and above switch the line to vector drawing; its "text" is a path.
		if drawing := regexp.MustCompile(`\\p[1-9]`); drawing.MatchString(body) {
			continue
		}
		plain, italic, top := plainCueText(body)
		if plain == "" {
			continue
		}
		key := strconv.FormatInt(start, 10) + "|" + strconv.FormatInt(end, 10) + "|" + plain
		if seen[key] {
			continue
		}
		seen[key] = true
		cues = append(cues, StyledCue{StartMS: start, EndMS: end, Text: plain, Italic: italic, Top: top})
		if len(cues) > MaxCues {
			return nil, ErrCapacity
		}
	}
	if len(cues) == 0 {
		return nil, ErrText
	}
	sort.SliceStable(cues, func(i, j int) bool { return cues[i].StartMS < cues[j].StartMS })
	return cues, nil
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) < len(prefix) || !strings.EqualFold(s[:len(prefix)], prefix) {
		return "", false
	}
	return s[len(prefix):], true
}

func validRender(v string) bool { return v == "" || v == "text" || v == "styled" }

// renderChoice applies the text-or-styled choice to a canonical subtitle. Only a
// styled text script has a choice to make: a plain document is already text, and
// a bitmap track has no text to keep. A script that cannot be read as text stays
// styled rather than being lost.
func renderChoice(canonical []byte, render string, durationUS int64) []byte {
	if render == "styled" || assetRenderer(canonical) != "burn_in" {
		return canonical
	}
	asset, err := DecodeRenderAsset(canonical)
	if err != nil || (asset.Format != "ass" && asset.Format != "ssa") {
		return canonical
	}
	cues, err := ParseASSAsText(asset.Data, durationUS)
	if err != nil {
		return canonical
	}
	document, err := textDocument(cues)
	if err != nil {
		return canonical
	}
	return document
}
