package subtitles

import (
	"errors"
	"strings"
	"testing"
)

func TestPlainTextBoundariesAndOverlaps(t *testing.T) {
	raw := []byte("\xef\xbb\xbf1\r\n00:00:05,800 --> 00:00:06,400\r\nCross boundary & remain\r\n\r\n2\r\n00:00:06,000 --> 00:00:07,000\r\nOverlap\r\n")
	cues, e := Parse(raw, "srt")
	if e != nil {
		t.Fatal(e)
	}
	if len(cues) != 2 || cues[0].StartMS != 5800 || cues[0].EndMS != 6400 || cues[1].StartMS != 6000 {
		t.Fatalf("wrong cue intervals: %+v", cues)
	}
	output, e := WebVTT(cues, 132000)
	if e != nil {
		t.Fatal(e)
	}
	s := string(output)
	for _, want := range []string{"X-TIMESTAMP-MAP=LOCAL:00:00:00.000,MPEGTS:132000", "00:00:05.800 --> 00:00:06.400", "Cross boundary &amp; remain", "00:00:06.000 --> 00:00:07.000"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q", want)
		}
	}
}
func TestUnsupportedFeaturesAreRejectedNotStripped(t *testing.T) {
	for name, raw := range map[string]string{
		"vtt_style":    "WEBVTT\n\nSTYLE\n::cue {color:red}\n\n00:00.000 --> 00:01.000\ntext",
		"vtt_region":   "WEBVTT\n\nREGION\nid:region1\n\n00:00.000 --> 00:01.000\ntext",
		"vtt_position": "WEBVTT\n\n00:00.000 --> 00:01.000 line:10%\ntext",
		"vtt_voice":    "WEBVTT\n\n00:00.000 --> 00:01.000\n<v Speaker>text",
		"vtt_note":     "WEBVTT\n\nNOTE retained source feature\ntext",
		"srt_markup":   "1\n00:00:00,000 --> 00:00:01,000\n<i>italic</i>",
		"srt_ass":      "1\n00:00:00,000 --> 00:00:01,000\n{\\an8}position",
		"srt_position": "1\n00:00:00,000 --> 00:00:01,000 X1:1\nposition",
	} {
		t.Run(name, func(t *testing.T) {
			format := "srt"
			if strings.HasPrefix(name, "vtt") {
				format = "vtt"
			}
			if _, e := Parse([]byte(raw), format); e == nil {
				t.Fatal("unsupported feature accepted")
			}
		})
	}
}
func TestTextEncodingAndLimits(t *testing.T) {
	base := "1\n00:00:00,000 --> 00:00:01,000\n"
	for _, body := range []string{"\x00", "\xff", strings.Repeat("a", MaxCueBytes+1)} {
		if _, e := Parse([]byte(base+body), "srt"); e == nil {
			t.Fatal("invalid body admitted")
		}
	}
	if _, e := Parse(make([]byte, MaxInputBytes+1), "srt"); !errors.Is(e, ErrCapacity) {
		t.Fatal(e)
	}
	if _, e := Parse([]byte(base+strings.Repeat("a", MaxCueBytes)), "srt"); e != nil {
		t.Fatal(e)
	}
}
func TestTimingRejectsRatherThanSortsOrClamps(t *testing.T) {
	for _, line := range []string{"-1:00:00,000 --> 00:00:01,000", "00:00:01,000 --> 00:00:01,000", "00:60:00,000 --> 01:00:01,000", "00:00:00,000 --> 24:00:00,001", "00:00:00,NaN --> 00:00:01,000"} {
		if _, e := Parse([]byte("1\n"+line+"\ntext"), "srt"); !errors.Is(e, ErrTiming) {
			t.Fatalf("%s: %v", line, e)
		}
	}
	if _, e := Parse([]byte("1\n00:00:02,000 --> 00:00:03,000\na\n\n2\n00:00:01,000 --> 00:00:02,000\nb"), "srt"); !errors.Is(e, ErrTiming) {
		t.Fatal(e)
	}
}
func TestVTTEntitiesArePlainTextNotMarkup(t *testing.T) {
	cues, e := Parse([]byte("WEBVTT\n\nlabel\n00:02.000 --> 00:03.000\nA &amp; B &lt; C"), "vtt")
	if e != nil {
		t.Fatal(e)
	}
	if cues[0].Text != "A & B < C" {
		t.Fatal(cues)
	}
	out, e := WebVTT(cues, 0)
	if e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(string(out), "A &amp; B &lt; C") {
		t.Fatal("text not escaped")
	}
	if _, e := Parse([]byte("WEBVTT\n\n00:02.000 --> 00:03.000\n&unsupported;"), "vtt"); !errors.Is(e, ErrText) {
		t.Fatal(e)
	}
}
func TestEmitterDoesNotTrustConstructedCue(t *testing.T) {
	for _, c := range []Cue{{-1, 100, "a"}, {100, 1, "a"}, {0, 100, "a\n\nWEBVTT"}, {0, 100, "a\x00"}} {
		if _, e := WebVTT([]Cue{c}, 0); e == nil {
			t.Fatal("unsafe cue accepted")
		}
	}
	for _, ts := range []int64{-1, 1 << 33} {
		if _, e := WebVTT([]Cue{{0, 1, "a"}}, ts); e == nil {
			t.Fatal("unsafe clock")
		}
	}
}
