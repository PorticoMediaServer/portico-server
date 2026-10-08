package decoder

import (
	"strings"
	"testing"
)

func TestAnalysisClosedRecipes(t *testing.T) {
	for _, kind := range []string{"pcm", "loudness", "image", "trickplay", "scenes"} {
		args, err := AnalysisArgs("http://127.0.0.1:1234/input/test", AnalysisSpec{Kind: kind, StartUS: 1234567, IntervalUS: 1000000, MaxFrames: 12, MaxDurationUS: 60000000})
		if err != nil {
			t.Fatal(err)
		}
		joined := " " + strings.Join(args, " ") + " "
		if strings.Contains(joined, " -t ") || strings.Contains(joined, " -to ") || kind != "image" && strings.Contains(joined, " -frames") {
			t.Fatalf("truncated full analysis: %s", joined)
		}
		if !strings.Contains(joined, " -xerror ") || !strings.Contains(joined, "-protocol_whitelist http,tcp") || !strings.HasSuffix(joined, "pipe:1 ") {
			t.Fatal(joined)
		}
		if kind == "image" && !strings.Contains(joined, "-ss 1.234567") {
			t.Fatal("timestamp precision", joined)
		}
		if strings.Contains(joined, "setpts=PTS-STARTPTS") {
			t.Fatal("discarded stream offset", joined)
		}
	}
	for _, spec := range []AnalysisSpec{{Kind: "arbitrary-filter", MaxDurationUS: 1}, {Kind: "pcm"}, {Kind: "image", StartUS: -1, MaxDurationUS: 1}, {Kind: "image", StartUS: 2, MaxDurationUS: 2}, {Kind: "scenes", IntervalUS: 1, MaxFrames: 1, MaxDurationUS: 2}, {Kind: "trickplay", IntervalUS: 1000000, MaxFrames: 65537, MaxDurationUS: 60000000}} {
		if _, err := AnalysisArgs("ignored", spec); err == nil {
			t.Fatalf("accepted unsafe recipe %+v", spec)
		}
	}
}
