// Package subtitles owns bounded subtitle admission. Rich assets go through
// CanonicalAsset and the confined renderer, never an untrusted HTML overlay.
package subtitles

import (
	"errors"
	"fmt"
	"html"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	MaxInputBytes       = 4 << 20
	MaxCues             = 20000
	MaxCueBytes         = 8192
	MaxDurationMS int64 = 24 * 60 * 60 * 1000
)

var (
	ErrCapacity = errors.New("subtitle capacity exceeded")
	ErrText     = errors.New("unsupported subtitle text")
	ErrTiming   = errors.New("unsupported subtitle timing")
)

type Cue struct {
	StartMS, EndMS int64
	Text           string
}

// Parse accepts the deliberately plain subset only. Overlapping cues are valid;
// out-of-order starts are rejected rather than silently reordered. Callers must
// pass richer formats to CanonicalAsset without substituting this plain output.
func Parse(raw []byte, format string) ([]Cue, error) {
	if len(raw) > MaxInputBytes {
		return nil, ErrCapacity
	}
	if !utf8.Valid(raw) {
		return nil, ErrText
	}
	text := strings.TrimPrefix(string(raw), "\ufeff")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	if strings.ContainsRune(text, '\r') {
		return nil, ErrText
	}
	for _, r := range text {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return nil, ErrText
		}
	}
	switch format {
	case "srt":
	case "vtt":
		if text != "WEBVTT" && !strings.HasPrefix(text, "WEBVTT\n") {
			return nil, ErrText
		}
		text = strings.TrimPrefix(text, "WEBVTT")
		if text != "" && !strings.HasPrefix(text, "\n\n") {
			return nil, ErrText
		}
	default:
		return nil, ErrText
	}
	var cues []Cue
	for _, block := range strings.Split(strings.TrimSpace(text), "\n\n") {
		block = strings.Trim(block, "\n")
		if block == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		if len(lines) < 2 {
			return nil, ErrText
		}
		timing := 0
		if !strings.Contains(lines[0], " --> ") {
			// SRT sequence numbers and simple VTT identifiers carry no visual styling.
			if format == "srt" {
				n, e := strconv.ParseUint(lines[0], 10, 32)
				if e != nil || n == 0 {
					return nil, ErrText
				}
			} else if len(lines[0]) > 128 || strings.ContainsAny(lines[0], "<>&") {
				return nil, ErrText
			}
			timing = 1
		}
		if len(lines) <= timing+1 {
			return nil, ErrText
		}
		parts := strings.Split(lines[timing], " --> ")
		if len(parts) != 2 {
			return nil, ErrTiming
		}
		start, e := timestamp(parts[0], format)
		if e != nil {
			return nil, e
		}
		end, e := timestamp(parts[1], format)
		if e != nil {
			return nil, e
		}
		if end <= start || end > MaxDurationMS {
			return nil, ErrTiming
		}
		if len(cues) > 0 && start < cues[len(cues)-1].StartMS {
			return nil, ErrTiming
		}
		body := strings.Join(lines[timing+1:], "\n")
		if len(body) > MaxCueBytes {
			return nil, ErrCapacity
		}
		if strings.TrimSpace(body) == "" || strings.ContainsAny(body, "<>") || strings.Contains(body, "{\\") {
			return nil, ErrText
		}
		if len(cues) >= MaxCues {
			return nil, ErrCapacity
		}
		if format == "vtt" {
			for _, part := range strings.Split(body, "&")[1:] {
				if !strings.HasPrefix(part, "amp;") && !strings.HasPrefix(part, "lt;") && !strings.HasPrefix(part, "gt;") && !strings.HasPrefix(part, "nbsp;") {
					return nil, ErrText
				}
			}
			body = html.UnescapeString(body)
		}
		cues = append(cues, Cue{start, end, body})
	}
	if len(cues) == 0 {
		return nil, ErrText
	}
	return cues, nil
}
func timestamp(s, format string) (int64, error) {
	sep := "."
	if format == "srt" {
		sep = ","
	}
	parts := strings.Split(s, sep)
	if len(parts) != 2 || len(parts[1]) != 3 {
		return 0, ErrTiming
	}
	hms := strings.Split(parts[0], ":")
	if len(hms) != 3 && (format != "vtt" || len(hms) != 2) {
		return 0, ErrTiming
	}
	if len(hms) == 2 {
		hms = append([]string{"00"}, hms...)
	}
	parse := func(s string) (int64, error) {
		if len(s) != 2 {
			return 0, ErrTiming
		}
		for _, r := range s {
			if r < '0' || r > '9' {
				return 0, ErrTiming
			}
		}
		return strconv.ParseInt(s, 10, 64)
	}
	h, e := parse(hms[0])
	if e != nil {
		return 0, e
	}
	m, e := parse(hms[1])
	if e != nil {
		return 0, e
	}
	sval, e := parse(hms[2])
	if e != nil {
		return 0, e
	}
	for _, r := range parts[1] {
		if r < '0' || r > '9' {
			return 0, ErrTiming
		}
	}
	ms, e := strconv.ParseInt(parts[1], 10, 64)
	if e != nil || m >= 60 || sval >= 60 {
		return 0, ErrTiming
	}
	result := ((h*60+m)*60+sval)*1000 + ms
	if result > MaxDurationMS {
		return 0, ErrTiming
	}
	return result, nil
}

// WebVTT emits a fixed safe header. The mapping is an artifact-derived 90 kHz
// clock, never a user-supplied header; cues are already in presentation-zero time.
func WebVTT(cues []Cue, mpegTS int64) ([]byte, error) {
	if mpegTS < 0 || mpegTS >= 1<<33 || len(cues) == 0 || len(cues) > MaxCues {
		return nil, ErrTiming
	}
	var b strings.Builder
	fmt.Fprintf(&b, "WEBVTT\nX-TIMESTAMP-MAP=LOCAL:00:00:00.000,MPEGTS:%d\n\n", mpegTS)
	stamp := func(v int64) string {
		return fmt.Sprintf("%02d:%02d:%02d.%03d", v/3600000, v/60000%60, v/1000%60, v%1000)
	}
	for i, c := range cues {
		if c.StartMS < 0 || c.EndMS <= c.StartMS || c.EndMS > MaxDurationMS || (i > 0 && c.StartMS < cues[i-1].StartMS) {
			return nil, ErrTiming
		}
		// Revalidate caller-constructed cues, not only Parse results.
		if !utf8.ValidString(c.Text) || len(c.Text) > MaxCueBytes || strings.TrimSpace(c.Text) == "" || strings.ContainsAny(c.Text, "\r") || strings.Contains(c.Text, "{\\") || strings.Contains(c.Text, "\n\n") {
			return nil, ErrText
		}
		for _, r := range c.Text {
			if unicode.IsControl(r) && r != '\n' && r != '\t' {
				return nil, ErrText
			}
		}
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", i+1, stamp(c.StartMS), stamp(c.EndMS), html.EscapeString(c.Text))
		if b.Len() > MaxInputBytes {
			return nil, ErrCapacity
		}
	}
	return []byte(b.String()), nil
}
