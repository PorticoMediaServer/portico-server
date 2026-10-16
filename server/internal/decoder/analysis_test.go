package decoder

import (
	"strings"
	"testing"
)

func TestAnalysisClosedRecipes(t *testing.T) {
	for _, kind := range []string{"pcm_shared", "image", "trickplay", "scenes"} {
		args, err := AnalysisArgs("http://127.0.0.1:1234/input/test", AnalysisSpec{Kind: kind, StartUS: 1234567, IntervalUS: 1000000, MaxFrames: 12, MaxDurationUS: 60000000})
		if err != nil {
			t.Fatal(err)
		}
		joined := " " + strings.Join(args, " ") + " "
		if strings.Contains(joined, " -t ") || strings.Contains(joined, " -to ") || kind != "image" && strings.Contains(joined, " -frames") {
			t.Fatalf("truncated full analysis: %s", joined)
		}
		preview := kind == "image" || kind == "trickplay" || kind == "scenes"
		if strings.Contains(joined, " -xerror ") == preview || preview && !strings.Contains(joined, "-max_error_rate 0.05") || !strings.Contains(joined, "-protocol_whitelist http,tcp") || !strings.HasSuffix(joined, "pipe:1 ") {
			t.Fatal(joined)
		}
		if kind == "image" && !strings.Contains(joined, "-ss 1.234567") {
			t.Fatal("timestamp precision", joined)
		}
		if strings.Contains(joined, "setpts=PTS-STARTPTS") {
			t.Fatal("discarded stream offset", joined)
		}
	}
	for _, spec := range []AnalysisSpec{{Kind: "arbitrary-filter", MaxDurationUS: 1}, {Kind: "pcm_shared"}, {Kind: "image", StartUS: -1, MaxDurationUS: 1}, {Kind: "image", StartUS: 2, MaxDurationUS: 2}, {Kind: "scenes", IntervalUS: 1, MaxFrames: 1, MaxDurationUS: 2}, {Kind: "trickplay", IntervalUS: 1000000, MaxFrames: 65537, MaxDurationUS: 60000000}} {
		if _, err := AnalysisArgs("ignored", spec); err == nil {
			t.Fatalf("accepted unsafe recipe %+v", spec)
		}
	}
}

func TestAnalysisPreviewColorAndDisplayGeometry(t *testing.T) {
	original := CurrentToolchain()
	defer RestoreToolchain(original)
	for _, kind := range []string{"image", "trickplay", "scenes"} {
		args, err := AnalysisArgs("input", AnalysisSpec{Kind: kind, MaxDurationUS: 60000000, IntervalUS: 1000000, MaxFrames: 12, SourceVideo: PreviewVideoFacts{DynamicRange: "hdr10"}})
		graph := strings.Join(args, " ")
		if err != nil || !strings.Contains(graph, "reset_sar=1") || !strings.Contains(graph, "zscale=t=linear") || !strings.Contains(graph, "tonemap=") {
			t.Fatal(graph, err)
		}
	}
	spec := AnalysisSpec{Kind: "image", MaxDurationUS: 60000000, SourceVideo: PreviewVideoFacts{DynamicRange: "dolby_vision", DolbyVisionProfile: 5}}
	RestoreToolchain(nil)
	if _, err := AnalysisArgs("input", spec); err == nil {
		t.Fatal("unqualified Dolby Vision color conversion")
	}
	RestoreToolchain(&ToolchainFacts{VulkanAvailable: true, Filters: map[string]bool{"libplacebo": true}})
	args, err := AnalysisArgs("input", spec)
	if err != nil || !strings.Contains(strings.Join(args, " "), "libplacebo=") || !analysisNeedsHardware(spec) {
		t.Fatal(args, err)
	}
}
